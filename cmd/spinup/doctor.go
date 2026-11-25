package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/darkyeg/spinup/internal/agentconfig"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/doctor"
	"github.com/darkyeg/spinup/internal/proxy"
	"github.com/darkyeg/spinup/internal/release"
	"github.com/darkyeg/spinup/internal/source"
	"github.com/darkyeg/spinup/internal/t3code"
	"github.com/darkyeg/spinup/internal/tailnet"
	"github.com/darkyeg/spinup/internal/tools"
	"github.com/darkyeg/spinup/internal/webui"
)

type doctorCmd struct {
	JSON bool `help:"Print checks as JSON for scripts and fleet monitoring."`
}

func (doctorCmd) Help() string {
	return `Checks this machine: tools, Tailscale and the paths to your other machines, the accounts,
skills and agent config, and whether spinup itself is up to date. Every problem
comes with the command that fixes it.`
}

func (cmd doctorCmd) Run() error {
	sections := doctorSections(context.Background())
	if cmd.JSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(webui.DoctorReport(sections)); err != nil {
			return err
		}
	} else {
		printSections(sections)
	}
	if n := doctor.Failures(sections); n > 0 {
		return fmt.Errorf("%d problem(s); see the reported fixes", n)
	}
	if !cmd.JSON {
		fmt.Println("\nAll good.")
	}
	return nil
}

// doctorSections runs independent checks concurrently and returns them in display order.
func doctorSections(ctx context.Context) []doctor.Section {
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	repo := repoData()
	checks := []func() doctor.Section{
		func() doctor.Section { return toolsSection(repo) },
		func() doctor.Section { return networkSection(ctx) },
		func() doctor.Section { return accountsSection(ctx) },
		func() doctor.Section { return agentsSection(repo) },
		func() doctor.Section { return packagesSection(ctx) },
		func() doctor.Section { return proxySection(ctx) },
		func() doctor.Section { return t3Section(ctx) },
		func() doctor.Section { return doctor.Update(version, latestVersion(ctx, releases)) },
	}
	sections := make([]doctor.Section, len(checks))
	var wg sync.WaitGroup
	for i, check := range checks {
		wg.Go(func() { sections[i] = check() })
	}
	wg.Wait()
	return sections
}

func printSections(sections []doctor.Section) {
	marks := map[doctor.Level]string{doctor.OK: "[ok]", doctor.Warn: "[!!]", doctor.Fail: "[XX]"}
	for i, s := range sections {
		if len(s.Checks) == 0 {
			continue
		}
		if i > 0 {
			fmt.Println()
		}
		fmt.Println(s.Title)
		for _, c := range s.Checks {
			fmt.Printf("  %s %s\n", marks[c.Level], c.Label)
			if c.Fix != "" {
				fmt.Printf("       fix: %s\n", c.Fix)
			}
		}
	}
}

func toolsSection(repo source.Source) doctor.Section {
	all, err := tools.Load(repo)
	if err != nil {
		return doctor.Section{Title: "Tools", Checks: []doctor.Check{{ID: "tools.install", Level: doctor.Fail, Label: err.Error()}}}
	}
	return doctor.Tools(toolNames(tools.Missing(all)), len(all))
}

func networkSection(ctx context.Context) doctor.Section {
	cli := tailnet.CLI{}
	st, _ := cli.Status(ctx)
	paths := map[string]tailnet.Path{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range st.Peers {
		if !p.Online {
			continue
		}
		wg.Go(func() {
			if path, err := cli.PathTo(ctx, p.Name); err == nil {
				mu.Lock()
				paths[p.Name] = path
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	return doctor.Network(st, paths, time.Now())
}

func accountsSection(ctx context.Context) doctor.Section {
	cfg, err := config.Load()
	if errors.Is(err, config.ErrNotInstalled) {
		return doctor.Accounts(doctor.Service{}, cfg.Port)
	}
	svc := doctor.Service{Installed: true}
	if r, err := localService(cfg, "").report(); err == nil {
		svc.Report = &r
	} else {
		svc.Problem = err.Error()
	}
	if cfg.Hold.CanHold() {
		svc.ProxyVersion = proxy.InstalledVersion(cfg)
		svc.LoginOnly = runtime.GOOS == "darwin"
		svc.ProxyLatest = latestVersion(ctx, proxy.Project)
	}
	return doctor.Accounts(svc, cfg.Port)
}

func agentsSection(repo source.Source) doctor.Section {
	manager := skillManager(repo)
	list, err := manager.List()
	if err != nil {
		return doctor.Section{Title: "Agents", Checks: []doctor.Check{{ID: "agents.skills", Level: doctor.Fail, Label: "skills: " + err.Error()}}}
	}
	var notInstalled []string
	for _, s := range list {
		if !s.Installed {
			notInstalled = append(notInstalled, s.Name)
		}
	}
	unlisted, err := manager.Unlisted()
	if err != nil {
		return doctor.Section{Title: "Agents", Checks: []doctor.Check{{ID: "agents.skills", Level: doctor.Fail, Label: "skills: " + err.Error()}}}
	}
	drifted, err := agentconfig.Drifted(agentSources(repo), agentconfig.HostHomes())
	if err != nil {
		drifted = []string{err.Error()}
	}
	return doctor.Agents(notInstalled, unlisted, drifted, len(list))
}

// latestVersion is the newest release of a GitHub project, or "" when it can't be read.
func latestVersion(ctx context.Context, repo string) string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	rel, err := release.Latest(ctx, repo)
	if err != nil {
		return ""
	}
	return rel.Version
}

func t3Section(ctx context.Context) doctor.Section {
	t3, err := t3code.Find()
	if err != nil {
		return doctor.T3(doctor.T3Facts{App: t3code.AppPresent(t3.Home)})
	}
	f := doctor.T3Facts{Installed: true, App: true, ServiceAtBoot: t3.ServiceAtBoot(ctx), LinkingOn: t3.LinkingOn(), NetworkAccess: t3.NetworkAccess(), AwaitsRestart: t3.AwaitsRestart(selfTailnetIP(ctx)), StaleQuotaKeys: staleT3QuotaSources(ctx, t3)}
	linked, err := t3.Linked(ctx)
	if err != nil {
		f.Problem = err.Error()
	}
	f.Linked = t3code.LinkCount(linked)
	return doctor.T3(f)
}

func proxySection(ctx context.Context) doctor.Section {
	cfg, err := config.Load()
	if err != nil {
		return doctor.Section{}
	}
	f := doctor.ProxyFacts{Port: cfg.Port, OwnProviders: cfg.OwnModels}
	r, err := localService(cfg, "").report()
	if err != nil {
		f.ForeignOnPort = proxy.AnswersOn(ctx, cfg.Port)
		return doctor.Proxy(f)
	}
	f.Leading, f.Running, f.OwnRunning = r.Leading, r.ProxyRunning, r.OwnRunning
	if r.Leading && r.ProxyRunning {
		if s, err := config.LoadSecrets(cfg); err == nil {
			if _, err := proxy.LoginStates(ctx, cfg.ProxyPort, s.ManagementPassword); err != nil {
				f.KeyProblem = err.Error()
			}
		}
	}
	return doctor.Proxy(f)
}
