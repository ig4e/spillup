package webui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type remoteBackend struct {
	fakeBackend
	remote Remote
}

func (b *remoteBackend) Remote(string) (Remote, error) { return b.remote, nil }

func TestARemoteCallReachesTheOtherMachineWithKeysAndWithoutCookies(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s %s key=%s cookie=%q", r.Method, r.URL.Path, r.Header.Get("X-Spinup-Key"), r.Header.Get("Cookie"))
	}))
	defer other.Close()
	base, _ := url.Parse(other.URL)
	h := http.Header{}
	h.Set("X-Spinup-Key", "m")
	b := &remoteBackend{remote: Remote{Base: base, Header: h}}
	srv := New(b, testToken).Handler()

	w := do(t, srv, http.MethodGet, "/api/remote/laptop/skills", signedIn)
	if got := w.Body.String(); !strings.HasPrefix(got, `GET /spinup/ui/api/skills key=m cookie=""`) {
		t.Errorf("remote call arrived as %q", got)
	}
	if w := do(t, srv, http.MethodGet, "/api/remote/laptop/skills", nil); w.Code != http.StatusUnauthorized {
		t.Errorf("remote call without a session: %d, want 401", w.Code)
	}
}

func TestTheBridgeKeepsKeysAndSignInsLocal(t *testing.T) {
	h := New(&fakeBackend{}, testToken).BridgeHandler()
	call := func(method, path, contentType string) int {
		r := httptest.NewRequest(method, path, strings.NewReader("{}"))
		r.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if got := call(http.MethodGet, "/spinup/ui/api/overview", ""); got != http.StatusOK {
		t.Errorf("overview over the bridge: %d, want 200", got)
	}
	if got := call(http.MethodGet, "/spinup/ui/api/keys", ""); got != http.StatusNotFound {
		t.Errorf("keys over the bridge: %d, want 404", got)
	}
	if got := call(http.MethodPost, "/spinup/ui/api/own/signin", "application/json"); got != http.StatusNotFound {
		t.Errorf("sign-in over the bridge: %d, want 404", got)
	}
	if got := call(http.MethodPost, "/spinup/ui/api/machines/main", "text/plain"); got != http.StatusUnsupportedMediaType {
		t.Errorf("a non-JSON change over the bridge: %d, want 415", got)
	}
}

func TestAnotherMachinesCLIProxyAPIDashboardComesFromThatMachine(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/spinup/ui/cpa/shared/" {
			fmt.Fprint(w, "<html>laptop's panel</html>")
			return
		}
		fmt.Fprintf(w, "%s panel=%s key=%s auth=%q cookie=%q", r.URL.Path, r.Header.Get("X-Spinup-Panel"),
			r.Header.Get("X-Spinup-Key"), r.Header.Get("Authorization"), r.Header.Get("Cookie"))
	}))
	defer other.Close()
	base, _ := url.Parse(other.URL)
	h := http.Header{}
	h.Set("X-Spinup-Key", "m")
	srv := New(&remoteBackend{remote: Remote{Base: base, Header: h}}, testToken).Handler()

	page := do(t, srv, http.MethodGet, "/cpa/shared/?machine=laptop", signedIn)
	if !strings.Contains(page.Body.String(), "laptop's panel") {
		t.Fatalf("panel page: %d %s", page.Code, page.Body)
	}
	var cookies []*http.Cookie
	for _, c := range page.Result().Cookies() {
		if c.Value != "" {
			cookies = append(cookies, c)
		}
	}
	call := do(t, srv, http.MethodGet, "/v8/management/config", func(r *http.Request) {
		signedIn(r)
		r.Header.Set("Authorization", "Bearer spinup")
		for _, c := range cookies {
			r.AddCookie(c)
		}
	})
	if got := call.Body.String(); got != `/spinup/ui/v8/management/config panel=shared key=m auth="" cookie=""` {
		t.Errorf("panel call reached the laptop as %q", got)
	}
}
