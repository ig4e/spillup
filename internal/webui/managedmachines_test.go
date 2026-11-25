package webui

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/darkyeg/spinup/internal/config"
)

type managedBackend struct {
	fakeBackend
	calls int
	err   error
}

func (b *managedBackend) ManagedMachines(context.Context) ([]ManagedMachine, error) {
	b.calls++
	return []ManagedMachine{{Machine: Machine{Name: "worker", Hold: config.HoldNever, Online: true}, Terminals: false}}, b.err
}

func TestOnlyAuthenticatedManagedInventoryTriggersDiscovery(t *testing.T) {
	b := &managedBackend{}
	h := New(b, testToken).Handler()
	const path = "/api/machines/managed"
	if r := do(t, h, http.MethodGet, path, nil); r.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated status %d", r.Code)
	}
	if r := do(t, h, http.MethodGet, "/api/machines", signedIn); r.Code != http.StatusOK || b.calls != 0 {
		t.Fatalf("raw inventory ran discovery: %d calls %d", r.Code, b.calls)
	}
	if r := do(t, h, http.MethodGet, path, signedIn); r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"terminals":false`) || !strings.Contains(r.Body.String(), `"hold":"never"`) {
		t.Fatalf("managed response: %d %s", r.Code, r.Body)
	}
	b.err = errors.New("internal details")
	if r := do(t, h, http.MethodGet, path, signedIn); r.Code != http.StatusServiceUnavailable || strings.Contains(r.Body.String(), "internal details") {
		t.Errorf("discovery error: %d %s", r.Code, r.Body)
	}
}
