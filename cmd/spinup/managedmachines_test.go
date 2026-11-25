package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/tailnet"
	"github.com/darkyeg/spinup/internal/webui"
)

func TestManagedMachinesVerifyServiceBeforeSendingAKeyAndKeepWorkers(t *testing.T) {
	const key = "test-common-key"
	var challenges, overviews atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(api.KeyHeader) != "" {
			t.Error("management credential on discovery request")
		}
		switch r.URL.Path {
		case api.PathLeader:
			challenges.Add(1)
			if r.Header.Get(api.APIKeyHeader) != "" || r.URL.Query().Get(api.NonceParam) == "" {
				t.Error("expected public, secret-free challenge")
			}
			l := api.Leader{Name: "worker", Hold: config.HoldNever}
			json.NewEncoder(w).Encode(l.Prove(config.Secrets{APIKey: key}, r.URL.Query().Get(api.NonceParam)))
		case webui.BridgePrefix + "/api/overview":
			overviews.Add(1)
			if r.Header.Get(api.APIKeyHeader) != key {
				t.Error("missing authenticated overview key")
			}
			json.NewEncoder(w).Encode(webui.Overview{Installed: true, Name: "worker", Hold: config.HoldNever, Service: &api.Report{Name: "worker", Version: "v1.0.0", Hold: config.HoldNever}, Settings: webui.Settings{Terminals: false}})
		default:
			t.Errorf("unexpected request path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	addr := server.Listener.Addr().(*net.TCPAddr)
	cfg := config.Config{Port: addr.Port, Hold: config.HoldHub, Terminals: true}
	node := tailnet.Node{ID: "first-node", Name: "worker", OS: "android", IP: addr.IP.String(), Online: true, UserID: 1}
	st := tailnet.Status{Running: true, Self: tailnet.Node{ID: "here", IsSelf: true, Name: "here", IP: "100.64.0.1", OS: "linux", Online: true, UserID: 1}, Peers: []tailnet.Node{
		node,
		{ID: "phone", Name: "plain-phone", OS: "iOS", IP: node.IP, Online: true, UserID: 1},
		{Name: "outside", OS: "windows", IP: node.IP, Online: true, UserID: 2},
		{Name: "tagged", IP: node.IP, Online: true, UserID: 1, Tagged: true},
		{ID: "collision", Name: "here", IP: node.IP, Online: true, UserID: 1},
		{Name: "same-name", IP: node.IP, Online: true, UserID: 1},
		{Name: "same-name", IP: node.IP, Online: true, UserID: 1},
		{Name: "expired", IP: node.IP, Online: true, UserID: 1, Expired: true},
		{Name: "offline", IP: node.IP, Online: false, UserID: 1},
	}}
	for range 2 {
		nodes, err := discoverDashboards(t.Context(), cfg, key, st)
		if err != nil || len(nodes) != 2 || nodes[0].Name != "here" || !nodes[0].IsSelf || !nodes[0].Terminals || nodes[1].Name != "worker" || nodes[1].Hold != config.HoldNever || nodes[1].Terminals {
			t.Fatalf("verified machines = %+v, err %v", nodes, err)
		}
	}
	if challenges.Load() != 2 || overviews.Load() != 1 {
		t.Fatalf("probe repetition or key sent to ordinary node: challenges=%d overviews=%d", challenges.Load(), overviews.Load())
	}
	if _, err := lookupDashboard(t.Context(), st, node, cfg.Port, "rotated-key"); err == nil || challenges.Load() != 3 || overviews.Load() != 1 {
		t.Fatalf("old proof accepted after key rotation: %v", err)
	}
	st.Peers[0].ID = "replacement-node"
	if _, err := lookupDashboard(t.Context(), st, st.Peers[0], cfg.Port, key); err != nil || challenges.Load() != 4 || overviews.Load() != 2 {
		t.Fatalf("stable node ID change did not trigger new proof: %v", err)
	}
}

func TestDiscoveryCoalescesRequestsAndWaitRespectsContext(t *testing.T) {
	var c discoveryCache[string, int]
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	read := func() (int, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return 42, nil
	}
	var wg sync.WaitGroup
	get := func() {
		v, err := c.get(t.Context(), "worker", time.Minute, read)
		if err != nil || v != 42 {
			t.Errorf("unexpected read %d %v", v, err)
		}
	}
	wg.Go(get)
	<-started
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := c.get(cancelled, "worker", time.Minute, read); err != context.Canceled {
		t.Fatalf("cancelled waiter: %v", err)
	}
	for range 6 {
		wg.Go(get)
	}
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Errorf("%d discovery calls", calls.Load())
	}
}
