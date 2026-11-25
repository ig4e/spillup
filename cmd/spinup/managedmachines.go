package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/tailnet"
	"github.com/darkyeg/spinup/internal/webui"
)

type discoveryResult[T any] struct {
	ready chan struct{}
	value T
	err   error
	until time.Time
}

type discoveryCache[K comparable, T any] struct {
	mu      sync.Mutex
	entries map[K]*discoveryResult[T]
	sweep   time.Time
}

// Coalesce concurrent requests without retaining cancelled reads or holding a lock across I/O.
func (c *discoveryCache[K, T]) get(ctx context.Context, key K, ttl time.Duration, read func() (T, error)) (T, error) {
	now := time.Now()
	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[K]*discoveryResult[T])
	}
	if now.After(c.sweep) {
		for k, v := range c.entries {
			if !v.until.IsZero() && !now.Before(v.until) {
				delete(c.entries, k)
			}
		}
		c.sweep = now.Add(time.Minute)
	}
	if old := c.entries[key]; old != nil && (old.until.IsZero() || now.Before(old.until)) {
		c.mu.Unlock()
		select {
		case <-old.ready:
			return old.value, old.err
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		}
	}
	entry := &discoveryResult[T]{ready: make(chan struct{})}
	c.entries[key] = entry
	c.mu.Unlock()
	value, err := read()
	c.mu.Lock()
	entry.value, entry.err, entry.until = value, err, time.Now().Add(ttl)
	if ctx.Err() != nil {
		delete(c.entries, key)
	}
	close(entry.ready)
	c.mu.Unlock()
	return value, err
}

type dashboardPeerID struct {
	nodeID, selfID, name, ip string
	owner                    int64
	port                     int
	key                      [sha256.Size]byte
}

type dashboardPeer struct {
	base      *url.URL
	hold      config.Hold
	leading   bool
	terminals bool
}

var dashboardNetwork discoveryCache[string, tailnet.Status]
var dashboardPeers discoveryCache[dashboardPeerID, dashboardPeer]

func dashboardFleet(ctx context.Context) (config.Config, string, tailnet.Status, error) {
	cfg, err := config.Load()
	if err != nil {
		return cfg, "", tailnet.Status{}, errNotInstalled
	}
	s, err := config.LoadSecrets(cfg)
	if err != nil || s.APIKey == "" {
		return cfg, "", tailnet.Status{}, errors.New("the fleet API key is unavailable")
	}
	st, err := dashboardNetwork.get(ctx, cfg.Tailscale, 4*time.Second, func() (tailnet.Status, error) {
		return (tailnet.CLI{Bin: cfg.Tailscale}).Status(ctx)
	})
	if err != nil || !st.Running || st.Self.Name == "" {
		return cfg, "", st, errors.New("Tailscale is unavailable")
	}
	return cfg, s.APIKey, st, nil
}

func lookupDashboard(ctx context.Context, st tailnet.Status, node tailnet.Node, port int, key string) (dashboardPeer, error) {
	if !st.Running || node.Name == st.Self.Name || !node.Online || node.Expired || !st.SameOwner(node) || node.IP == "" || !tailnet.ValidName(node.Name) || key == "" {
		return dashboardPeer{}, errors.New("peer is unavailable on your Tailscale account")
	}
	id := dashboardPeerID{node.ID, st.Self.ID, node.Name, node.IP, node.UserID, port, sha256.Sum256([]byte(key))}
	return dashboardPeers.get(ctx, id, 10*time.Second, func() (dashboardPeer, error) {
		ctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
		defer cancel()
		base, err := proveFleetNode(ctx, node, port, key)
		if err != nil {
			return dashboardPeer{}, err
		}
		var overview struct {
			Installed bool
			Name      string
			Service   *struct {
				Name    string
				Version string
				Hold    config.Hold
				Leading bool
			}
			Settings struct{ Terminals bool }
		}
		client := api.Client{HTTP: directMachineClient, APIKey: key, MaxResponseBytes: 1 << 20}
		if err := client.Get(ctx, base+webui.BridgePrefix+"/api/overview", &overview); err != nil || !overview.Installed || overview.Name != node.Name || overview.Service == nil || overview.Service.Name != node.Name || !reportedVersion.MatchString(overview.Service.Version) {
			return dashboardPeer{}, errors.New("the verified service did not return its dashboard capabilities")
		}
		address, err := url.Parse(base)
		return dashboardPeer{address, overview.Service.Hold, overview.Service.Leading, overview.Settings.Terminals}, err
	})
}

func (cliBackend) Remote(ctx context.Context, machine string) (webui.Remote, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	cfg, key, st, err := dashboardFleet(ctx)
	if err != nil {
		return webui.Remote{}, err
	}
	var node tailnet.Node
	for _, candidate := range st.Peers {
		if candidate.Name == machine {
			if node.Name != "" {
				return webui.Remote{}, errors.New("the machine name is ambiguous on your Tailnet")
			}
			node = candidate
		}
	}
	if node.Name == "" {
		return webui.Remote{}, errors.New("machine is unavailable on your Tailscale account")
	}
	peer, err := lookupDashboard(ctx, st, node, cfg.Port, key)
	if err != nil {
		return webui.Remote{}, err
	}
	h := make(http.Header)
	h.Set(api.APIKeyHeader, key)
	return webui.Remote{Base: peer.base, Header: h}, nil
}

func (cliBackend) ManagedMachines(ctx context.Context) ([]webui.ManagedMachine, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	cfg, key, st, err := dashboardFleet(ctx)
	if err != nil {
		return nil, err
	}
	return discoverDashboards(ctx, cfg, key, st)
}

func discoverDashboards(ctx context.Context, cfg config.Config, key string, st tailnet.Status) ([]webui.ManagedMachine, error) {
	nodes := st.Machines()
	rows := make([]webui.ManagedMachine, len(nodes))
	nameCounts := make(map[string]int, len(nodes))
	for _, node := range nodes {
		nameCounts[node.Name]++
	}
	work := make(chan int)
	var wg sync.WaitGroup
	for range min(8, len(nodes)) {
		wg.Go(func() {
			for i := range work {
				n := nodes[i]
				m := webui.Machine{Name: n.Name, IP: n.IP, IPv6: n.IPv6, DNSName: n.DNSName, OS: n.OS, Online: n.Online, IsSelf: n.IsSelf, Yours: st.SameOwner(n)}
				if m.IsSelf {
					m.Hold = cfg.Hold
					rows[i] = webui.ManagedMachine{Machine: m, Terminals: cfg.Terminals}
					continue
				}
				if nameCounts[n.Name] != 1 {
					continue
				}
				peer, err := lookupDashboard(ctx, st, n, cfg.Port, key)
				if err != nil {
					continue
				}
				m.Hold, m.Leading = peer.hold, peer.leading
				rows[i] = webui.ManagedMachine{Machine: m, Terminals: peer.terminals}
			}
		})
	}
send:
	for i := range nodes {
		select {
		case work <- i:
		case <-ctx.Done():
			break send
		}
	}
	close(work)
	wg.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return slices.DeleteFunc(rows, func(row webui.ManagedMachine) bool { return row.Name == "" }), nil
}
