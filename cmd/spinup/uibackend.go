package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"time"

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

func (cliBackend) Remote(machine string) (webui.Remote, error) {
	cfg, err := config.Load()
	if err != nil {
		return webui.Remote{}, errNotInstalled
	}
	s, err := config.LoadSecrets(cfg)
	if err != nil {
		return webui.Remote{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	st, err := tailnet.CLI{Bin: cfg.Tailscale}.Status(ctx)
	if err != nil {
		return webui.Remote{}, fmt.Errorf("Tailscale: %w", err)
	}
	node, ok := st.Peer(machine)
	if !ok || node.IP == "" {
		return webui.Remote{}, fmt.Errorf("%s isn't on your tailnet", machine)
	}
	h := http.Header{}
	if s.ManagementPassword != "" {
		h.Set(api.KeyHeader, s.ManagementPassword)
	}
	h.Set(api.APIKeyHeader, s.APIKey)
	return webui.Remote{Base: &url.URL{Scheme: "http", Host: net.JoinHostPort(node.IP, strconv.Itoa(cfg.Port))}, Header: h}, nil
}

// Panel is a CLIProxyAPI that spillup itself runs, never just whatever answers on a port: a different
// proxy there, such as an old standalone CLIProxyAPI, would be sent a key it doesn't know until it bans
// this machine.
func (cliBackend) Panel(target string) (webui.Panel, error) {
	cfg, err := config.Load()
	if err != nil {
		return webui.Panel{}, errNotInstalled
	}
	s, err := config.LoadSecrets(cfg)
	if err != nil {
		return webui.Panel{}, err
	}
	rep, err := localService(cfg, "").report()
	if err != nil {
		return webui.Panel{}, fmt.Errorf("spillup's accounts service isn't answering on port %d: is another program, such as an old CLIProxyAPI, using it? Stop that program, then run `spinup setup` again", cfg.Port)
	}
	switch target {
	case "own":
		if !rep.OwnRunning {
			return webui.Panel{}, errors.New("this machine's own CLIProxyAPI isn't running: turn on a separate sign-in under Accounts first")
		}
		return webui.Panel{Port: cfg.OwnProxyPort, Key: s.APIKey}, nil
	case "shared":
		if s.ManagementPassword == "" {
			return webui.Panel{}, errors.New("only the main machine and standbys keep the password for the shared accounts' dashboard")
		}
		if !rep.Leading {
			if rep.Leader == "" {
				return webui.Panel{}, errors.New("no machine holds the accounts right now, so there is no shared CLIProxyAPI to show")
			}
			return webui.Panel{}, fmt.Errorf("the shared accounts' CLIProxyAPI runs on %s, not here: open it there", rep.Leader)
		}
		return webui.Panel{Port: cfg.ProxyPort, Key: s.ManagementPassword}, nil
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

// knownApps are the AI apps whose hints the dashboard tailors: each found on PATH by its command.
var knownApps = []struct{ id, name, command string }{
	{"claude", "Claude Code", "claude"},
	{"codex", "Codex", "codex"},
	{"t3", "T3 Code", "t3"},
	{"gemini", "Gemini CLI", "gemini"},
	{"opencode", "OpenCode", "opencode"},
	{"cursor", "Cursor Agent", "cursor-agent"},
	{"amp", "Amp", "amp"},
	{"droid", "Factory Droid", "droid"},
	{"ccp", "ccp (Claude Code with the shared accounts)", "ccp"},
}

func (cliBackend) Apps() []webui.App {
	out := make([]webui.App, 0, len(knownApps))
	for _, a := range knownApps {
		_, err := exec.LookPath(a.command)
		out = append(out, webui.App{ID: a.id, Name: a.name, Installed: err == nil})
	}
	return out
}
