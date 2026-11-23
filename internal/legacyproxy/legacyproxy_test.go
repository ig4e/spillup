package legacyproxy

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseReadsKeysAndAuthDir(t *testing.T) {
	text := `config-version: 8
server:
  port: 8317
access:
  api-keys: # your clients' keys
    - "key-one"
    - 'key-two'
    - key-three
management:
  secret-key: "not-an-api-key"
oauth:
  auth-dir: "~/.config/cliproxyapi/auth"  # logins
`
	got := Parse(text, "/home/me")
	if want := []string{"key-one", "key-two", "key-three"}; !reflect.DeepEqual(got.APIKeys, want) {
		t.Errorf("keys %v, want %v", got.APIKeys, want)
	}
	if want := filepath.Join("/home/me", ".config/cliproxyapi/auth"); got.AuthDir != want {
		t.Errorf("auth dir %q, want %q", got.AuthDir, want)
	}
}

func TestFindSkipsSpillupsOwnConfigAndListsLogins(t *testing.T) {
	home := t.TempDir()
	own := filepath.Join(home, ".config", "cliproxyapi", "config.yaml")
	os.MkdirAll(filepath.Dir(own), 0o700)
	os.WriteFile(own, []byte("access:\n  api-keys:\n    - old\n"), 0o600)
	if f, ok := Find(home, own); ok {
		t.Fatalf("found spillup's own config as an earlier one: %+v", f)
	}
	f, ok := Find(home, "")
	if !ok || f.APIKeys[0] != "old" || f.AuthDir != filepath.Join(home, ".cli-proxy-api") {
		t.Fatalf("found %+v %v", f, ok)
	}
	os.MkdirAll(f.AuthDir, 0o700)
	for _, n := range []string{"claude-a.json", "codex-b.json", "kv.db", ".oauth-x.json", "codex-b.json.lock"} {
		os.WriteFile(filepath.Join(f.AuthDir, n), []byte("{}"), 0o600)
	}
	if got := Logins(f.AuthDir); !reflect.DeepEqual(got, []string{"claude-a.json", "codex-b.json"}) {
		t.Errorf("logins %v", got)
	}
}
