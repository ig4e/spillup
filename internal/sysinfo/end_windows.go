package sysinfo

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/sysproc"
)

// end asks the program's windows to close, or with force terminates it, through taskkill.
func end(pid int, force bool) error {
	args := []string{"/PID", strconv.Itoa(pid)}
	if force {
		args = append(args, "/F")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "taskkill.exe", args...)
	cmd.WaitDelay = 3 * time.Second
	sysproc.Hide(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("taskkill: %w", ctx.Err())
		}
		return fmt.Errorf("taskkill: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
