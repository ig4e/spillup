package webui

import (
	"net/http"
	"strings"
	"testing"
)

func TestSetupPreviewIsLocalAndNeedsSession(t *testing.T) {
	s := New(&fakeBackend{}, testToken)
	const path = "/api/setup/preview?name=desk&role=use"
	if w := do(t, s.Handler(), http.MethodGet, path, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned: %d", w.Code)
	}
	if w := do(t, s.Handler(), http.MethodGet, path, signedIn); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"name":"desk"`) {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, s.BridgeHandler(), http.MethodGet, BridgePrefix+path, nil); w.Code != http.StatusNotFound {
		t.Fatalf("remote preview: %d", w.Code)
	}
}

func TestFleetPreviewOnlyAnswersInLocalDashboard(t *testing.T) {
	s := New(&fakeBackend{}, testToken)
	const path = "/api/update/fleet"
	if w := do(t, s.Handler(), http.MethodGet, path, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned: %d", w.Code)
	}
	if w := do(t, s.Handler(), http.MethodGet, path, signedIn); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "fleet preview") {
		t.Fatalf("signed in: %d, %s", w.Code, w.Body.String())
	}
	if w := do(t, s.BridgeHandler(), http.MethodGet, BridgePrefix+path, nil); w.Code != http.StatusNotFound {
		t.Fatalf("remote: %d", w.Code)
	}
}
