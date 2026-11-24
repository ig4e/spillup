package main

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/darkyeg/spinup/internal/shell"
	"github.com/darkyeg/spinup/internal/sunshine"
)

// installRemotePackages installs Sunshine and Moonlight with the system's packages: pacman (Sunshine is in
// some Arch repos, the AUR otherwise), or Flatpak for Moonlight elsewhere.
func installRemotePackages(ctx context.Context) error {
	if _, ok := shell.Find("pacman"); ok {
		if sunshine.MoonlightExe() == "" {
			if err := shell.AsRoot(ctx, "pacman", "-S", "--needed", "--noconfirm", "moonlight-qt"); err != nil {
				return err
			}
		}
		if sunshine.Exe() != "" {
			return nil
		}
		if err := shell.AsRoot(ctx, "pacman", "-S", "--needed", "--noconfirm", "sunshine"); err == nil {
			return nil
		}
		for _, helper := range []string{"yay", "paru"} {
			if _, ok := shell.Find(helper); ok {
				return shell.Run(ctx, helper, "-S", "--needed", "--noconfirm", "sunshine")
			}
		}
		return errors.New("Sunshine isn't in your pacman repositories: install it from the AUR (yay -S sunshine), then set remote desktop up again")
	}
	if _, ok := shell.Find("flatpak"); ok {
		if err := shell.Run(ctx, "flatpak", "install", "-y", "--user", "flathub", "com.moonlight_stream.Moonlight"); err != nil {
			return err
		}
		return errors.New("Moonlight is installed; install Sunshine for your distribution (docs.lizardbyte.dev), then set remote desktop up again")
	}
	return errors.New("install Sunshine and Moonlight with your distribution's packages, then set remote desktop up again")
}

// startSunshine gives Sunshine spillup's login and runs its systemd user service at boot.
func startSunshine(ctx context.Context, exe string, login sunshine.Login) error {
	if err := shell.Run(ctx, exe, "--creds", login.User, login.Password); err != nil {
		return err
	}
	unit := sunshineUnit()
	if unit == "" {
		return errors.New("Sunshine has no systemd user service here: start it from your desktop's autostart")
	}
	if err := run("systemctl", "--user", "enable", unit); err != nil {
		return err
	}
	return run("systemctl", "--user", "restart", unit)
}

func sunshineUnit() string {
	out, err := exec.Command("systemctl", "--user", "list-unit-files", "--no-legend", "--type=service").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if f := strings.Fields(line); len(f) > 0 && strings.Contains(strings.ToLower(f[0]), "sunshine") {
			return f[0]
		}
	}
	return ""
}
