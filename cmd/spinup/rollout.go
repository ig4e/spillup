package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/filelock"
	"github.com/darkyeg/spinup/internal/release"
	"github.com/darkyeg/spinup/internal/tailnet"
	"github.com/darkyeg/spinup/internal/webui"
)

type rolloutCmd struct {
	Apply bool `help:"Install on one follower at a time from the configured hub. The active account holder stays in service."`
}

type rolloutFleet struct {
	cfg config.Config
	key string
}

type rolloutNode struct {
	name string
	rep  api.Report
}

func (f rolloutFleet) report(ctx context.Context, name string) (api.Report, error) {
	ctx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	st, err := (tailnet.CLI{Bin: f.cfg.Tailscale}).Status(ctx)
	if err != nil || !st.Running || !st.Self.Online {
		return api.Report{}, errors.New("Tailscale is unavailable")
	}
	var r api.Report
	client := api.Client{HTTP: directMachineClient, MaxResponseBytes: 1 << 20}
	if name == st.Self.Name {
		err = client.Get(ctx, localService(f.cfg, "").base+api.PathState, &r)
	} else {
		base, err := verifiedFleetPeer(ctx, st, name, f.cfg.Port, f.key)
		if err != nil {
			return r, err
		}
		client.APIKey = f.key
		var o webui.Overview
		if err := client.Get(ctx, base+webui.BridgePrefix+"/api/overview", &o); err != nil {
			return r, errors.New("the remote dashboard did not return a verified overview")
		}
		if o.Service == nil {
			return r, errors.New("service status unavailable")
		}
		r = *o.Service
	}
	if err != nil {
		return r, err
	}
	if r.Name != name || !reportedVersion.MatchString(r.Version) {
		return r, errors.New("service identity or version unavailable")
	}
	return r, nil
}

func verifiedFleetPeer(ctx context.Context, st tailnet.Status, name string, port int, key string) (string, error) {
	node, ok := st.Peer(name)
	if !st.Running || !ok || !node.Online || node.Expired || !st.SameOwner(node) || node.IP == "" {
		return "", errors.New("peer is unavailable on your Tailscale account")
	}
	base := "http://" + net.JoinHostPort(node.IP, strconv.Itoa(port))
	client := api.Client{HTTP: directMachineClient, MaxResponseBytes: 1 << 20}
	_, proof, err := client.AskLeader(ctx, base, name, config.Secrets{APIKey: key})
	if err != nil || proof != api.KnowsAPIKey {
		return "", errors.New("no verified Spillup service (older workers need a local update)")
	}
	return base, nil
}

func (f rolloutFleet) guard(ctx context.Context, r api.Report) error {
	if r.Starting || r.Leading {
		return errors.New("active account holder deferred")
	}
	if r.Leader == "" || r.Leader == r.Name {
		return errors.New("no other account holder is confirmed")
	}
	leader, err := f.report(ctx, r.Leader)
	if err != nil || !leader.Leading || leader.Starting || !leader.ProxyRunning {
		return errors.New("the other account holder is not serving")
	}
	return nil
}

func (f rolloutFleet) stage(ctx context.Context, name, target string) error {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	st, err := (tailnet.CLI{Bin: f.cfg.Tailscale}).Status(ctx)
	if err != nil {
		return err
	}
	if name == st.Self.Name {
		return (rolloutInstallCmd{Version: target}).Run()
	}
	base, err := verifiedFleetPeer(ctx, st, name, f.cfg.Port, f.key)
	if err != nil {
		return err
	}
	return (api.Client{HTTP: directMachineClient, APIKey: f.key}).Post(ctx, base+webui.BridgePrefix+"/api/update/rollout", webui.RolloutRequest{Version: target}, nil)
}

