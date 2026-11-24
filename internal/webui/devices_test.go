package webui

import (
	"context"
	"net/http"
	"testing"

	"github.com/darkyeg/spinup/internal/tailnet"
)

type tailnetBackend struct {
	*fakeBackend
	status tailnet.Status
}

func (b tailnetBackend) Tailnet(context.Context) (tailnet.Status, error) { return b.status, nil }

func TestYourDevicesGetTheDashboardOverTheTailnet(t *testing.T) {
	b := tailnetBackend{&fakeBackend{}, tailnet.Status{
		Self: tailnet.Node{Name: "desk", DNSName: "desk.tail1.ts.net.", IP: "100.64.0.1", UserID: 7},
		Peers: []tailnet.Node{
			{Name: "phone", IP: "100.64.0.2", UserID: 7},
			{Name: "friend", IP: "100.64.0.3", UserID: 9},
			{Name: "server", IP: "100.64.0.4", UserID: 7, Tagged: true},
		},
	}}
	h := New(b, testToken).DeviceHandler()
	from := func(host, ip string) func(*http.Request) {
		return func(r *http.Request) { r.Host, r.RemoteAddr = host, ip+":5000" }
	}

	for _, host := range []string{"desk:8320", "desk.tail1.ts.net:8320", "100.64.0.1:8320"} {
		if w := do(t, h, http.MethodGet, "/api/overview", from(host, "100.64.0.2")); w.Code != http.StatusOK {
			t.Errorf("your phone at %s: %d, want 200", host, w.Code)
		}
	}
	for name, ip := range map[string]string{"someone else's device": "100.64.0.3", "a tagged server": "100.64.0.4", "a LAN address": "192.168.1.9"} {
		if w := do(t, h, http.MethodGet, "/api/overview", from("desk:8320", ip)); w.Code != http.StatusForbidden {
			t.Errorf("%s: %d, want 403", name, w.Code)
		}
	}
	if w := do(t, h, http.MethodGet, "/api/overview", from("evil.example:8320", "100.64.0.2")); w.Code != http.StatusMisdirectedRequest {
		t.Errorf("another site's name for this address: %d, want 421", w.Code)
	}
	if w := do(t, h, http.MethodGet, "/api/keys", from("desk:8320", "100.64.0.2")); w.Code == http.StatusOK {
		t.Error("the keys must stay on this machine")
	}
	cross := func(r *http.Request) {
		from("desk:8320", "100.64.0.2")(r)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "http://evil.example")
	}
	if w := do(t, h, http.MethodPost, "/api/machines/main", cross); w.Code != http.StatusForbidden {
		t.Errorf("a change from another site: %d, want 403", w.Code)
	}
}
