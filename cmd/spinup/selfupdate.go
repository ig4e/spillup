package main

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/release"
	"github.com/darkyeg/spinup/internal/sysproc"
	"github.com/darkyeg/spinup/internal/webui"
)

// autoUpdateEvery is how often the service looks for a new spillup release.
const autoUpdateEvery = 6 * time.Hour

func (cliBackend) UpdateState(ctx context.Context) webui.Update {
	cfg, _ := config.Load()
	return webui.Update{Current: version, Latest: latestVersion(ctx, releases), Auto: cfg.AutoUpdate}
}

func (cliBackend) StartUpdate() error {
	if version == "dev" {
		return errors.New("this spillup was built from source; update it with git pull and go build")
	}
	return startDetachedUpdate()
}

func (cliBackend) SetAutoUpdate(auto bool) error {
	cfg, err := config.Load()
	if err != nil {
		return errNotInstalled
	}
	cfg.AutoUpdate = auto
	return config.Save(cfg)
}

// startDetachedUpdate runs `spinup update` outside the service, which the update restarts.
func startDetachedUpdate() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return sysproc.StartDetached(filepath.Join(config.StateDir(), "update.log"), exe, "update")
}

// keepUpdated installs new releases while config.json allows it, trying each version once per run so a
// failing update doesn't repeat every few hours.
func keepUpdated(ctx context.Context, logger *log.Logger) {
	if version == "dev" {
		return
	}
	tried := ""
	for sleepCtx(ctx, 10*time.Minute); ctx.Err() == nil; sleepCtx(ctx, autoUpdateEvery) {
		cfg, err := config.Load()
		if err != nil || !cfg.AutoUpdate {
			continue
		}
		rel, err := release.Latest(ctx, releases)
		if err != nil || !release.Newer(rel.Version, version) || rel.Version == tried {
			continue
		}
		tried = rel.Version
		logger.Printf("updating spillup %s → %s", version, rel.Version)
		if err := startDetachedUpdate(); err != nil {
			logger.Printf("update: %v", err)
		}
	}
}
