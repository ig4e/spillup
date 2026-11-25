package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/leadership"
	"github.com/darkyeg/spinup/internal/tailnet"
)

func TestRolloutChecksOwnershipAndProofBeforeSendingAKey(t *testing.T) {
	s := config.Secrets{APIKey: "test-api"}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get(api.APIKeyHeader) != "" || r.Header.Get(api.KeyHeader) != "" {
			t.Error("challenge sent a secret")
		}
		l := api.Leader{Name: "worker", Hold: config.HoldNever}
		json.NewEncoder(w).Encode(l.Prove(s, r.URL.Query().Get(api.NonceParam)))
	}))
	defer srv.Close()
	addr := srv.Listener.Addr().(*net.TCPAddr)
	fleet := rolloutFleet{config.Config{Port: addr.Port}, s.APIKey}
	ts := tailnet.Status{Running: true, Self: tailnet.Node{Name: "hub", UserID: 1}, Peers: []tailnet.Node{{Name: "worker", UserID: 2, Online: true, IP: addr.IP.String()}}}
	if _, err := verifiedFleetPeer(t.Context(), ts, "worker", fleet.cfg.Port, fleet.key); err == nil || calls != 0 {
		t.Fatal("contacted another owner's device")
	}
	ts.Peers[0].UserID = 1
	if base, err := verifiedFleetPeer(t.Context(), ts, "worker", fleet.cfg.Port, fleet.key); err != nil || base != srv.URL {
		t.Fatalf("owned worker: %v %s", err, base)
	}
	fleet.key = "wrong"
	if _, err := verifiedFleetPeer(t.Context(), ts, "worker", fleet.cfg.Port, fleet.key); err == nil {
		t.Error("accepted unproven device")
	}
}

func TestRolloutLeavesTheAccountHolderRunning(t *testing.T) {
	f := rolloutFleet{}
	leader := api.Report{Name: "hub", Hold: config.HoldHub, Leading: true, ProxyRunning: true, Epoch: 4}
	if err := f.guard(t.Context(), leader); err == nil {
		t.Error("would stage active holder")
	}
	if err := f.healthy(t.Context(), leader); err != nil {
		t.Fatal(err)
	}
	if err := autoUpdateHealth(leader); err == nil {
		t.Error("would restart a sole holder automatically")
	}
	if err := f.healthy(t.Context(), api.Report{Hold: config.HoldStandby, Epoch: 1, Synced: &api.Sync{Epoch: 0}}); err == nil {
		t.Fatal("accepted stale standby")
	}
	leader.Peers = []leadership.Peer{{Hold: config.HoldStandby, State: leadership.Synced, Epoch: 4}}
	if err := autoUpdateHealth(leader); err != nil {
		t.Error(err)
	}
	leader.HubOnly = true
	if err := autoUpdateHealth(leader); err == nil {
		t.Error("would restart in hub-only mode")
	}
	if !(rolloutOrder(api.Report{Hold: config.HoldNever}) < rolloutOrder(api.Report{Hold: config.HoldStandby}) && rolloutOrder(api.Report{Hold: config.HoldStandby}) < rolloutOrder(leader)) {
		t.Error("wrong stage order")
	}
	if _, err := rolloutReady(context.Background(), "1.2.3/other"); err == nil {
		t.Error("accepted invalid release")
	}
}
func TestStatusPollingReusesLoopbackConnections(t *testing.T) {
	peers := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peers[r.RemoteAddr] = true
		json.NewEncoder(w).Encode(api.Report{Name: "worker"})
	}))
	defer srv.Close()
	_, p, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(p)
	for range 30 {
		if _, err := localService(config.Config{Port: port}, "").report(); err != nil {
			t.Fatal(err)
		}
	}
	srv.Close()
	if len(peers) > 2 {
		t.Fatalf("30 polls consumed %d loopback ports", len(peers))
	}
	tr := directMachineClient.Transport.(*http.Transport)
	if tr.IdleConnTimeout <= 0 || tr.IdleConnTimeout > 2*time.Minute {
		t.Error("idle sockets have no short release deadline")
	}
}

func TestRolloutRecoveryRestoresPreviousBinary(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "spinup")
	if err := os.WriteFile(exe, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := replaceRunningBinary(exe, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := restoreRolloutBinary(exe); err != nil {
		t.Fatal(err)
	}
	if err := restoreRolloutBinary(exe); err == nil {
		t.Fatal("restored from nonexistent backup")
	}
	b, err := os.ReadFile(exe)
	if err != nil || string(b) != "old" {
		t.Fatal("original executable missing", err)
	}
}
