//go:build !windows

package sysinfo

import (
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestEndQuitsAProcessAndRefusesProtectedOnes(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if err := End(cmd.Process.Pid, false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the process kept running")
	}
	for _, pid := range []int{1, os.Getpid()} {
		if err := End(pid, true); !errors.Is(err, ErrProtected) {
			t.Errorf("End(%d) = %v, want ErrProtected", pid, err)
		}
	}
}
