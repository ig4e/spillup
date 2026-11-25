package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeAliasOwnership(t *testing.T) {
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			exe := filepath.Join(t.TempDir(), "spillup")
			pending := filepath.Join(t.TempDir(), "http.pending")
			first := Definition{Name: "notes", Transport: "http", URL: "https://notes.example.test/mcp"}
			second := first
			second.URL = "https://other.example.test/mcp"
			run := func(current json.RawMessage, d Definition, enable bool, old *Definition, write bool) error {
				t.Helper()
				called := false
				config := nativeConfig{current: current, apply: func(_ context.Context, entry map[string]any) error {
					called = true
					if (entry == nil) == enable {
						t.Fatal("unexpected mutation")
					}
					return nil
				}}
				err := configureAlias(t.Context(), config, target, d, enable, old, exe, pending)
				if called != write {
					t.Fatalf("write=%v, want %v, err=%v", called, write, err)
				}
				return err
			}
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			raw := func(d Definition) json.RawMessage { b, _ := json.Marshal(registration(target, d, exe)); return b }
			// Same name and URL on their own do not prove a previous Spillup write.
			if err := run(raw(first), first, true, nil, false); !errors.Is(err, errCollision) {
				t.Fatal(err)
			}
			if err := run(raw(second), second, true, &first, false); !errors.Is(err, errCollision) {
				t.Fatal(err)
			}
			must(run(nil, first, true, nil, true)) // created the native entry; approval save was interrupted
			must(run(raw(first), first, true, nil, false))
			must(os.Remove(pending))
			must(run(raw(first), second, true, &first, true))
			must(run(raw(second), first, false, &first, true)) // interrupted replacement can be disabled
			edited := registration(target, first, exe)
			edited["headers"] = map[string]string{"Authorization": "fixture-private"}
			data, _ := json.Marshal(edited)
			if err := run(data, first, false, &first, false); !errors.Is(err, errCollision) || strings.Contains(err.Error(), "fixture-private") {
				t.Fatal("manual edit was accepted or disclosed")
			}
			must(run(nil, first, false, &first, false))
			stdio := Definition{Name: "notes", Transport: "stdio", Command: "example"}
			must(run(raw(stdio), stdio, true, nil, false))
		})
	}
}

func TestPreviouslyRegisteredLauncherMoved(t *testing.T) {
	pending := filepath.Join(t.TempDir(), "notes.pending")
	d := Definition{Name: "notes", Transport: "stdio", Command: "tool"}
	gui := registration("codex", d, "/old/desktop/spillup")
	raw, _ := json.Marshal(gui)
	if err := os.WriteFile(registeredPath(pending), []byte(expectedDigest(gui)), 0600); err != nil {
		t.Fatal(err)
	}
	called := false
	config := nativeConfig{current: raw, apply: func(_ context.Context, entry map[string]any) error {
		if entry != nil {
			t.Fatal("expected removal")
		}
		called = true
		return nil
	}}
	if err := configureAlias(t.Context(), config, "codex", d, false, &d, "/new/cli/spillup", pending); err != nil || !called {
		t.Fatal("a different executable could not remove the earlier registration", err)
	}
	gui["env"] = map[string]string{"CHANGED": "manually"}
	config.current, _ = json.Marshal(gui)
	called = false
	if err := configureAlias(t.Context(), config, "codex", d, false, &d, "/new/cli/spillup", pending); !errors.Is(err, errCollision) || called {
		t.Fatal("hand-edited launcher was removed", err)
	}
}

func TestWindowsNPMShimLookup(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "node with spaces")
	bin := filepath.Join(dir, "node_modules", "npm", "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	node := filepath.Join(dir, "node.exe")
	if err := os.WriteFile(node, nil, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"npm", "npx"} {
		script := filepath.Join(bin, name+"-cli.js")
		shim := filepath.Join(dir, name+".cmd")
		if _, ok := npmShellShim(shim); ok {
			t.Fatal("accepted missing native entrypoint")
		}
		if err := os.WriteFile(script, nil, 0600); err != nil {
			t.Fatal(err)
		}
		tool, ok := npmShellShim(shim)
		if !ok || tool.path != node || len(tool.prefix) != 1 || tool.prefix[0] != script {
			t.Fatal("could not resolve native entrypoint")
		}
	}
	for _, name := range []string{"other.cmd", "npx.bat", "npm.ps1"} {
		if _, ok := npmShellShim(filepath.Join(dir, name)); ok {
			t.Fatal("unknown shim accepted")
		}
	}
}

