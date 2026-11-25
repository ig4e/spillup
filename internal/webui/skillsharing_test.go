package webui

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkyeg/spinup/internal/library"
	"github.com/darkyeg/spinup/internal/skills"
	"github.com/darkyeg/spinup/internal/source"
)

type skillSharingBackend struct {
	*fakeBackend
	manager skills.Manager
}

func (b skillSharingBackend) Skills() skills.Manager { return b.manager }

func TestPrivateSkillViewsAndScopeActions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, "claude"))
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex"))
	m := skills.New(source.Builtin(), library.At(filepath.Join(home, "library")), nil)
	if _, err := m.CreateScoped(t.Context(), "only-on-host", "local description", true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(t.Context(), "fleet", "fleet description"); err != nil {
		t.Fatal(err)
	}
	s := New(skillSharingBackend{&fakeBackend{}, m}, testToken)
	w := do(t, s.Handler(), "GET", "/api/skills", signedIn)
	var local Skills
	if err := json.Unmarshal(w.Body.Bytes(), &local); err != nil || w.Code != 200 || !local.SharingAvailable || len(local.Shared) != 2 {
		t.Fatal(w.Code, w.Body.String(), err)
	}
	if !local.Shared[1].Private || local.Shared[1].SharedAvailable || local.Shared[1].Path == "" {
		t.Fatal(local.Shared[1])
	}
	w = do(t, s.BridgeHandler(), "GET", BridgePrefix+"/api/skills", nil)
	var remote Skills
	if err := json.Unmarshal(w.Body.Bytes(), &remote); err != nil || w.Code != 200 || remote.SharingAvailable || len(remote.Shared) != 1 || remote.Shared[0].Name != "fleet" || strings.Contains(w.Body.String(), "only-on-host") {
		t.Fatal(w.Code, w.Body.String(), err)
	}
	w = do(t, s.BridgeHandler(), http.MethodPost, BridgePrefix+"/api/skills/sharing", signedIn)
	if w.Code != http.StatusNotFound {
		t.Fatal("private scope exposed via bridge", w.Code)
	}
}
