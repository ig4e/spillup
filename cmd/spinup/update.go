package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/host"
	"github.com/darkyeg/spinup/internal/proxy"
	"github.com/darkyeg/spinup/internal/release"
	"github.com/darkyeg/spinup/internal/tailnet"
)

type updateCmd struct{}

func (updateCmd) Help() string {
	return `Updates spinup itself and, on a hub or standby, CLIProxyAPI (both checksum-verified from their
GitHub releases), then your skills and Tailscale. The accounts stay where they are.`
}

func (updateCmd) Run() error {
	ctx := context.Background()
	removeReplacedBinary()
	newer, err := updateSelf(ctx)
	switch {
	case err != nil:
		step("Couldn't update spinup: %v", err)
	case newer != "":
		return runUpdated(newer)
	}
	var failed []string
	for _, part := range []struct {
		name string
		run  func(context.Context) error
	}{
		{"accounts service", refreshService},
		{"terminal copy", func(context.Context) error { refreshLauncherCopy(); return nil }},
		{"CLIProxyAPI", updateProxy},
		{"skills", func(ctx context.Context) error { return syncSkills(ctx, repoData()) }},
		{"packages", syncPackages},
		{"Tailscale", tailnet.Update},
	} {
		if err := part.run(ctx); err != nil {
			step("%s: %v", part.name, err)
			failed = append(failed, part.name)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("not updated: %s", strings.Join(failed, ", "))
	}
	return nil
}

// updateSelf replaces this binary with the latest release and returns its path, or "" when already current.
func updateSelf(ctx context.Context) (string, error) {
	if version == "dev" {
		step("spinup was built from source; update it with git pull and go install ./cmd/spinup")
		return "", nil
	}
	rel, err := release.Latest(ctx, releases)
	if err != nil {
		return "", err
	}
	if !release.Newer(rel.Version, version) {
		step("spinup %s is the latest", version)
		return "", nil
	}
	data, err := rel.Fetch(ctx, releaseAsset())
	if err != nil {
		return "", err
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if err := replaceRunningBinary(exe, data); err != nil {
		return "", err
	}
	step("spinup %s -> %s", version, rel.Version)
	return exe, nil
}

func releaseAsset() string {
	name := fmt.Sprintf("spinup_%s_%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// replaceRunningBinary renames the running binary aside, which Windows allows, and puts the new one in its place.
func replaceRunningBinary(exe string, data []byte) error {
	if err := os.WriteFile(exe+".new", data, 0o755); err != nil {
		return err
	}
	_ = os.Remove(exe + ".old")
	if err := os.Rename(exe, exe+".old"); err != nil {
		return err
	}
	if err := os.Rename(exe+".new", exe); err != nil {
		return errors.Join(err, os.Rename(exe+".old", exe))
	}
	return nil
}

func removeReplacedBinary() {
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old")
	}
}

// runUpdated finishes the update with the new binary, so the rest runs the new code.
func runUpdated(exe string) error {
	cmd := exec.Command(exe, "update")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	if exit := (*exec.ExitError)(nil); errors.As(err, &exit) {
		return childExit{code: exit.ExitCode()}
	}
	return err
}

// childExit means the updated spinup ran and already printed why it failed.
type childExit struct{ code int }

func (e childExit) Error() string {
	return fmt.Sprintf("the updated spinup exited with code %d", e.code)
}

// refreshService puts the accounts service on this binary. It goes by the version the running service
// reports, not the file it started from: an update the service ran replaced that file while the old
// code kept running, and comparing files then says there is nothing to do.
func refreshService(ctx context.Context) error {
	cfg, err := config.Load()
	if errors.Is(err, config.ErrNotInstalled) {
		return nil
	}
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	running := serviceVersion(cfg)
	if sameContent(self, installedBinary()) && (running == "" || running == version) {
		return nil
	}
	if interactive() {
		step("Restarting the accounts service on spinup %s", version)
		return installService(ctx, serviceRequest{repo: repoData()})
	}
	if !sameFile(self, installedBinary()) {
		data, err := os.ReadFile(self)
		if err != nil {
			return err
		}
		if err := replaceRunningBinary(installedBinary(), data); err != nil {
			return err
		}
	}
	step("Restarting the accounts service on spinup %s (it ran %s)", version, orNone(running, "nothing"))
	return restartServiceNow(cfg)
}

// refreshLauncherCopy keeps the spinup you run in a terminal in step when the service updated itself.
func refreshLauncherCopy() {
	self, err := os.Executable()
	if err != nil || !sameFile(self, installedBinary()) {
		return
	}
	name := "spinup"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	launcher := filepath.Join(host.Home(), ".local", "bin", name)
	if _, err := os.Stat(launcher); err != nil || sameContent(self, launcher) {
		return
	}
	if data, err := os.ReadFile(self); err == nil && replaceRunningBinary(launcher, data) == nil {
		step("Updated the spinup in %s too", filepath.Dir(launcher))
	}
}

func updateProxy(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil || !cfg.Hold.CanHold() {
		return nil
	}
	rel, err := release.Latest(ctx, proxy.Project)
	if err != nil {
		return err
	}
	current := proxy.InstalledVersion(cfg)
	if rel.Version == current {
		step("CLIProxyAPI %s is the latest", current)
		return nil
	}
	if err := proxy.Install(ctx, cfg, rel); err != nil {
		return err
	}
	step("CLIProxyAPI %s -> %s", orNone(current, "unknown"), rel.Version)
	service, err := keyedLocalService()
	if err != nil {
		return err
	}
	return service.restartProxy()
}

func sameContent(a, b string) bool {
	ha, errA := fileHash(a)
	hb, errB := fileHash(b)
	return errA == nil && errB == nil && bytes.Equal(ha, hb)
}

func fileHash(path string) ([]byte, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
