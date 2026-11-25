package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkyeg/spinup/internal/library"
	"github.com/darkyeg/spinup/internal/mcpvault"
)

func testManager(t *testing.T) Manager {
	t.Helper()
	return Manager{Library: library.At(t.TempDir()), State: t.TempDir()}
}
func rows(t *testing.T, m Manager) []Server {
	t.Helper()
	v, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	return v.Servers
}
func snapshot(t *testing.T, m Manager) []library.File {
	t.Helper()
	files, err := m.Library.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return files
}
func TestPrivateMCPAndSharedBaselineAreIndependent(t *testing.T) {
	a, b := testManager(t), testManager(t)
	d := Definition{Name: "notes", Transport: "stdio", Command: "notes", SecretKeys: []string{"NOTES_TOKEN"}}
	ctx := t.Context()
	if err := a.Save(ctx, d, "local"); err != nil {
		t.Fatal(err)
	}
	vault := mcpvault.Store{Dir: a.State}
	if err := vault.Set(ctx, "notes", "NOTES_TOKEN", "fixture-secret-value"); err != nil {
		t.Fatal(err)
	}
	if got := snapshot(t, a); len(got) != 0 {
		t.Fatal("private server in outgoing snapshot")
	}
	if err := a.Sharing(ctx, d.Name, "publish", d.Revision()); err != nil {
		t.Fatal(err)
	}
	baseline := snapshot(t, a)
	if len(baseline) != 1 || strings.Contains(string(baseline[0].Data), "fixture-secret-value") {
		t.Fatal("unexpected snapshot")
	}
	if err := b.Library.Take(baseline, time.Now()); err != nil {
		t.Fatal(err)
	}
	if r := rows(t, b); len(r) != 1 || len(r[0].Enabled) != 0 || len(r[0].AvailableKeys) != 0 {
		t.Fatal("shared definition enabled an agent or transferred a secret")
	}
	if err := a.Sharing(ctx, d.Name, "detach", d.Revision()); err != nil {
		t.Fatal(err)
	}
	mine := d
	mine.Command = "private-tool"
	if err := a.Save(ctx, mine, "local"); err != nil {
		t.Fatal(err)
	}
	other := d
	other.Command = "team-tool"
	if err := b.Save(ctx, other, "shared"); err != nil {
		t.Fatal(err)
	}
	if err := a.Library.Take(snapshot(t, b), time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := rows(t, a)[0]; got.Command != "private-tool" || got.Sharing != "local" {
		t.Fatal(got)
	}
	if strings.Contains(string(snapshot(t, a)[0].Data), "private-tool") {
		t.Fatal("private edit leaked")
	}
	if err := a.Sharing(ctx, d.Name, "follow", mine.Revision()); err != nil {
		t.Fatal(err)
	}
	if got := rows(t, a)[0]; got.Command != "team-tool" || len(got.AvailableKeys) != 1 {
		t.Fatal(got)
	}
	if err := a.Sharing(ctx, d.Name, "unpublish", other.Revision()); err != nil {
		t.Fatal(err)
	}
	if got := rows(t, a)[0]; got.HasShared || got.Command != "team-tool" || got.Sharing != "local" {
		t.Fatal(got)
	}
	if err := b.Library.Take(snapshot(t, a), time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(rows(t, b)) != 0 {
		t.Fatal("withdrawn definition still shared")
	}
}
func TestMCPRejectsCredentialBearingURLsAndUnknownFields(t *testing.T) {
	for _, s := range []string{"http://example.test/mcp", "https://u:p@example.test/mcp", "https://example.test/mcp?x=1", "https://example.test/#token"} {
		if (Definition{Name: "notes", Transport: "http", URL: s}).Validate() == nil {
			t.Fatalf("accepted URL %s", s)
		}
	}
	for _, s := range []string{`{"servers":[{"name":"x","transport":"stdio","command":"example","env":{"TOKEN":"x"}}]}`, `{"servers":[]}{}`, `{}`} {
		if ValidateManifest([]byte(s)) == nil {
			t.Fatal("accepted invalid manifest")
		}
	}
}
func TestMCPLaunchRequiresCurrentLocalApprovalAndScopedSecrets(t *testing.T) {
	m := testManager(t)
	d := Definition{Name: "demo", Transport: "stdio", Command: "demo", SecretKeys: []string{"DEMO_KEY"}}
	if err := m.Save(t.Context(), d, "local"); err != nil {
		t.Fatal(err)
	}
	if err := m.Run(t.Context(), "demo", "codex"); err == nil {
		t.Fatal("launched without local approval")
	}
	if err := saveCatalog(m.approvalPath("codex"), map[string]Definition{"demo": d}); err != nil {
		t.Fatal(err)
	}
	if err := m.Run(t.Context(), "demo", "codex"); err == nil || !strings.Contains(err.Error(), "set the MCP secret") {
		t.Fatal(err)
	}
	changed := d
	changed.Command = "another"
	if err := m.Save(t.Context(), changed, "local"); err != nil {
		t.Fatal(err)
	}
	if err := m.Run(t.Context(), "demo", "codex"); err == nil || !strings.Contains(err.Error(), "review") {
		t.Fatal(err)
	}
	if err := m.Target(t.Context(), "demo", "codex", d.Revision(), true); err == nil {
		t.Fatal("stale approval accepted")
	}
	if err := m.Remove(t.Context(), "demo"); err == nil {
		t.Fatal("removed an approved native registration")
	}
	if err := os.Remove(m.localPath()); err != nil {
		t.Fatal(err)
	}
	got := rows(t, m)[0]
	if !got.Removed || len(got.Enabled) != 1 {
		t.Fatal("lost locally active orphan")
	}
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "isolated"))
	if len(rows(t, m)) != 0 {
		t.Fatal("approval leaked into isolated native home")
	}
}
func TestMCPSettingsSurviveConcurrentUpdates(t *testing.T) {
	m := testManager(t)
	done := make(chan error, 2)
	for _, name := range []string{"a", "b"} {
		go func() {
			done <- m.Save(context.Background(), Definition{Name: name, Transport: "http", URL: "https://example.test/mcp"}, "shared")
		}()
	}
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	b, _ := json.Marshal(rows(t, m))
	if !strings.Contains(string(b), `"name":"a"`) || !strings.Contains(string(b), `"name":"b"`) {
		t.Fatal("lost an edit")
	}
}
