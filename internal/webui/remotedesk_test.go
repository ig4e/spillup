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

type remoteDeskBackend struct {
	fakeBackend
	port int
	pin  []string
}

func (b *remoteDeskBackend) RemoteWebPort() int { return b.port }

func TestMoonlightWebIsServedSignedInAndOnlyToTheDashboardsBrowser(t *testing.T) {
	mws := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s user=%s cookie=%q", r.URL.Path, r.Header.Get("X-Spinup-User"), r.Header.Get("Cookie"))
	}))
	defer mws.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(mws.URL, "http://"))
	p, _ := strconv.Atoi(port)
	h := New(&remoteDeskBackend{port: p}, testToken).Handler()

	if w := do(t, h, http.MethodGet, "/remote/", nil); w.Code != http.StatusUnauthorized {
		t.Errorf("without a session: %d, want 401", w.Code)
	}
	w := do(t, h, http.MethodGet, "/remote/api/hosts", func(r *http.Request) {
		signedIn(r)
		r.Header.Set("X-Spinup-User", "admin")
	})
	if got := w.Body.String(); got != `/remote/api/hosts user=spillup cookie=""` {
		t.Errorf("reached moonlight-web as %q", got)
	}
	foreign := do(t, h, http.MethodGet, "/remote/api/host/stream/web_socket", func(r *http.Request) {
		signedIn(r)
		r.Header.Set("Upgrade", "websocket")
		r.Header.Set("Origin", "https://other.example")
	})
	if foreign.Code != http.StatusForbidden {
		t.Fatal("cross-site stream upgrade accepted", foreign.Code)
	}
	if legacy := do(t, h, http.MethodGet, "/remote/stream.html?hostId=7&appId=1", signedIn); legacy.Code != 303 || legacy.Header().Get("Location") != "/desktop?hostId=7" {
		t.Fatal("legacy stream did not open Spillup")
	}
	if admin := do(t, h, http.MethodGet, "/remote/api/users", signedIn); admin.Code != 404 {
		t.Fatal("raw upstream admin exposed")
	}
	if fo := w.Header().Get("X-Frame-Options"); fo != "SAMEORIGIN" {
		t.Errorf("X-Frame-Options %q, want SAMEORIGIN so the Remote page can show it", fo)
	}
	cross := do(t, h, http.MethodPost, "/remote/api/host", func(r *http.Request) {
		signedIn(r)
		r.Header.Set("Origin", "https://evil.example")
	})
	if cross.Code != http.StatusForbidden {
		t.Errorf("a cross-site post to moonlight-web: %d, want 403", cross.Code)
	}
}

func TestSunshinePinTakesOnlyFourDigits(t *testing.T) {
	h := New(&fakeBackend{}, testToken).Handler()
	for body, want := range map[string]int{`{"pin":"12a4"}`: 400, `{"pin":"12345"}`: 400, `{"pin":"1234"}`: 200} {
		r := httptest.NewRequest(http.MethodPost, "/api/sunshine/pin", strings.NewReader(body))
		r.Host = "localhost:8320"
		signedIn(r)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Errorf("%s: %d, want %d", body, w.Code, want)
		}
	}
}

func TestClipboardRoutesAreDirectionalAndNeverCached(t *testing.T) {
	b := &fakeBackend{}
	local := New(b, testToken).Handler()
	if w := do(t, local, "POST", "/api/remote/clipboard", nil); w.Code == 200 {
		t.Fatal("anonymous clipboard")
	}
	if w := do(t, local, "POST", "/api/host/clipboard", signedIn); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := do(t, local, "POST", "/api/remote/clipboard", signedIn); w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code)
	}
	bridge := New(b, "").BridgeHandler()
	if w := do(t, bridge, "POST", BridgePrefix+"/api/remote/clipboard", signedIn); w.Code != 404 {
		t.Fatal("clipboard relay reachable over bridge", w.Code)
	}
}
