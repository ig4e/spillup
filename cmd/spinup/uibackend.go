package main

import (
	"context"
	"errors"
	"runtime"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/doctor"
	"github.com/darkyeg/spinup/internal/skills"
	"github.com/darkyeg/spinup/internal/tailnet"
	"github.com/darkyeg/spinup/internal/webui"
)

// cliBackend gives the dashboard what the commands use, read fresh on every call.
type cliBackend struct{}

func (cliBackend) Facts() webui.Facts {
	cfg, err := config.Load()
	f := webui.Facts{Installed: err == nil, Config: cfg, Version: version, OS: runtime.GOOS}
	if f.Installed && cfg.Hold.CanHold() {
		s, err := config.LoadSecrets(cfg)
		f.CanManage = err == nil && s.ManagementPassword != ""
	}
	return f
}

func (cliBackend) Report(context.Context) (api.Report, error) {
	cfg, err := config.Load()
	if err != nil {
		return api.Report{}, err
	}
	return localService(cfg, "").report()
}

func (cliBackend) Tailnet(ctx context.Context) (tailnet.Status, error) {
	cfg, _ := config.Load()
	return tailnet.CLI{Bin: cfg.Tailscale}.Status(ctx)
}

func (cliBackend) Doctor(ctx context.Context) []doctor.Section { return doctorSections(ctx) }

func (cliBackend) Keys() (config.Secrets, error) {
	cfg, err := config.Load()
	if err != nil {
		return config.Secrets{}, err
	}
	return config.LoadSecrets(cfg)
}

func (cliBackend) HandOff(to string) error {
	return withKeyedService(func(l local) error { return l.handOff(to) })
}

func (cliBackend) TakeOver() error { return withKeyedService(local.takeOver) }

func (cliBackend) MakeMain(machine string) error {
	return withKeyedService(func(l local) error { return l.makeMain(machine) })
}

func (cliBackend) RestartProxy() error { return withKeyedService(local.restartProxy) }

func (cliBackend) Skills() skills.Manager { return skillManager(repoData()) }

func (cliBackend) CatchUpLibrary() { catchUpLibrary() }

var errNotInstalled = errors.New("spinup isn't set up on this machine yet: run `spinup setup <name>` first")

func withKeyedService(do func(local) error) error {
	l, err := keyedLocalService()
	if errors.Is(err, config.ErrNotInstalled) {
		return errNotInstalled
	}
	if err != nil {
		return err
	}
	return do(l)
}
