package service

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/tailnet"
)

type quotaProxy struct{ Proxy }

func (quotaProxy) Running() bool { return true }

func TestQuotaViewRunsThroughOwnerAndKeyChecksAndNeverRelaysTheManagementPassword(t *testing.T) {
	var reads atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		if r.Header.Get("Authorization") != "Bearer holder-only" {
			t.Error("expected the holder's management key at its local proxy")
		}
		fmt.Fprint(w, `{"files":[{"provider":"codex","status":"active","name":"private-login"}]}`)
	}))
	defer source.Close()
	cfg := config.Defaults()
	_, port, _ := net.SplitHostPort(source.Listener.Addr().String())
	cfg.ProxyPort, _ = strconv.Atoi(port)
	hub := New(Options{Config: cfg, StatePath: t.TempDir() + "/state", Proxy: quotaProxy{}, Secrets: config.Secrets{APIKey: "fleet", ManagementPassword: "holder-only"}})
	hub.tail.set(tailnet.Status{Self: tailnet.Node{Name: "hub", UserID: 7}, Peers: []tailnet.Node{
		{Name: "worker", IP: "100.64.0.2", UserID: 7}, {Name: "other", IP: "100.64.0.3", UserID: 8},
	}})
	hub.ledger.beginLeading(1, "hub")
	hub.ledger.finishLeading()
	peer := hub.routes(onTailnet)
	call := func(h http.Handler, method, source, key string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, api.PathQuotas, nil)
		r.RemoteAddr = source + ":1234"
		r.Header.Set(api.APIKeyHeader, key)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct {
		method, source, key string
		want                int
	}{
		{"GET", "100.64.0.2", "", 401}, {"GET", "100.64.0.3", "fleet", 403},
		{"POST", "100.64.0.2", "fleet", 404},
	} {
		if got := call(peer, tc.method, tc.source, tc.key).Code; got != tc.want {
			t.Errorf("%s %s: %d, want %d", tc.method, tc.source, got, tc.want)
		}
	}
	if reads.Load() != 0 {
		t.Fatal("unauthorized requests reached the proxy")
	}
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(api.KeyHeader) != "" || r.Header.Get("Authorization") != "" {
			t.Error("worker sent a privileged credential")
		}
		r.RemoteAddr = "100.64.0.2:1234"
		peer.ServeHTTP(w, r)
	}))
	defer endpoint.Close()
	cfg.Hold = config.HoldNever
	worker := New(Options{Config: cfg, StatePath: t.TempDir() + "/state", Secrets: config.Secrets{APIKey: "fleet"}, PeerAddr: func(string, string) string { return endpoint.Listener.Addr().String() }})
	worker.tail.set(tailnet.Status{Self: tailnet.Node{Name: "worker", UserID: 7}, Peers: []tailnet.Node{{Name: "hub", IP: "100.64.0.1", UserID: 7}}})
	worker.notes.routeTo("hub", endpoint.Listener.Addr().String())
	for range 2 {
		w := call(worker.routes(onLocalhost), "GET", "127.0.0.1", "fleet")
		if w.Code != 200 || !strings.Contains(w.Body.String(), "codex") || strings.Contains(w.Body.String(), "private-login") {
			t.Fatalf("worker quota view: %d %s", w.Code, w.Body.String())
		}
	}
	if reads.Load() != 1 {
		t.Fatalf("dashboard reads reached management %d times", reads.Load())
	}
	worker.tail.set(tailnet.Status{Self: tailnet.Node{Name: "worker", UserID: 7}, Peers: []tailnet.Node{{Name: "hub", IP: "100.64.0.1", UserID: 8}}})
	if got := call(worker.routes(onLocalhost), "GET", "127.0.0.1", "fleet").Code; got != 503 {
		t.Errorf("worker still trusts previous holder: %d", got)
	}
}
