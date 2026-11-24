//go:build !linux && !windows

package main

import (
	"context"

	"github.com/darkyeg/spinup/internal/shell"
	"github.com/darkyeg/spinup/internal/sunshine"
)

// installRemotePackages installs Moonlight and Sunshine with Homebrew.
func installRemotePackages(ctx context.Context) error {
	if sunshine.MoonlightExe() == "" {
		if err := shell.Run(ctx, "brew", "install", "--cask", "moonlight"); err != nil {
			return err
		}
	}
	if sunshine.Exe() == "" {
		return shell.Run(ctx, "brew", "install", "lizardbyte/homebrew/sunshine")
	}
	return nil
}

// startSunshine gives Sunshine spillup's login; on macOS it runs as a login item you start once.
func startSunshine(ctx context.Context, exe string, login sunshine.Login) error {
	return shell.Run(ctx, exe, "--creds", login.User, login.Password)
}
