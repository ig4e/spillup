package sysproc

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// StartDetached starts a program that must outlive spinup's service, such as an update that restarts it:
// in a systemd unit of its own when there is one, since systemd stops a service's whole cgroup, otherwise
// in a session of its own. Its output goes to logPath.
func StartDetached(logPath string, exe string, args ...string) error {
	if run, err := exec.LookPath("systemd-run"); err == nil {
		unit := fmt.Sprintf("spinup-detached-%d", time.Now().Unix())
		cmd := exec.Command(run, append([]string{"--user", "--collect", "--quiet", "--unit=" + unit,
			"--property=StandardOutput=append:" + logPath, "--property=StandardError=append:" + logPath, exe}, args...)...)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
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
