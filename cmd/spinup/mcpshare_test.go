package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/mcpvault"
	"github.com/darkyeg/spinup/internal/tailnet"
	"github.com/darkyeg/spinup/internal/webui"
)

func TestMCPTransferChecksCallerAndConsensusBeforeSharing(t *testing.T) {
	n := tailnet.Node{ID: "sender-id", Name: "sender", Online: true, UserID: 1, IP: "100.64.0.2"}
	st := tailnet.Status{Running: true, Self: tailnet.Node{ID: "target-id", Name: "target", UserID: 1}, Peers: []tailnet.Node{n}}
	ref := webui.MCPShareRef{Target: "target", Source: "sender", Server: "probe", Key: "TOKEN"}
	if _, err := mcpCaller(st, "100.64.0.4:123", ref); err == nil {
		t.Fatal("spoofed source")
	}
	if _, err := mcpCaller(st, "[::ffff:100.64.0.2]:123", ref); err != nil {
		t.Fatal(err)
	}
	wrong := ref
	wrong.Target = "other"
	if _, err := mcpCaller(st, "100.64.0.2:123", wrong); err == nil {
		t.Fatal("wrong target")
	}
	source := mcpvault.Store{Dir: t.TempDir()}
	sink := mcpvault.Store{Dir: t.TempDir()}
	const value = "synthetic-just-for-this-test"
	if err := source.Set(t.Context(), ref.Server, ref.Key, value); err != nil {
		t.Fatal(err)
	}
	bind := mcpBinding(st, n)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.RawQuery != "" || r.Header.Get(api.KeyHeader) != "" || r.Header.Get(api.APIKeyHeader) != "fleet" {
			t.Error("unexpected transfer metadata")
		}
		switch r.URL.Path {
		case "/permit":
			var payload map[string]string
			json.NewDecoder(r.Body).Decode(&payload)
			if _, ok := payload["value"]; ok {
				t.Error("sent value before consent")
			}
			permit, err := sink.Permit(ref.Server, ref.Key, bind)
			if err != nil {
				http.Error(w, value, 403)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"permit": permit})
		case "/receive":
			var in webui.MCPTransfer
			if json.NewDecoder(r.Body).Decode(&in) != nil || in.MCPShareRef != ref {
				t.Error("bad transfer")
			}
			if err := sink.Receive(r.Context(), ref.Server, ref.Key, bind, in.Permit, string(in.Value)); err != nil {
				t.Error(err)
				w.WriteHeader(403)
			}
		}
	}))
	defer srv.Close()
	client := api.Client{HTTP: srv.Client(), APIKey: "fleet"}
	err := sendMCPSecret(t.Context(), client, srv.URL+"/", ref, source)
	if err == nil || strings.Contains(err.Error(), value) {
		t.Fatal("missing consent or error leaked value")
	}
	if err := sink.Allow(t.Context(), ref.Server, ref.Key, bind); err != nil {
		t.Fatal(err)
	}
	if err := sendMCPSecret(t.Context(), client, srv.URL+"/", ref, source); err != nil {
		t.Fatal(err)
	}
	if v, err := sink.Get(ref.Server, ref.Key); err != nil || v != value {
		t.Fatal("missing transferred secret")
	}
}
