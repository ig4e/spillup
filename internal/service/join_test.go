package service

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/tailnet"
)

func TestOnlyYourOwnDevicesJoinByThemselves(t *testing.T) {
	cfg := config.Defaults()
	cfg.Hold = config.HoldHub
	m := New(Options{Config: cfg, Secrets: config.Secrets{APIKey: "k", ManagementPassword: "m"}, Log: log.New(io.Discard, "", 0),
		StatePath: t.TempDir() + "/state.json"})
	m.tail.set(tailnet.Status{
		Self: tailnet.Node{Name: "desk", IP: "100.64.0.1", UserID: 7},
		Peers: []tailnet.Node{
			{Name: "laptop", IP: "100.64.0.2", UserID: 7},
			{Name: "friend", IP: "100.64.0.3", UserID: 9},
			{Name: "server", IP: "100.64.0.4", UserID: 7, Tagged: true},
		},
	})
	join := func(ip, hold string) (int, config.Secrets) {
		r := httptest.NewRequest(http.MethodPost, api.PathJoin, strings.NewReader(`{"hold":"`+hold+`"}`))
		r.RemoteAddr = ip + ":51000"
		w := httptest.NewRecorder()
		m.routes(onTailnet).ServeHTTP(w, r)
		var s config.Secrets
		_ = json.Unmarshal(w.Body.Bytes(), &s)
		return w.Code, s
	}
	if code, s := join("100.64.0.2", "standby"); code != http.StatusOK || s.APIKey != "k" || s.ManagementPassword != "m" {
		t.Errorf("your laptop joining as a standby: %d %+v", code, s)
	}
	if code, s := join("100.64.0.2", "never"); code != http.StatusOK || s.ManagementPassword != "" {
		t.Errorf("a machine that only uses the accounts got the password: %d %+v", code, s)
	}
	for _, ip := range []string{"100.64.0.3", "100.64.0.4", "100.64.0.99", "127.0.0.1"} {
		if code, s := join(ip, "standby"); code != http.StatusForbidden || s.APIKey != "" {
			t.Errorf("%s joining: %d %+v, want 403 and no keys", ip, code, s)
		}
	}
	r := httptest.NewRequest(http.MethodPost, api.PathJoin, strings.NewReader(`{"hold":"standby"}`))
	r.RemoteAddr = "100.64.0.2:51000"
	w := httptest.NewRecorder()
	m.routes(onLocalhost).ServeHTTP(w, r)
	if w.Code == http.StatusOK {
		t.Error("joining must be on the tailnet only")
	}
}

func TestYourWorkersOpenTheMainMachinesDashboardAndNobodyElse(t *testing.T) {
	cfg := config.Defaults()
	cfg.Hold = config.HoldHub
	m := New(Options{Config: cfg, Secrets: config.Secrets{APIKey: "k", ManagementPassword: "m"}, Log: log.New(io.Discard, "", 0),
		StatePath: t.TempDir() + "/state.json", Dashboard: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ok") })})
	m.tail.set(tailnet.Status{
		Self:  tailnet.Node{Name: "desk", IP: "100.64.0.1", UserID: 7},
		Peers: []tailnet.Node{{Name: "laptop", IP: "100.64.0.2", UserID: 7}, {Name: "friend", IP: "100.64.0.3", UserID: 9}},
	})
	call := func(ip, header, key string) int {
		r := httptest.NewRequest(http.MethodGet, "/spinup/ui/api/overview", nil)
		r.RemoteAddr = ip + ":51000"
		r.Header.Set(header, key)
		w := httptest.NewRecorder()
		m.routes(onTailnet).ServeHTTP(w, r)
		return w.Code
	}
	if got := call("100.64.0.2", api.APIKeyHeader, "k"); got != http.StatusOK {
		t.Errorf("your worker with the API key: %d, want 200", got)
	}
	if got := call("100.64.0.3", api.APIKeyHeader, "k"); got != http.StatusUnauthorized {
		t.Errorf("someone else's device with the API key: %d, want 401", got)
	}
	if got := call("100.64.0.3", api.KeyHeader, "m"); got != http.StatusOK {
		t.Errorf("the dashboard password: %d, want 200", got)
	}
	if got := call("100.64.0.2", api.APIKeyHeader, "wrong"); got != http.StatusUnauthorized {
		t.Errorf("a wrong key: %d, want 401", got)
	}
}
