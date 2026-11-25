package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/darkyeg/spinup/internal/agentconfig"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/library"
	"github.com/darkyeg/spinup/internal/proxy"
	"github.com/darkyeg/spinup/internal/source"
	"github.com/darkyeg/spinup/internal/t3code"
	"github.com/darkyeg/spinup/internal/tailnet"
	"github.com/darkyeg/spinup/internal/tools"
	"github.com/darkyeg/spinup/internal/webui"
)

type setupChanges []webui.SetupChange

func (p *setupChanges) add(action, target, detail string) {
	*p = append(*p, webui.SetupChange{Action: action, Target: target, Detail: detail})
}

func previewSetup(ctx context.Context, name string, req serviceRequest) (webui.SetupPreview, error) {
	current, err := config.Load()
	fresh := errors.Is(err, config.ErrNotInstalled)
	if err != nil && !fresh {
		return webui.SetupPreview{}, fmt.Errorf("check %s before running setup", config.Path())
	}
	cfg := desiredServiceConfig(current, req)
	changes := setupChanges{}
	if name != "" && !tailnet.ValidName(name) {
		return webui.SetupPreview{}, errors.New("enter a valid Tailnet machine name")
	}
	bin := tailnet.Find()
	var st tailnet.Status
	var statusErr error
	if bin != "" {
		st, statusErr = (tailnet.CLI{Bin: bin}).Status(ctx)
	}
	if name == "" {
		name = st.Self.Name
		if name == "" {
			name = osMachineName()
		}
		if !tailnet.ValidName(name) {
			return webui.SetupPreview{}, errors.New("specify a valid Tailnet machine name")
		}
	}
	switch {
	case bin == "":
		changes.add("Install", "Tailscale", "Sign in and request the machine name "+name)
	case statusErr != nil:
		changes.add("Check", "Tailscale", "Current status unavailable. Setup checks the service before requesting "+name)
	case !st.Running:
		changes.add("Connect", "Tailscale", "Bring the service online and request "+name)
	case st.Self.Name != name:
		changes.add("Rename", "Tailscale hostname", st.Self.Name+" → "+name)
	}
	changes.service(current, cfg, fresh, req)
	changes.library(req.repo)
	if all, err := tools.Load(req.repo); err != nil {
		changes.add("Check", "tools.json", "Could not read the tool list")
	} else {
		for _, tool := range tools.Missing(all) {
			changes.add("Missing", tool.Name, "Setup checks the configured installer for this OS")
		}
	}
	changes.t3()
	if err := ctx.Err(); err != nil {
		return webui.SetupPreview{}, err
	}
	return webui.SetupPreview{Name: name, Role: cfg.Hold, Changes: changes,
		Deferred: "Setup checks live credentials, skills, native packages, remote desktop, T3 peer links and boot services when it runs."}, nil
}

func (p *setupChanges) service(current, wanted config.Config, fresh bool, req serviceRequest) {
	if fresh {
		p.add("Create", config.Path(), fmt.Sprintf("hold = %s, account router port = %d", wanted.Hold, wanted.Port))
	} else {
		if current.Hold != wanted.Hold {
			p.add("Set", config.Path()+" · hold", string(current.Hold)+" → "+string(wanted.Hold))
		}
		for _, field := range []struct{ name, before, after string }{
			{"tailscale", current.Tailscale, wanted.Tailscale}, {"repo", current.Repo, wanted.Repo},
			{"library", current.Library, wanted.Library}, {"home", current.Home, wanted.Home},
		} {
			if field.before != field.after {
				p.add("Set", config.Path()+" · "+field.name, field.after)
			}
		}
	}
	if _, err := config.LoadSecrets(wanted); err != nil || req.replacesKeys(wanted.Hold) {
		p.add("Configure", wanted.SecretsPath(), "Provision machine credentials for the selected role")
	}
	if wanted.Hold.CanHold() && !proxy.Installed(wanted) {
		p.add("Install", wanted.ProxyExe(), "CLIProxyAPI release selected at install time")
	}
	self, _ := os.Executable()
	dest := installedBinary()
	fi, err := os.Stat(dest)
	switch {
	case errors.Is(err, os.ErrNotExist):
		p.add("Install", dest, "Spillup "+version)
	case err != nil || !fi.Mode().IsRegular():
		p.add("Check", dest, "Unable to compare the installed Spillup binary")
	case !sameFile(self, dest) && !sameContent(self, dest):
		p.add("Replace", dest, "Stage this Spillup build ("+version+") for the service")
	}
}

func (p *setupChanges) library(repo source.Source) {
	lib := library.Here()
	if dir, err := repo.Checkout(); err == nil {
		old := filepath.Join(dir, oldPrivateRepo)
		names, err := lib.Adoptable(old)
		if err != nil {
			p.add("Check", lib.Dir(), "Unable to inspect the legacy library")
			return
		}
		for _, name := range names {
			p.add("Copy", lib.Path(name), "Adopt from the legacy library")
		}
		if len(names) > 0 {
			lib = library.At(old)
		}
	}
	files, err := agentconfig.Drifted(agentconfig.Sources{Spinup: repo, Yours: lib}, agentconfig.HostHomes())
	if err != nil {
		p.add("Check", "Agent configuration", "Unable to read the managed files or parse the configuration")
		return
	}
	for _, path := range files {
		action := "Update"
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			action = "Create"
		}
		p.add(action, path, "Install the managed agent configuration")
	}
}

func (p *setupChanges) t3() {
	t3, err := t3code.Find()
	if errors.Is(err, t3code.ErrNotInstalled) {
		p.add("Install", "T3 Code CLI", "Use the T3 installer")
	} else if err != nil {
		p.add("Check", "T3 Code", "Unable to find the T3 settings folder")
		return
	}
	changes, err := t3.SettingsDrift()
	for _, setting := range changes {
		p.add("Set", setting.Path+" · "+setting.Field, setting.Value)
	}
	if err != nil {
		p.add("Check", filepath.Join(t3.Home, "userdata"), "Unable to read the T3 settings as JSON objects")
	}
}

func (cliBackend) PreviewSetup(ctx context.Context, name, role string) (webui.SetupPreview, error) {
	var hold config.Hold
	switch role {
	case "hub", "standby":
		hold = config.Hold(role)
	case "use", "":
	default:
		return webui.SetupPreview{}, errors.New("unknown machine role")
	}
	return previewSetup(ctx, name, serviceRequest{hold: hold, repo: repoData()})
}
