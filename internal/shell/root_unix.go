//go:build !windows

package shell

import (
	"context"
	"os"
)

// AsRoot runs the program through sudo unless spinup already runs as root.
func AsRoot(ctx context.Context, name string, args ...string) error {
	if os.Geteuid() == 0 {
		return Run(ctx, name, args...)
	}
	sudo := Sudo()
	return Run(ctx, sudo[0], append(sudo[1:], append([]string{name}, args...)...)...)
}

// Sudo is how to ask for root: with -A when a helper asks for the password, as the dashboard does when it
// runs setup without a terminal.
func Sudo() []string {
	if os.Getenv("SUDO_ASKPASS") != "" {
		return []string{"sudo", "-A"}
	}
	return []string{"sudo"}
}

// RefreshPath is only needed on Windows.
func RefreshPath() {}
