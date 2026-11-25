package sysproc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSystemdRejectsAnUnstartableUnitWithoutRunningInTheParentCgroup(t *testing.T) {
	t.Setenv("INVOCATION_ID", "test-unit")
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	marker := filepath.Join(dir, "unexpected")
	exe := filepath.Join(dir, "helper")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n: > "+marker+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	err := StartDetached(filepath.Join(dir, "job.log"), exe)
	if err == nil || !strings.Contains(err.Error(), "systemd") {
		t.Fatalf("start: %v", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("helper ran inside old cgroup")
	}
}

func TestUserUnitHasADeadlineAndCarriesStateDirectory(t *testing.T) {
	dir := t.TempDir()
	run := filepath.Join(dir, "systemd-run")
	t.Setenv("PATH", dir)
	t.Setenv("SPINUP_HOME", filepath.Join(dir, "state with spaces"))
	args := filepath.Join(dir, "args")
	t.Setenv("TEST_UNIT_ARGS", args)
	if err := os.WriteFile(run, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TEST_UNIT_ARGS\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := startUserUnit(context.Background(), "job.log", "/bin/true", []string{"--home", "a$b"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(args)
	if err != nil || !strings.Contains(string(got), "--setenv=SPINUP_HOME="+os.Getenv("SPINUP_HOME")+"\n") || !strings.Contains(string(got), "--service-type=exec\n") || !strings.HasSuffix(string(got), "--home\na$$b\n") {
		t.Fatalf("arguments %q, %v", got, err)
	}
	if err := os.WriteFile(run, []byte("#!/bin/sh\nexec /bin/sleep 20\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := startUserUnit(ctx, "job.log", "/bin/true", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stuck launcher: %v", err)
	}
}
