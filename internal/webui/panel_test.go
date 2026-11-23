package webui

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// fakeCPA serves a Management Center page and echoes what its management API received.
func fakeCPA(t *testing.T, name string) Panel {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/management.html" {
			fmt.Fprint(w, `<!doctype html><html><head><script type="module">app()</script></head></html>`)
			return
		}
		fmt.Fprintf(w, "%s|%s|%s|%s", name, r.Header.Get("Authorization"), r.Header.Get("Cookie"), r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	p, _ := strconv.Atoi(port)
	return Panel{Port: p, Key: name + "-key"}
}

func TestTheManagementCenterOpensSignedInWithoutTheKeyInTheBrowser(t *testing.T) {
	b := &fakeBackend{panels: map[string]Panel{"shared": fakeCPA(t, "shared"), "own": fakeCPA(t, "own")}}
	h := New(b, testToken).Handler()

	page := do(t, h, http.MethodGet, "/cpa/own/", signedIn)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "cli-proxy-auth") {
		t.Fatalf("panel page: %d %s", page.Code, page.Body)
	}
	if strings.Contains(page.Body.String(), "own-key") {
		t.Fatal("the panel page carries the real key")
	}
	if i, j := strings.Index(page.Body.String(), "cli-proxy-auth"), strings.Index(page.Body.String(), "app()"); i > j {
		t.Error("the sign-in seed must run before the app")
	}
	if fo := page.Header().Get("X-Frame-Options"); fo != "SAMEORIGIN" {
		t.Errorf("panel X-Frame-Options %q, want SAMEORIGIN so the dashboard can show it", fo)
	}
	cookie := page.Result().Cookies()[0]

	call := func(origin string) *httptest.ResponseRecorder {
		return do(t, h, http.MethodGet, "/v8/management/config", func(r *http.Request) {
			signedIn(r)
			r.Header.Set("Content-Type", "")
			r.AddCookie(cookie)
			if origin != "" {
				r.Header.Set("Origin", origin)
			}
		})
	}
	got := call("").Body.String()
	if !strings.HasPrefix(got, "own|Bearer own-key||/v8/management/config") {
		t.Errorf("panel call reached %q; want the own proxy, with its key and no cookies", got)
	}

	put := do(t, h, http.MethodPut, "/v0/management/config.yaml", func(r *http.Request) {
		signedIn(r)
		r.Header.Set("Content-Type", "application/yaml")
		r.Header.Set("Origin", "https://evil.example")
	})
	if put.Code != http.StatusForbidden {
		t.Errorf("a cross-site panel call: %d, want 403", put.Code)
	}
	same := do(t, h, http.MethodPut, "/v0/management/config.yaml", func(r *http.Request) {
		signedIn(r)
		r.Header.Set("Content-Type", "application/yaml")
		r.Header.Set("Origin", "http://localhost:8320")
	})
	if same.Code != http.StatusOK || !strings.HasPrefix(same.Body.String(), "shared|Bearer shared-key") {
		t.Errorf("a same-origin YAML upload without the panel cookie: %d %q; want the shared proxy", same.Code, same.Body)
	}
	if w := do(t, h, http.MethodGet, "/v8/management/config", nil); w.Code != http.StatusUnauthorized {
		t.Errorf("a panel call without a session: %d, want 401", w.Code)
	}
}
