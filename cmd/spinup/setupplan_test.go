package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/library"
	"github.com/darkyeg/spinup/internal/source"
)

func TestSetupPreviewDoesNotAdoptOrWriteManagedFiles(t *testing.T) {
	root := t.TempDir()
	state, lib := filepath.Join(root, "state"), filepath.Join(root, "library")
	t.Setenv("SPINUP_HOME", state)
	t.Setenv("SPINUP_LIBRARY", lib)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(root, "claude"))
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex"))
	t.Setenv("T3CODE_HOME", filepath.Join(root, "t3"))
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Setenv("LOCALAPPDATA", root)
	t.Setenv("PATH", t.TempDir())
	checkout := filepath.Join(root, "checkout")
	for path, data := range map[string]string{
		"local/AGENTS.md": "private-library-canary", "agents/AGENTS.md": "shared",
		"agents/claude/settings.json": "{}", "agents/codex/config.toml": "", "tools.json": `{"tools":[]}`,
	} {
		dest := filepath.Join(checkout, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	req := serviceRequest{repo: source.At(checkout), hold: config.HoldHub, apiKey: "private-api-canary", password: "private-admin-canary"}
	plan, err := previewSetup(context.Background(), "test-machine", req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "canary") {
		t.Fatal("preview exposed credentials or file contents")
	}
	if plan.Role != config.HoldHub {
		t.Fatalf("planned hold = %s", plan.Role)
	}
	want := map[string]string{config.Path(): "Create", library.Here().Path("AGENTS.md"): "Copy", filepath.Join(root, "claude", "CLAUDE.md"): "Create", installedBinary(): "Install"}
	for _, c := range plan.Changes {
		if want[c.Target] == c.Action {
			delete(want, c.Target)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing proposed changes: %v", want)
	}
	for _, dir := range []string{state, lib, filepath.Join(root, "claude"), filepath.Join(root, "codex"), filepath.Join(root, "t3")} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("preview created %s: %v", dir, err)
		}
	}
	cfg := config.Defaults()
	cfg.LegacyKeys = []string{"private-legacy-canary"}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	plan, err = previewSetup(context.Background(), "test-machine", req)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range plan.Changes {
		if c.Target == config.Path()+" · hold" && c.Detail == "never → hub" {
			found = true
		}
	}
	if !found {
		t.Fatal("preview omitted the role change")
	}
	raw, _ = json.Marshal(plan)
	if strings.Contains(string(raw), "canary") {
		t.Fatal("preview exposed private file contents")
	}
	after, err := config.Load()
	if err != nil || after.Hold != config.HoldNever {
		t.Fatal("preview changed saved role")
	}
}
