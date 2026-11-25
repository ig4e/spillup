package proxy

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestWorkerQuotasReadOnlyAnonymousProviderSignals(t *testing.T) {
	listing := `{"files":[{"name":"private-account","provider":"codex","status":"active","email":"private-email",
		"quota":{"observed_at":"2026-01-02T03:04:05Z","signals":{"X-Codex-Primary-Used-Percent":"0","X-Codex-Primary-Window-Minutes":"300","X-Codex-Primary-Reset-After-Seconds":"900","X-Codex-Secondary-Used-Percent":"44.1","Opaque":"private-header"}}},
		{"provider":"claude","status":"error","unavailable":true,"status_message":"429 private-detail","quota":{"observed_at":"2026-01-02T03:04:05Z","signals":{"anthropic-ratelimit-unified-7d-utilization":"0.53","anthropic-ratelimit-unified-7d-reset":"1769999999","anthropic-ratelimit-unified-5h-utilization":"NaN"}}},
		{"type":"gemini","status":"active"},
		{"provider":"private-provider","status":"error","status_message":"refresh error: private-token"},
		{"provider":"codex","disabled":true,"quota":{"signals":{"X-Codex-Primary-Used-Percent":"97"}}}]}`
	holder := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.RequestURI() != "/v0/management/auth-files" || r.Header.Get("Authorization") != "Bearer private-password" {
			t.Error("unexpected management request")
		}
		_, _ = w.Write([]byte(listing))
	}))
	defer holder.Close()
	_, port, _ := net.SplitHostPort(holder.Listener.Addr().String())
	n, _ := strconv.Atoi(port)
	s, err := ReadQuotas(context.Background(), n, "private-password")
	if err != nil || len(s.Accounts) != 5 || s.CheckedAt.IsZero() {
		t.Fatalf("unexpected summary: %+v, %v", s, err)
	}
	c := s.Accounts[0]
	if c.Provider != "codex" || c.State != "ready" || c.ObservedAt == nil || len(c.Windows) != 2 ||
		c.Windows[0].Label != "5h" || c.Windows[0].UsedPercent != 0 || c.Windows[0].ResetsAt.Unix()-c.ObservedAt.Unix() != 900 || c.Windows[1].UsedPercent != 44.1 {
		t.Errorf("Codex: %+v", c)
	}
	a := s.Accounts[1]
	if a.State != "limited" || len(a.Windows) != 1 || a.Windows[0].Label != "7d" || a.Windows[0].UsedPercent != 53 || a.Windows[0].ResetsAt.Unix() != 1769999999 {
		t.Errorf("Claude: %+v", a)
	}
	if s.Accounts[2].Provider != "gemini" || len(s.Accounts[2].Windows) != 0 || s.Accounts[3].State != "sign-in" || s.Accounts[4].State != "disabled" || len(s.Accounts[4].Windows) != 0 {
		t.Errorf("unsupported or incomplete observations: %+v", s.Accounts[2:])
	}
	wire, err := json.Marshal(s)
	if err != nil || strings.Contains(string(wire), "private-") {
		t.Error("summary included private metadata or invalid JSON")
	}
}

func TestAuthFileReadsNeverFollowManagementRedirects(t *testing.T) {
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("management request followed a redirect")
	}))
	defer redirect.Close()
	holder := httptest.NewServer(http.RedirectHandler(redirect.URL, http.StatusTemporaryRedirect))
	defer holder.Close()
	_, port, _ := net.SplitHostPort(holder.Listener.Addr().String())
	n, _ := strconv.Atoi(port)
	if _, err := ReadQuotas(context.Background(), n, "private-password"); err == nil {
		t.Fatal("redirect accepted")
	}
}

func TestIdleAccountsUseOnlyFixedReadOnlyUpstreams(t *testing.T) {
	tests := []struct {
		provider, url, body string
		status              int
		used                float64
	}{
		{"codex", "https://chatgpt.com/backend-api/wham/usage", `{"rate_limit":{"primary_window":{"used_percent":11.4,"limit_window_seconds":18000,"reset_at":1789999999}}}`, 200, 11.4},
		{"claude", "https://api.anthropic.com/api/oauth/usage", `{"five_hour":{"utilization":0,"resets_at":"2026-11-15T00:00:00Z"}}`, 200, 0},
		{"codex", "https://chatgpt.com/backend-api/wham/usage", "private upstream error", 401, 0},
	}
	for _, tc := range tests {
		t.Run(tc.provider+strconv.Itoa(tc.status), func(t *testing.T) {
			var calls int
			holder := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer management-only" {
					t.Error("missing local management credential")
				}
				if r.URL.Path == "/v0/management/auth-files" {
					_, _ = w.Write([]byte(`{"files":[{"type":"` + tc.provider + `","status":"active","auth_index":"opaque","id_token":{"chatgpt_account_id":"account-id"},"url":"http://ignored.invalid"}]}`))
					return
				}
				calls++
				var req struct {
					Method    string
					URL       string
					Header    map[string]string
					AuthIndex string `json:"auth_index"`
				}
				if r.Method != "POST" || r.URL.Path != "/v0/management/api-call" || json.NewDecoder(r.Body).Decode(&req) != nil || req.Method != "GET" || req.URL != tc.url || req.AuthIndex != "opaque" || req.Header["Authorization"] != "Bearer $TOKEN$" {
					t.Error("unexpected upstream request")
				}
				if tc.provider == "codex" && req.Header["Chatgpt-Account-Id"] != "account-id" {
					t.Error("missing tenant selection")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"status_code": tc.status, "body": tc.body})
			}))
			defer holder.Close()
			_, port, _ := net.SplitHostPort(holder.Listener.Addr().String())
			n, _ := strconv.Atoi(port)
			s, err := ReadQuotas(context.Background(), n, "management-only")
			if err != nil || calls != 1 || len(s.Accounts) != 1 {
				t.Fatalf("calls %d: %+v %v", calls, s, err)
			}
			a := s.Accounts[0]
			if tc.status != 200 {
				if len(a.Windows) != 0 || a.UsageStatus != "refused" {
					t.Fatalf("upstream refusal: %+v", a)
				}
				return
			}
			if len(a.Windows) != 1 || a.Windows[0].Label != "5h" || a.Windows[0].UsedPercent != tc.used || a.Windows[0].ResetsAt == nil || a.ObservedAt == nil {
				t.Fatalf("usage: %+v", a)
			}
			wire, _ := json.Marshal(s)
			if strings.Contains(string(wire), "opaque") || strings.Contains(string(wire), "account-id") {
				t.Fatal("credential identity leaked")
			}
		})
	}
}
