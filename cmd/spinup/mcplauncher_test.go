package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMCPLauncherUsesMatchingInstalledRelease(t *testing.T) {
	t.Setenv("SPINUP_HOME", t.TempDir())
	self := filepath.Join(t.TempDir(), "spillup")
	if err := os.WriteFile(self, []byte("new"), 0755); err != nil {
		t.Fatal(err)
	}
	installed := installedBinary()
	if mcpLauncher(self) != self {
		t.Fatal("uninstalled CLI selected a different binary")
	}
	if err := os.MkdirAll(filepath.Dir(installed), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installed, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	if mcpLauncher(self) != self {
		t.Fatal("older daemon chosen for a newer CLI")
	}
	if err := os.WriteFile(installed, []byte("new"), 0755); err != nil {
		t.Fatal(err)
	}
	if mcpLauncher(self) != installed || mcpLauncher(installed) != installed {
		t.Fatal("CLI and daemon did not select the same durable launcher")
	}
}
