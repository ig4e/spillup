package webui

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/darkyeg/spinup/internal/t3code"
	"github.com/darkyeg/spinup/internal/tailnet"
)

type phonesBackend struct {
	tailnetBackend
	phones []t3code.Phone
	err    error
}

func (b phonesBackend) T3Phones(context.Context) ([]t3code.Phone, error) { return b.phones, b.err }

func TestT3PhonesRequiresDashboardAccessAndReturnsOnlySummaries(t *testing.T) {
	b := phonesBackend{
		tailnetBackend: tailnetBackend{&fakeBackend{}, tailnet.Status{
			Self:  tailnet.Node{Name: "desk", IP: "100.64.0.1", UserID: 1},
			Peers: []tailnet.Node{{Name: "phone", IP: "100.64.0.2", UserID: 1}},
		}},
		phones: []t3code.Phone{{Name: "phone", Kind: "phone", Count: 2}, {Kind: "tablet", Count: 1}},
	}
	path := "/api/t3/phones"
	s := New(b, testToken)
	if got := do(t, s.Handler(), http.MethodGet, path, nil); got.Code != http.StatusUnauthorized {
		t.Errorf("unsigned status = %d", got.Code)
	}
	body := `{"phones":[{"name":"phone","kind":"phone","count":2},{"kind":"tablet","count":1}]}`
	owned := func(r *http.Request) { r.Host = "desk:8320"; r.RemoteAddr = "100.64.0.2:4000" }
	for _, tc := range []struct {
		h       http.Handler
		path    string
		prepare func(*http.Request)
	}{
		{s.Handler(), path, signedIn}, {s.DeviceHandler(), path, owned}, {s.BridgeHandler(), BridgePrefix + path, nil},
	} {
		r := do(t, tc.h, http.MethodGet, tc.path, tc.prepare)
		if r.Code != http.StatusOK || strings.TrimSpace(r.Body.String()) != body {
			t.Errorf("%s status %d, body %s", tc.path, r.Code, r.Body.String())
		}
	}
	if r := do(t, s.DeviceHandler(), http.MethodGet, path, func(r *http.Request) { r.Host = "desk:8320"; r.RemoteAddr = "100.64.0.4:4000" }); r.Code != http.StatusForbidden {
		t.Errorf("unknown device status %d", r.Code)
	}
	b.phones = nil
	if r := do(t, New(b, testToken).Handler(), http.MethodGet, path, signedIn); strings.TrimSpace(r.Body.String()) != `{"phones":[]}` {
		t.Errorf("empty body %s", r.Body.String())
	}
	b.err = errors.New("private command output")
	if r := do(t, New(b, testToken).Handler(), http.MethodGet, path, signedIn); r.Code != http.StatusServiceUnavailable || strings.Contains(r.Body.String(), "private") {
		t.Errorf("failed read status %d, body %s", r.Code, r.Body.String())
	}
}
