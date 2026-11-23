package main

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/doctor"
	"github.com/darkyeg/spinup/internal/logins"
	"github.com/darkyeg/spinup/internal/proxy"
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

func (cliBackend) OwnLogins() ([]logins.Summary, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return logins.Summarize(cfg.OwnAuthDir), nil
}

func (cliBackend) SetOwn(ctx context.Context, families []string) error {
	cfg, err := config.Load()
	if errors.Is(err, config.ErrNotInstalled) {
		return errNotInstalled
	}
	if err != nil {
		return err
	}
	s, err := config.LoadSecrets(cfg)
	if err != nil {
		return err
	}
	if len(families) > 0 {
		if err := installProxy(ctx, cfg); err != nil {
			return err
		}
	}
	l := localService(cfg, "")
	l.client.APIKey = s.APIKey
	return l.setOwn(families)
}

func (cliBackend) OwnManagement() (proxy.Management, error) {
	cfg, err := config.Load()
	if err != nil {
		return proxy.Management{}, err
	}
	s, err := config.LoadSecrets(cfg)
	if err != nil {
		return proxy.Management{}, err
	}
	return proxy.Management{Port: cfg.OwnProxyPort, Key: s.APIKey}, nil
}

func (cliBackend) Panel(target string) (webui.Panel, error) {
	cfg, err := config.Load()
	if err != nil {
		return webui.Panel{}, errNotInstalled
	}
	s, err := config.LoadSecrets(cfg)
	if err != nil {
		return webui.Panel{}, err
	}
	switch target {
	case "own":
		return webui.Panel{Port: cfg.OwnProxyPort, Key: s.APIKey}, nil
	case "shared":
		if s.ManagementPassword == "" {
			return webui.Panel{}, errors.New("only the main machine and standbys keep the password for the shared accounts' dashboard")
		}
		return webui.Panel{Port: cfg.Port, Key: s.ManagementPassword}, nil
	}
	return webui.Panel{}, fmt.Errorf("no CLIProxyAPI called %q", target)
}

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
