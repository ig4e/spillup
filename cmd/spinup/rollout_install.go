package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/filelock"
	"github.com/darkyeg/spinup/internal/release"
	"github.com/darkyeg/spinup/internal/sysproc"
)

var reportedVersion = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+_-]{0,79}$`)
var stableVersion = regexp.MustCompile(`^[0-9]{1,8}\.[0-9]{1,8}\.[0-9]{1,8}$`)

type rolloutInstallCmd struct {
	Version string `arg:""`
	Home    string `hidden:""`
}

func rolloutReady(ctx context.Context, target string) (api.Report, error) {
	if !stableVersion.MatchString(target) {
		return api.Report{}, errors.New("expected a stable release number")
	}
	cfg, err := config.Load()
	if err != nil {
		return api.Report{}, err
	}
	s, err := config.LoadSecrets(cfg)
	if err != nil {
		return api.Report{}, err
	}
	var r api.Report
	if err := (api.Client{HTTP: directMachineClient}).Get(ctx, localService(cfg, "").base+api.PathState, &r); err != nil {
		return r, err
	}
	if sameRelease(r.Version, target) {
		return r, nil
	}
	if !release.Newer(target, r.Version) {
		return r, errors.New("the requested version is not an upgrade")
	}
	return r, (rolloutFleet{cfg, s.APIKey}).guard(ctx, r)
}

func (cliBackend) StartRollout(ctx context.Context, target string) error {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	r, err := rolloutReady(ctx, target)
	if err != nil || sameRelease(r.Version, target) {
		return err
	}
	return sysproc.StartDetached(filepath.Join(config.StateDir(), "rollout.log"), installedBinary(), "rollout-install", target, "--home", config.StateDir())
}

func (c rolloutInstallCmd) Run() error {
	if c.Home != "" {
		if err := os.Setenv("SPINUP_HOME", c.Home); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	lockCtx, stop := context.WithTimeout(ctx, time.Second)
	lock, err := filelock.Acquire(lockCtx, filepath.Join(config.StateDir(), "update.lock"))
	stop()
	if err != nil {
		return fmt.Errorf("acquire update lock: %w", err)
	}
	defer lock.Close()
	r, err := rolloutReady(ctx, c.Version)
	if err != nil || sameRelease(r.Version, c.Version) {
		return err
	}
	rel, err := release.Latest(ctx, releases)
	if err != nil {
		return err
	}
	if rel.Version != c.Version {
		return errors.New("latest release has changed; preview the fleet rollout again")
	}
	data, err := rel.Fetch(ctx, releaseAsset())
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	r, err = rolloutReady(ctx, c.Version)
	if err != nil || sameRelease(r.Version, c.Version) {
		return err
	}
	exe := installedBinary()
	if err := replaceRunningBinary(exe, data); err != nil {
		return err
	}

	startErr := restartServiceNow(cfg)
	check, stop := context.WithTimeout(context.Background(), 45*time.Second)
	defer stop()
	for check.Err() == nil {
		if sameRelease(serviceVersion(cfg), c.Version) {
			refreshLauncherFrom(exe)
			fmt.Printf("Service verified on v%s\n", c.Version)
			return nil
		}
		sleepCtx(check, time.Second)
	}
	if startErr == nil {
		startErr = errors.New("the service did not report the requested version")
	}
	return errors.Join(startErr, recoverRollout(cfg, r, exe))
}

func recoverRollout(cfg config.Config, prior api.Report, exe string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	secrets, err := config.LoadSecrets(cfg)
	if err != nil {
		return err
	}
	fleet := rolloutFleet{cfg, secrets.APIKey}
	if err := fleet.guard(ctx, prior); err != nil {
		return fmt.Errorf("rollback deferred: %w; previous binary at %s.old", err, exe)
	}
	if live, err := localService(cfg, "").report(); err == nil && (live.Leading || live.Starting) {
		return errors.New("rollback deferred because this machine now holds the accounts")
	}
	if err := restoreRolloutBinary(exe); err != nil {
		return fmt.Errorf("restore previous release: %w", err)
	}
	if err := restartServiceNow(cfg); err != nil {
		return fmt.Errorf("previous binary restored, restart failed: %w", err)
	}
	if !pollUntil(30*time.Second, func() bool { return sameRelease(serviceVersion(cfg), prior.Version) }) {
		return fmt.Errorf("previous binary restored, the service has not confirmed %s", prior.Version)
	}
	return fmt.Errorf("restored service on %s", prior.Version)
}

func restoreRolloutBinary(exe string) error {
	if _, err := os.Stat(exe + ".old"); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(exe), ".rollout-recovery-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	failed := filepath.Join(tmp, "new-binary")
	if err := os.Rename(exe, failed); err != nil {
		return err
	}
	if err := os.Rename(exe+".old", exe); err != nil {
		return errors.Join(err, os.Rename(failed, exe))
	}
	_ = os.Remove(failed)
	return nil
}
