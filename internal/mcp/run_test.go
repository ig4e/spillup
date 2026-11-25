package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/darkyeg/spinup/internal/filelock"
	"github.com/darkyeg/spinup/internal/library"
	"github.com/darkyeg/spinup/internal/mcpvault"
)

// The same binary serves as the Spillup CLI entry and a tiny MCP server in
// separate subprocesses. Stdin/out and termination use ordinary OS pipes.
func TestMCPStdioFixture(t *testing.T) {
	i := slices.Index(os.Args, "--")
	if i < 0 {
		return
	}
	args := os.Args[i+1:]
	if len(args) == 3 && args[0] == "launcher" {
		m := Manager{State: args[1], Library: library.At(args[2])}
		if m.Run(t.Context(), "notes", "codex") != nil {
			os.Exit(45)
		}
		os.Exit(0)
	}
	if len(args) != 2 || args[0] != "server" {
		os.Exit(46)
	}
	var req struct {
		ID     int
		Method string
	}
	if json.NewDecoder(os.Stdin).Decode(&req) != nil || req.Method != "initialize" {
		os.Exit(47)
	}
	if os.Getenv("NOTES_TOKEN") != "mcp-fixture" || os.Getenv("UNRELATED_FIXTURE") != "" {
		os.Exit(48)
	}
	lock, err := filelock.Acquire(context.Background(), args[1])
	if err != nil {
		os.Exit(49)
	}
	if json.NewEncoder(os.Stdout).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{
		"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "serverInfo": map[string]string{"name": "fixture", "version": "1"},
	}}) != nil {
		os.Exit(50)
	}
	// Simulate a server which ignores closed stdin and keeps serving.
	time.Sleep(time.Minute)
	lock.Close()
	os.Exit(0)
}

func TestApprovedMCPStdioLifecycle(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("PATH", filepath.Dir(exe)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("UNRELATED_FIXTURE", "other-fixture")
	lockPath := filepath.Join(t.TempDir(), "running")
	m := testManager(t)
	d := Definition{Name: "notes", Transport: "stdio", Command: filepath.Base(exe), Args: []string{"-test.run=TestMCPStdioFixture", "--", "server", lockPath}, SecretKeys: []string{"NOTES_TOKEN"}}
	if err = m.Save(t.Context(), d, "local"); err != nil {
		t.Fatal(err)
	}
	if err = saveCatalog(m.approvalPath("codex"), map[string]Definition{"notes": d}); err != nil {
		t.Fatal(err)
	}
	if err = (mcpvault.Store{Dir: m.State}).Set(t.Context(), d.Name, "NOTES_TOKEN", "mcp-fixture"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=TestMCPStdioFixture", "--", "launcher", m.State, m.Library.Dir())
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	if err = json.NewEncoder(input).Encode(map[string]any{"jsonrpc": "2.0", "id": 7, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "test", "version": "1"},
	}}); err != nil {
		t.Fatal(err)
	}
	var resp struct {
		JSONRPC string
		ID      int
		Result  struct {
			ProtocolVersion string
			ServerInfo      struct{ Name string }
		}
	}
	if err = json.NewDecoder(output).Decode(&resp); err != nil || resp.ID != 7 || resp.JSONRPC != "2.0" || resp.Result.ServerInfo.Name != "fixture" || resp.Result.ProtocolVersion != "2025-11-25" {
		t.Fatalf("invalid initialize response: %v", err)
	}
	_ = input.Close()
	// The initial response proves that the fixture has the lock. The native
	// client's hard disconnect must release it even when the server ignores EOF.
	if err = cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Fatal(err)
	}
	stop, done := context.WithTimeout(ctx, 2*time.Second)
	defer done()
	lock, err := filelock.Acquire(stop, lockPath)
	if err != nil {
		t.Fatal("server survived launcher termination:", err)
	}
	lock.Close()
}
