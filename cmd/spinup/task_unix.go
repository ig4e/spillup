//go:build !windows

package main

import (
	"context"
	"io"
	"os/exec"
	"time"
)

// launchTask runs a spinup command with sudo asking the dashboard for your password.
func launchTask(ctx context.Context, exe string, args []string) (io.Reader, func() error, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.WaitDelay = 5 * time.Second
	stopAsking, err := askForPasswords(cmd, exe)
	if err != nil {
		return nil, nil, err
	}
	out, writer := io.Pipe()
	cmd.Stdout, cmd.Stderr = writer, writer
	if err := cmd.Start(); err != nil {
		stopAsking()
		out.Close()
		writer.Close()
		return nil, nil, err
	}
	done := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		stopAsking()
		writer.Close()
		done <- err
	}()
	return out, func() error { return <-done }, nil
}
