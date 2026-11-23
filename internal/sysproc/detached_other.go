//go:build !windows && !linux

package sysproc

import (
	"os"
	"os/exec"
	"syscall"
)

// StartDetached starts a program that must outlive spinup's service, in a session of its own.
func StartDetached(logPath string, exe string, args ...string) error {
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.Command(exe, args...)
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
