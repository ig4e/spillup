//go:build !windows

package mcp

import (
	"os/exec"
	"syscall"
)

// The agent owns this process and its stdin/stdout. Replace the launcher so
// disconnect signals reach the approved server even when it ignores stdin EOF.
func runStdio(cmd *exec.Cmd) error {
	return syscall.Exec(cmd.Path, cmd.Args, cmd.Env)
}
