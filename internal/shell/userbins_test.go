package shell

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestToolsInUserFoldersAreFoundWithoutANewTerminal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", "")
	name := "cargo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(home, ".cargo", "bin")
	os.MkdirAll(bin, 0o755)
	os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0o755)
	if _, ok := Find("cargo"); ok {
		t.Fatal("found before adding the user folders")
	}
	AddUserBins()
	if _, ok := Find("cargo"); !ok {
		t.Fatal("cargo in ~/.cargo/bin wasn't found after AddUserBins")
	}
}
