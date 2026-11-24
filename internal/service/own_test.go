package service

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
)

func ask(addr, path, body string) string {
	resp, err := http.Post("http://"+addr+path, "application/json", strings.NewReader(body))
	if err != nil {
		return "error: " + err.Error()
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return string(out)
}

const (
	claudeRequest = `{"model":"claude-sonnet-4-5","messages":[]}`
	gptRequest    = `{"model":"gpt-5-codex","input":"hi"}`
)

func TestOwnLoginsServeOnlyThisMachinesOwnRequests(t *testing.T) {
	tn := newTestTailnet(t)
	hub := tn.add("hub", config.HoldHub)
	hub.own = []string{"claude"}
	laptop := tn.add("laptop", config.HoldNever)
	laptop.own = []string{"claude"}
	tn.start(hub)
	tn.waitFor("the hub leads", tn.leaderIs("hub"))
	tn.start(laptop)
	tn.waitFor("the laptop's own proxy runs", laptop.ownProxy.Running)
	tn.waitFor("the laptop reaches the hub", func() bool { return strings.Contains(ask(laptop.front, "/v1/responses", gptRequest), "on hub") })

	if got := ask(laptop.front, "/v1/messages", claudeRequest); !strings.Contains(got, "on laptop-own") {
		t.Errorf("a Claude request on the laptop went to %q, want its own login", got)
	}
	if got := ask(laptop.front, "/v1/responses", gptRequest); !strings.Contains(got, "on hub") || strings.Contains(got, "own") {
		t.Errorf("a GPT request on the laptop went to %q, want the shared accounts", got)
	}
	if got := ask(hub.front, "/v1/messages", claudeRequest); !strings.Contains(got, "on hub-own") {
		t.Errorf("a Claude request on the hub went to %q, want the hub's own login", got)
	}
	if got := ask(hub.peerAPI, "/v1/messages", claudeRequest); !strings.Contains(got, "on hub") || strings.Contains(got, "own") {
		t.Errorf("another machine's Claude request went to %q: own logins never serve other machines", got)
	}

	req, _ := http.NewRequest(http.MethodPost, "http://"+laptop.front+api.PathOwn, strings.NewReader(`{"providers":[]}`))
	req.Header.Set(api.APIKeyHeader, "k")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("turning own logins off: %v %v", resp, err)
	}
	resp.Body.Close()
	if laptop.ownProxy.Running() {
		t.Error("the own proxy still runs after turning own logins off")
	}
	if got := ask(laptop.front, "/v1/messages", claudeRequest); !strings.Contains(got, "on hub") || strings.Contains(got, "own") {
		t.Errorf("with own logins off, a Claude request went to %q, want the shared accounts", got)
	}

	tn.shutDown(laptop)
	tn.shutDown(hub)
	if most := tn.most.Load(); most != 1 {
		t.Fatalf("%d shared proxies ran at once; never more than 1", most)
	}
}

func TestOwnLoginsRefuseUnknownFamiliesAndNeedTheAPIKey(t *testing.T) {
	tn := newTestTailnet(t)
	laptop := tn.add("laptop", config.HoldNever)
	tn.start(laptop)
	tn.waitFor("the laptop answers", func() bool { return !strings.HasPrefix(get(laptop.front, api.PathLeader), "error") })
	for _, c := range []struct {
		key, body string
		want      int
	}{
		{"", `{"providers":["claude"]}`, http.StatusUnauthorized},
		{"k", `{"providers":["../bad"]}`, http.StatusConflict},
	} {
		req, _ := http.NewRequest(http.MethodPost, "http://"+laptop.front+api.PathOwn, strings.NewReader(c.body))
		req.Header.Set(api.APIKeyHeader, c.key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != c.want {
			t.Errorf("key %q body %s: %d, want %d", c.key, c.body, resp.StatusCode, c.want)
		}
	}
	if got := laptop.m.Report().Own; len(got) != 0 {
		t.Errorf("a refused change left own logins %v", got)
	}
	tn.shutDown(laptop)
}

func TestAnOldKeyBecomesTheSharedOneOnThisMachineOnly(t *testing.T) {
	m := &Machine{cfg: config.Config{LegacyKeys: []string{"old"}}, o: Options{Secrets: config.Secrets{APIKey: "shared"}}}
	r, _ := http.NewRequest(http.MethodPost, "/v1/messages", nil)
	r.Header.Set("Authorization", "Bearer old")
	r.Header.Set("X-Api-Key", "old")
	r.Header.Set("X-Goog-Api-Key", "someone-else")
	m.swapLegacyKey(r)
	if got := r.Header.Get("Authorization"); got != "Bearer shared" {
		t.Errorf("Authorization %q, want the shared key", got)
	}
	if got := r.Header.Get("X-Api-Key"); got != "shared" {
		t.Errorf("X-Api-Key %q, want the shared key", got)
	}
	if got := r.Header.Get("X-Goog-Api-Key"); got != "someone-else" {
		t.Errorf("a key that isn't an old one was changed to %q", got)
	}
}

func TestAnOldManagementKeyBecomesTheDashboardPasswordOnManagementPaths(t *testing.T) {
	m := &Machine{cfg: config.Config{LegacyManagementKeys: []string{"old-mgmt"}, LegacyKeys: []string{"old"}},
		o: Options{Secrets: config.Secrets{APIKey: "shared", ManagementPassword: "pw"}}}
	r, _ := http.NewRequest(http.MethodGet, "/v0/management/auth-files", nil)
	r.Header.Set("Authorization", "Bearer old-mgmt")
	m.swapLegacyKey(r)
	if got := r.Header.Get("Authorization"); got != "Bearer pw" {
		t.Errorf("management call carries %q, want the dashboard password", got)
	}
	api, _ := http.NewRequest(http.MethodPost, "/v1/messages", nil)
	api.Header.Set("Authorization", "Bearer old-mgmt")
	m.swapLegacyKey(api)
	if got := api.Header.Get("Authorization"); got != "Bearer old-mgmt" {
		t.Errorf("an API call must not get the dashboard password: %q", got)
	}
}
