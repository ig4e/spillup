package t3code

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLinksToMatchesTheHostInAURL(t *testing.T) {
	linked := `[{"url":"http://100.88.30.17:3773","label":"desktop"}]`
	if !LinksTo(linked, "100.88.30.17") {
		t.Error("the linked desktop wasn't found")
	}
	if LinksTo(linked, "100.88.30.1") || LinksTo(linked, "") || LinksTo("[]", "100.88.30.17") {
		t.Error("matched a host that isn't linked")
	}
}

func TestEnableLinkingKeepsEveryOtherSetting(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "userdata", "settings.json")
	os.MkdirAll(filepath.Dir(path), 0o700)
	os.WriteFile(path, []byte(`{"providerInstances":{"claude":{"driver":"claudeAgent"}},"idleCompaction":true}`), 0o600)
	t3 := T3{Home: home}

	changed, err := t3.EnableLinking()
	if err != nil || !changed {
		t.Fatalf("first time: changed=%v err=%v", changed, err)
	}
	var got map[string]any
	data, _ := os.ReadFile(path)
	json.Unmarshal(data, &got)
	if got["linkedEnvironmentsEnabled"] != true || got["idleCompaction"] != true || got["providerInstances"] == nil {
		t.Fatalf("settings after enabling: %s", data)
	}
	if changed, _ := t3.EnableLinking(); changed {
		t.Error("enabling twice changed the file again")
	}
}

func TestPortComesFromTheRuntimeFile(t *testing.T) {
	home := t.TempDir()
	if got := (T3{Home: home}).Port(); got != DefaultPort {
		t.Errorf("without a runtime file: %d, want %d", got, DefaultPort)
	}
	os.MkdirAll(filepath.Join(home, "userdata"), 0o700)
	os.WriteFile(filepath.Join(home, "userdata", "server-runtime.json"), []byte(`{"port":4000}`), 0o600)
	if got := (T3{Home: home}).Port(); got != 4000 {
		t.Errorf("with a runtime file: %d, want 4000", got)
	}
}
