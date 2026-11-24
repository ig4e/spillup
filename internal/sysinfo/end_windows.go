package sysinfo

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/darkyeg/spinup/internal/sysproc"
)

// end asks the program's windows to close, or with force terminates it, through taskkill.
func end(pid int, force bool) error {
	args := []string{"/PID", strconv.Itoa(pid)}
	if force {
		args = append(args, "/F")
	}
	cmd := exec.Command("taskkill.exe", args...)
	sysproc.Hide(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("taskkill: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