func TestClaudeUserConfigScope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	manager := testManager(t)
	first := manager.approvalPath("claude")
	if nativeConfigPath("claude") != filepath.Join(home, ".claude.json") {
		t.Fatal("incorrect default user config path")
	}
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	if first == manager.approvalPath("claude") || nativeConfigPath("claude") != filepath.Join(home, ".claude", ".claude.json") {
		t.Fatal("separate user configs share an approval")
	}
}

// Opt in for the full native registration lifecycle on hosts with the CLIs.
// Every write is directed into a fresh temporary HOME and agent config folder.
func TestInstalledNativeMCP(t *testing.T) {
	if os.Getenv("SPILLUP_TEST_NATIVE_MCP") != "1" {
		t.Skip("set SPILLUP_TEST_NATIVE_MCP=1 to test the installed agent CLIs")
	}
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, "claude"))
			t.Setenv("CODEX_HOME", filepath.Join(home, "codex"))
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
			t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
			t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
			t.Setenv("CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "1")
			if err := os.MkdirAll(filepath.Dir(nativeConfigPath(target)), 0700); err != nil {
				t.Fatal(err)
			}
			tool, ok := findNative(target)
			if !ok {
				t.Skip("native CLI unavailable")
			}
			m := testManager(t)
			d := Definition{Name: "notes", Transport: "stdio", Command: "notes", SecretKeys: []string{"NOTES_TOKEN"}}
			http := Definition{Name: "notes", Transport: "http", URL: "https://example.invalid/mcp"}
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			must(m.Save(ctx, d, "local"))
			must(m.Target(ctx, d.Name, target, d.Revision(), true))
			exe, err := os.Executable()
			must(err)
			read := func() json.RawMessage {
				t.Helper()
				if target == "claude" {
					r, err := readClaude(nativeConfigPath(target), "spillup-notes")
					must(err)
					return r
				}
				c, err := openCodex(ctx, tool, t.TempDir(), "spillup-notes")
				must(err)
				defer c.close()
				return c.current
			}
			if !matchesRegistration(read(), registration(target, d, exe)) {
				t.Fatal("actual user config has unexpected stdio registration")
			}
			must(m.Save(ctx, http, "local"))
			must(m.Target(ctx, http.Name, target, http.Revision(), true))
			if !matchesRegistration(read(), registration(target, http, exe)) {
				t.Fatal("actual user config has unexpected HTTP registration")
			}
			path := nativeConfigPath(target)
			original, err := os.ReadFile(path)
			must(err)
			if target == "codex" {
				f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
				must(err)
				_, err = f.WriteString("\n[mcp_servers.spillup-notes.tools.sample]\napproval_mode = \"prompt\"\n")
				must(err)
				must(f.Close())
			} else {
				var config map[string]any
				must(json.Unmarshal(original, &config))
				config["mcpServers"].(map[string]any)["spillup-notes"].(map[string]any)["headers"] = map[string]string{"Authorization": "fixture-private"}
				newData, err := json.Marshal(config)
				must(err)
				must(os.WriteFile(path, newData, 0600))
			}
			if err := m.Target(ctx, http.Name, target, http.Revision(), false); !errors.Is(err, errCollision) {
				t.Fatal("unrecognized user edit was not preserved:", err)
			}
			must(os.WriteFile(path, original, 0600))
			must(m.Target(ctx, http.Name, target, http.Revision(), false))
			if read() != nil {
				t.Fatal("agent still has the alias after removal")
			}
			must(NativeTarget(ctx, target, http, false, &http))
			// Crash after initial HTTP registration and before saving any approval.
			pending := m.approvalPath(target) + "." + http.Name + ".pending"
			must(nativeTarget(ctx, target, http, true, nil, pending, nil))
			must(m.Target(ctx, http.Name, target, "", false))
			if read() != nil {
				t.Fatal("disable left an interrupted initial registration behind")
			}
		})
	}
}
