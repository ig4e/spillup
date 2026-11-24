package main

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"

	"github.com/darkyeg/spinup/internal/release"
	"github.com/darkyeg/spinup/internal/remoteweb"
	"github.com/darkyeg/spinup/internal/shell"
	"github.com/darkyeg/spinup/internal/sunshine"
	"github.com/darkyeg/spinup/internal/t3code"
)

// systemManaged reports whether the system's package manager owns Tailscale, Sunshine and Moonlight here:
// Arch supports only full system updates, so they update with the system.
func systemManaged() bool {
	_, pacman := shell.Find("pacman")
	return runtime.GOOS == "linux" && pacman
}

// updateT3 updates T3 Code and restarts its service, unless the service started this update: restarting
// T3 stops its running agent turns, so that waits for an update you start.
func updateT3(ctx context.Context, auto bool) error {
	t3, err := t3code.Find()
	if err != nil {
		return nil
	}
	if auto {
		step("T3 Code: left for an update you start (restarting it stops running agents)")
		return nil
	}
	return shell.Run(ctx, t3.Bin, "update", "--yes")
}

// updateRemote updates moonlight-web, and Sunshine and Moonlight where their installer allows it.
func updateRemote(ctx context.Context, auto bool) error {
	in := remoteInstall()
	if in.Installed() {
		if rel, err := release.Latest(ctx, remoteweb.Project); err == nil && rel.Version != in.Version() {
			if v, err := in.Download(ctx); err != nil {
				return err
			} else {
				step("moonlight-web %s -> %s (the service picks it up when it restarts)", orNone(in.Version(), "unknown"), v)
			}
		} else if err == nil {
			step("moonlight-web %s is the latest", rel.Version)
		}
	}
	if auto || systemManaged() {
		return nil
	}
	switch runtime.GOOS {
	case "windows":
		for _, id := range []string{"LizardByte.Sunshine", "MoonlightGameStreamingProject.Moonlight"} {
			if id == "LizardByte.Sunshine" && sunshine.Exe() == "" || id != "LizardByte.Sunshine" && sunshine.MoonlightExe() == "" {
				continue
			}
			err := shell.Run(ctx, "winget", "upgrade", "--id", id, "-e", "--silent", "--accept-package-agreements", "--accept-source-agreements")
			if exit := (*exec.ExitError)(nil); err != nil && !(asExit(err, &exit) && wingetNothingNew(exit.ExitCode())) {
				return err
			}
		}
	case "darwin":
		if sunshine.MoonlightExe() != "" {
			_ = shell.Run(ctx, "brew", "upgrade", "--cask", "moonlight")
		}
	}
	return nil
}

// wingetNoUpgrade (0x8A15002B) is winget saying the package is already current.
func wingetNothingNew(code int) bool { return int32(code) == -1978335189 }

// pendingSystemUpdates names the system packages that can update, on systems spillup knows how to ask.
func pendingSystemUpdates(ctx context.Context) []string {
	if !systemManaged() {
		return nil
	}
	out, err := exec.CommandContext(ctx, "checkupdates").Output()
	if err != nil {
		return nil
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if f := strings.Fields(line); len(f) > 0 {
			names = append(names, f[0])
		}
	}
	return names
}

// updateSystem runs a full system update when asked to, and otherwise says what's waiting.
func updateSystem(ctx context.Context, run bool) error {
	if !systemManaged() {
		return nil
	}
	if !run {
		if pending := pendingSystemUpdates(ctx); len(pending) > 0 {
			step("%d system packages can update%s: Updates → Update System, or spinup update --system", len(pending), notable(pending))
		}
		return nil
	}
	step("Updating the system (pacman -Syu)")
	return shell.AsRoot(ctx, "pacman", "-Syu", "--noconfirm")
}

// notable names the packages spillup set up among those waiting.
func notable(pending []string) string {
	var ours []string
	for _, p := range pending {
		switch p {
		case "tailscale", "sunshine", "moonlight-qt":
			ours = append(ours, p)
		}
	}
	if len(ours) == 0 {
		return ""
	}
	return ", including " + strings.Join(ours, ", ")
}

func asExit(err error, target **exec.ExitError) bool { return errors.As(err, target) }
