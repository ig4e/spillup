//go:build !windows

package main

import (
	"io"
	"os/exec"
)

// launchTask runs a spinup command with sudo asking the dashboard for your password.
func launchTask(exe string, args []string) (io.Reader, func() error, error) {
	cmd := exec.Command(exe, args...)
	stopAsking, err := askForPasswords(cmd, exe)
	if err != nil {
		return nil, nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		stopAsking()
		return nil, nil, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		stopAsking()
		return nil, nil, err
	}
	return out, func() error {
		defer stopAsking()
		return cmd.Wait()
	}, nil
}
