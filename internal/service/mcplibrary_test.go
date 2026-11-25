package service

import (
	"os"
	"testing"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/library"
)

func TestMCPManifestRequiresCompatiblePeerAndSurvivesLegacyReplace(t *testing.T) {
	m := artifactMachine(t)
	baseline, err := m.library.snapshot(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Protocol != 2 {
		t.Fatal("a fresh library cannot sync with package-aware releases")
	}
	if err := os.WriteFile(m.library.lib.Path(library.MCPServers), []byte(`{"servers":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	state, err := m.library.state(now)
	if err != nil || state.Protocol != 3 || !state.HasMCP {
		t.Fatal(state, err)
	}
	outgoing, err := m.library.snapshot(now)
	if err != nil || outgoing.Protocol != 3 {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		protocol int
		files    []library.File
	}{
		{2, []library.File{{Path: library.Instructions, Data: []byte("legacy edit")}}},
		{2, outgoing.Files},
		{3, []library.File{{Path: library.MCPServers, Data: []byte(`{"servers":[{"name":"x","transport":"stdio","command":"tool","secretValues":{"KEY":"fixture"}}]}`)}}},
	} {
		incoming := api.Library{Protocol: tc.protocol, Files: tc.files, Stamp: library.Stamp{Hash: library.Fingerprint(tc.files), ChangedAt: now.Add(time.Second)}}
		if err := m.library.take(t.Context(), incoming, now); err == nil {
			t.Fatal("accepted incompatible manifest")
		}
	}
	after, err := m.library.snapshot(time.Now())
	if err != nil || after.Stamp != outgoing.Stamp {
		t.Fatal("rejected replacement changed the library")
	}
}
