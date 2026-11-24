//go:build !windows

package sshd

import (
	"context"
	"encoding/json"
	"os/exec"
	"strconv"

	"github.com/darkyeg/spinup/internal/shell"
	"github.com/darkyeg/spinup/internal/sysproc"
)

// On reports whether Tailscale SSH is on here.
func On(ctx context.Context, tailscale string) bool {
	cmd := exec.CommandContext(ctx, tailscale, "debug", "prefs")
	sysproc.Hide(cmd)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	var prefs struct{ RunSSH bool }
	return json.Unmarshal(out, &prefs) == nil && prefs.RunSSH
}

// Enable turns Tailscale SSH on or off, which needs root.
func Enable(ctx context.Context, tailscale string, on bool) error {
	return shell.AsRoot(ctx, tailscale, "set", "--ssh="+strconv.FormatBool(on))
}
