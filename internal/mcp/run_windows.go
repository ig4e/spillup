package mcp

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/darkyeg/spinup/internal/sysproc"
)

func runStdio(cmd *exec.Cmd) error {
	switch strings.ToLower(filepath.Ext(cmd.Path)) {
	case ".bat", ".cmd", ".ps1":
		tool, ok := npmShellShim(cmd.Path)
		if !ok {
			return errors.New("use a native MCP executable, or run its script through an explicitly approved interpreter")
		}
		cmd.Path = tool.path
		cmd.Args = append([]string{tool.path, tool.prefix[0]}, cmd.Args[1:]...)
	}
	// Closing the launcher's Windows job also stops a server which ignored stdin EOF.
	wait, err := sysproc.StartBound(cmd)
	if err != nil {
		return err
	}
	return wait()
}
