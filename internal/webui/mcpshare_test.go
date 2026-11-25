package webui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func (*fakeBackend) MCPSecret(context.Context, string, MCPSecretRequest) error      { return nil }
func (*fakeBackend) MCPPermit(context.Context, string, MCPShareRef) (string, error) { return "", nil }
func (*fakeBackend) MCPReceive(context.Context, string, MCPTransfer) error          { return nil }

func TestMCPVaultRoutesStayAtTheirTrustBoundary(t *testing.T) {
	s := New(&fakeBackend{}, testToken)
	const path = "/api/mcp/secret"
	if r := do(t, s.Handler(), http.MethodGet, path, signedIn); r.Code == 200 {
		t.Fatal("GET can access a secret")
	}
	post := func(h http.Handler, path string, auth bool) int {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"server":"example","key":"TOKEN","value":"synthetic"}`))
		r.Host = "localhost"
		r.Header.Set("Content-Type", "application/json")
		if auth {
			signedIn(r)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if strings.Contains(w.Body.String(), "synthetic") {
			t.Fatal("response echoed a secret")
		}
		return w.Code
	}
	if post(s.Handler(), path, false) != 401 || post(s.Handler(), path, true) != 200 {
		t.Fatal("local session guard")
	}
	for _, op := range []string{"secret", "forget", "allow", "share"} {
		if post(s.BridgeHandler(), BridgePrefix+"/api/mcp/"+op, false) != 404 {
			t.Fatal("bridge permits local operation", op)
		}
	}
	for _, op := range []string{"mcp", "mcp/target", "mcp/save", "mcp/sharing", "mcp/remove"} {
		if post(s.Handler(), "/api/remote/peer/"+op, true) != 404 {
			t.Fatal("local MCP route reached a remote bridge", op)
		}
	}
	for _, op := range []string{"permit", "receive"} {
		if post(s.Handler(), "/api/host/mcp/"+op, true) != 404 || post(s.Handler(), "/api/remote/peer/host/mcp/"+op, true) != 404 {
			t.Fatal("untrusted access to transfer transport", op)
		}
	}
}
