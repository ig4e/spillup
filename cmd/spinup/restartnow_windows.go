package main

import (
	"errors"
	"os"
	"os/exec"
	"time"

	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/sysproc"
)

// restartServiceNow restarts the accounts service without asking for admin: it stops the running one,
// gracefully where this machine has the key to ask, and starts the boot task again, which runs as this
// user and starts the binary installed now.
func restartServiceNow(cfg config.Config) error {
	l := localService(cfg, "")
	rep, reportErr := l.report()
	stopService()
	if reportErr == nil && rep.PID > 0 {
		if _, err := l.leader(); err == nil {
			if p, err := os.FindProcess(rep.PID); err == nil {
				_ = p.Kill()
			}
		}
	}
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(300 * time.Millisecond) {
		if _, err := l.leader(); err != nil {
			break
		}
	}
	cmd := exec.Command("schtasks.exe", "/Run", "/TN", "spinup")
	sysproc.Hide(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.New("Windows didn't start the boot task again: " + string(out))
	}
	return nil
}
