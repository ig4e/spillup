package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/t3code"
)

func TestDirectQuotaPortRequiresTheVerifiedServingProxy(t *testing.T) {
	secret := config.Secrets{APIKey: "test-api", ManagementPassword: "test-management"}
	for _, tc := range []struct {
		name                     string
		leading, running, proven bool
		managed                  int
		wantDirect               bool
	}{
		{"serving", true, true, true, 8327, true},
		{"inactive", false, true, true, 8327, false},
		{"stopped", true, false, true, 8327, false},
		{"unverified", true, true, false, 8327, false},
		{"old configuration", true, true, true, 9999, false},
		{"older service", true, true, true, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get(api.KeyHeader) != "" || r.Header.Get(api.APIKeyHeader) != "" || r.Header.Get("Authorization") != "" {
					t.Error("credentials sent to proof endpoint")
				}
				switch r.URL.Path {
				case api.PathState:
					json.NewEncoder(w).Encode(api.Report{Name: "test-hub", Hold: config.HoldHub, Leading: tc.leading, ProxyRunning: tc.running, ManagedProxyPort: tc.managed})
				case api.PathLeader:
					leader := api.Leader{Name: "test-hub", Hold: config.HoldHub, Leading: tc.leading}
					if tc.proven {
						leader = leader.Prove(secret, r.URL.Query().Get(api.NonceParam))
					}
					json.NewEncoder(w).Encode(leader)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			u, err := url.Parse(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			port, err := strconv.Atoi(u.Port())
			if err != nil {
				t.Fatal(err)
			}
			cfg := config.Config{Hold: config.HoldHub, Port: port, ProxyPort: 8327, OwnProxyPort: 8328, Tailscale: filepath.Join(t.TempDir(), "missing")}
			got := quotaEndpoints(context.Background(), cfg, secret)
			direct := false
			for _, endpoint := range got {
				if endpoint.Port == cfg.ProxyPort {
					direct = true
					for _, h := range endpoint.Hosts {
						if h != "127.0.0.1" && h != "localhost" {
							t.Fatalf("direct proxy exposed to %s", h)
						}
					}
				}
			}
			if direct != tc.wantDirect || (!tc.proven && len(got) > 0) {
				t.Fatalf("endpoints %+v", got)
			}
			home := t.TempDir()
			secrets := filepath.Join(home, "userdata", "secrets")
			if err := os.MkdirAll(secrets, 0o700); err != nil {
				t.Fatal(err)
			}
			settings := []byte(`{"usageLimitSources":{"direct":{"kind":"cliproxy","url":"http://127.0.0.1:8327"}}}`)
			if err := os.WriteFile(filepath.Join(home, "userdata", "settings.json"), settings, 0o600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(secrets, "usage-limit-source-ZGlyZWN0.bin")
			if err := os.WriteFile(path, []byte(secret.APIKey), 0o600); err != nil {
				t.Fatal(err)
			}
			n, err := (t3code.T3{Home: home}).FixQuotaKeys(got, secret.ManagementPassword)
			if err != nil {
				t.Fatal(err)
			}
			stored, err := os.ReadFile(path)
			if err != nil || (string(stored) == secret.ManagementPassword) != tc.wantDirect || (n == 1) != tc.wantDirect {
				t.Fatal("incorrect direct-source repair outcome", n, err)
			}

		})
	}
}