func (c rolloutCmd) Run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if c.Apply {
		if cfg.Hold != config.HoldHub {
			return errors.New("start the fleet rollout on the configured hub")
		}
		lockCtx, stop := context.WithTimeout(ctx, time.Second)
		lock, err := filelock.Acquire(lockCtx, filepath.Join(config.StateDir(), "rollout.lock"))
		stop()
		if err != nil {
			return fmt.Errorf("acquire fleet rollout lock: %w", err)
		}
		defer lock.Close()
	}
	s, err := config.LoadSecrets(cfg)
	if err != nil {
		return err
	}
	f := rolloutFleet{cfg, s.APIKey}
	rel, err := release.Latest(ctx, releases)
	if err != nil {
		return err
	}
	if !stableVersion.MatchString(rel.Version) {
		return fmt.Errorf("invalid stable release %q", rel.Version)
	}
	st, err := (tailnet.CLI{Bin: cfg.Tailscale}).Status(ctx)
	if err != nil {
		return err
	}
	if !st.Running || !st.Self.Online {
		return errors.New("Tailscale is unavailable")
	}
	fmt.Printf("Fleet rollout to v%s\n", rel.Version)
	var nodes []rolloutNode
	var skipped []string
	for _, n := range st.Machines() {
		if !n.IsSelf && !st.SameOwner(n) {
			continue
		}
		if !n.Online || n.Expired {
			fmt.Printf("  %s: offline or expired; skipped\n", n.Name)
			skipped = append(skipped, n.Name)
			continue
		}
		r, err := f.report(ctx, n.Name)
		if err != nil {
			fmt.Printf("  %s: %v; skipped\n", n.Name, err)
			skipped = append(skipped, n.Name)
			continue
		}
		nodes = append(nodes, rolloutNode{n.Name, r})
	}
	if len(nodes) == 0 {
		return errors.New("no verified Spillup services answered")
	}
	sort.SliceStable(nodes, func(i, j int) bool { return rolloutOrder(nodes[i].rep) < rolloutOrder(nodes[j].rep) })
	var deferred []string
	for i, n := range nodes {
		fmt.Printf("[%d/%d] %s (%s)  ", i+1, len(nodes), n.name, n.rep.Version)
		if !release.Newer(rel.Version, n.rep.Version) {
			if !sameRelease(rel.Version, n.rep.Version) {
				fmt.Println("outside the upgrade path; skipped")
				deferred = append(deferred, n.name)
				continue
			}
			if err := f.healthy(ctx, n.rep); err != nil {
				fmt.Println(err)
				if c.Apply {
					return fmt.Errorf("%s: %w", n.name, err)
				}
				continue
			}
			fmt.Println("verified on the target")
			continue
		}
		if err := f.guard(ctx, n.rep); err != nil {
			fmt.Println(err)
			deferred = append(deferred, n.name)
			continue
		}
		if !n.rep.Rollout && n.name != st.Self.Name {
			fmt.Println("one local update needed to enable staged installs")
			deferred = append(deferred, n.name)
			continue
		}
		if !c.Apply {
			fmt.Println("ready to update")
			continue
		}
		fmt.Println("updating")
		if err := f.stage(ctx, n.name, rel.Version); err != nil {
			return fmt.Errorf("%s: %w", n.name, err)
		}
		if err := f.wait(ctx, n.name, rel.Version); err != nil {
			return err
		}
		fmt.Printf("  %s verified on v%s\n", n.name, rel.Version)
	}
	if !c.Apply {
		fmt.Println("Run spinup rollout --apply on the hub to update ready machines.")
		return nil
	}
	if len(skipped) > 0 {
		fmt.Printf("Unverified Tailnet devices: %s\n", strings.Join(skipped, ", "))
	}
	if len(deferred) > 0 {
		return fmt.Errorf("deferred %s; rerun after a safe handoff or update the sole holder during a maintenance window", strings.Join(deferred, ", "))
	}
	return nil
}

func sameRelease(a, b string) bool { return strings.TrimPrefix(a, "v") == strings.TrimPrefix(b, "v") }

func rolloutOrder(r api.Report) int {
	if r.Starting || r.Leading {
		return 2
	}
	if r.Hold.CanHold() {
		return 1
	}
	return 0
}

func (f rolloutFleet) wait(ctx context.Context, name, target string) error {
	ctx, cancel := context.WithTimeout(ctx, 9*time.Minute)
	defer cancel()
	for ctx.Err() == nil {
		r, err := f.report(ctx, name)
		if err == nil && sameRelease(r.Version, target) {
			if err := f.healthy(ctx, r); err == nil {
				return nil
			}
		}
		sleepCtx(ctx, 3*time.Second)
	}
	return fmt.Errorf("%s has not confirmed a healthy v%s: %w; inspect rollout.log there before retrying", name, target, ctx.Err())
}

func (f rolloutFleet) healthy(ctx context.Context, r api.Report) error {
	if r.Leading && r.ProxyRunning && !r.Starting {
		return nil
	}
	if r.Hold.CanHold() && !r.HubOnly && !r.Synced.Fresh(r.Epoch) {
		return errors.New("standby is waiting for a fresh account copy")
	}
	return f.guard(ctx, r)
}
