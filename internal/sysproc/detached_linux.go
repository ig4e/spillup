package sysproc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// StartDetached uses a transient user unit when systemd manages Spillup. A new session alone
// would still belong to the service's cgroup and would be killed during the restart.
func StartDetached(logPath string, exe string, args ...string) error {
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer log.Close()
	if underSystemd() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return startUserUnit(ctx, logPath, exe, args)
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func underSystemd() bool {
	if os.Getenv("INVOCATION_ID") != "" {
		return true
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		if _, err := os.Stat(filepath.Join(dir, "systemd", "private")); err == nil {
			return true
		}
	}
	groups, _ := os.ReadFile("/proc/self/cgroup")
	for _, entry := range strings.Split(string(groups), "/") {
		if strings.TrimSpace(entry) == "spinup.service" {
			return true
		}
	}
	return false
}

func startUserUnit(ctx context.Context, logPath, exe string, args []string) error {
	opts := []string{"--user", "--collect", "--quiet", "--service-type=exec", "--property=UMask=0077",
		"--property=StandardOutput=append:" + logPath, "--property=StandardError=append:" + logPath}
	if dir := os.Getenv("SPINUP_HOME"); dir != "" {
		opts = append(opts, "--setenv=SPINUP_HOME="+dir)
	}
	opts = append(opts, "--", exe)
	for _, arg := range args {
		opts = append(opts, strings.ReplaceAll(arg, "$", "$$"))
	}
	cmd := exec.CommandContext(ctx, "systemd-run", opts...)
	cmd.WaitDelay = 3 * time.Second
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("start separate systemd user unit: %w", ctx.Err())
	}
	if err != nil {
		return fmt.Errorf("start separate systemd user unit: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
