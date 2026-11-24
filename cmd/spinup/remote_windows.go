package main

import (
	"context"
	"strings"

	"github.com/darkyeg/spinup/internal/shell"
	"github.com/darkyeg/spinup/internal/sunshine"
)

// installRemotePackages installs Sunshine and Moonlight with winget.
func installRemotePackages(ctx context.Context) error {
	for _, id := range []string{"LizardByte.Sunshine", "MoonlightGameStreamingProject.Moonlight"} {
		if id == "LizardByte.Sunshine" && sunshine.Exe() != "" || id != "LizardByte.Sunshine" && sunshine.MoonlightExe() != "" {
			continue
		}
		if err := shell.Run(ctx, "winget", "install", "--id", id, "-e", "--silent", "--accept-package-agreements", "--accept-source-agreements"); err != nil {
			return err
		}
	}
	shell.RefreshPath()
	return nil
}

// startSunshine gives Sunshine spillup's login and runs its Windows service at boot: Sunshine keeps both
// under Program Files, so Windows asks for admin once.
func startSunshine(ctx context.Context, exe string, login sunshine.Login) error {
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	script := "& " + quote(exe) + " --creds " + quote(login.User) + " " + quote(login.Password) + " | Out-Null\n" +
		"Get-Service | Where-Object { $_.Name -like '*sunshine*' } | ForEach-Object { Set-Service $_.Name -StartupType Automatic; Restart-Service $_.Name -Force }"
	return shell.Elevated(ctx, "Sunshine's login and service", script)
}
