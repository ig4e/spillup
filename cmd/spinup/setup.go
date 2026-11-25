package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/tailnet"
)

type setupCmd struct {
	Plan     bool   `help:"Preview setup and the selected role without making changes."`
	Name     string `arg:"" optional:"" help:"This machine's name on your tailnet; other machines reach it by it (asked when omitted)."`
	Hub      bool   `xor:"hold" help:"This machine normally holds the accounts. One hub per tailnet."`
	Standby  bool   `xor:"hold" help:"This machine takes the accounts while the hub is off."`
	Password string `env:"SPINUP_PASSWORD" placeholder:"PASSWORD" help:"The dashboard password, for --standby (asked when omitted; on the hub: spinup keys)."`
	APIKey   string `name:"api-key" env:"SPINUP_API_KEY" placeholder:"KEY" help:"The API key, for a machine that only uses the accounts (asked when omitted; on the hub: spinup keys)."`
}

func (setupCmd) Help() string {
	return `Sets up the whole machine: Tailscale, the accounts service, dev tools, skills and agent
config, then checks it all. Without --hub or --standby the machine only uses the accounts.
Re-running setup keeps the machine's hold and repairs everything else.`
}

// setupStep is one part of setup; a needed step stops setup when it fails, the others are reported at the end.
type setupStep struct {
	title  string
	needed bool
	run    func(context.Context) error
}

func (c setupCmd) Run() error {
	ctx := context.Background()
	if c.Plan && strings.TrimSpace(c.Name) == "" {
		c.Name = currentMachineName(ctx)
	}
	repo := repoData()
	name, err := c.machineName(ctx)
	if err != nil {
		return err
	}
	steps := []setupStep{
		{"Tailscale", true, func(ctx context.Context) error { return tailnet.Setup(ctx, name) }},
		{"Accounts service", true, func(ctx context.Context) error {
			return installService(ctx, serviceRequest{hold: c.hold(), password: c.Password, apiKey: c.APIKey, repo: repo})
		}},
		{"Dev tools", false, func(ctx context.Context) error { return installTools(ctx, repo) }},
		{"Skills", false, func(ctx context.Context) error { return syncSkills(ctx, repo) }},
		{"Packages", false, syncPackages},
		{"Agent config", false, func(context.Context) error { return installAgentConfig(repo) }},
		{"Remote desktop", false, setUpRemote},
		{"T3 Code", false, setUpT3},
	}
	if c.Plan {
		cfg, err := serviceConfig(serviceRequest{hold: c.hold(), repo: repo})
		if err != nil {
			return err
		}
		fmt.Printf("Setup preview for %s (%s)\n", name, holdPhrase(cfg.Hold))
		for i, step := range steps {
			requirement := "report errors and continue"
			if step.needed {
				requirement = "stop on error"
			}
			fmt.Printf("  %d. %s (%s)\n", i+1, step.title, requirement)
		}
		fmt.Printf("  %d. Check setup\n", len(steps)+1)
		return nil
	}
	var failed []string
	for i, s := range steps {
		fmt.Printf("\n[%d/%d] %s\n", i+1, len(steps)+1, s.title)
		stepCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		err := s.run(stepCtx)
		if stepCtx.Err() != nil {
			err = stepCtx.Err()
		}
		cancel()
		if err != nil {
			if s.needed {
				return fmt.Errorf("%s: %w", s.title, err)
			}
			fmt.Printf("    %s failed: %v\n", s.title, err)
			failed = append(failed, s.title)
		}
	}
	fmt.Printf("\n[%d/%d] Check setup\n", len(steps)+1, len(steps)+1)
	printAddresses(ctx, name)
	fmt.Println()
	if err := (doctorCmd{}).Run(); err != nil {
		return err
	}
	if len(failed) > 0 {
		return fmt.Errorf("these steps failed: %s (re-run spinup setup after fixing them)", strings.Join(failed, ", "))
	}
	return nil
}

func (c setupCmd) hold() config.Hold {
	switch {
	case c.Hub:
		return config.HoldHub
	case c.Standby:
		return config.HoldStandby
	}
	return ""
}

func (c setupCmd) machineName(ctx context.Context) (string, error) {
	current := currentMachineName(ctx)
	name := strings.ToLower(strings.TrimSpace(c.Name))
	if name == "" && interactive() {
		fmt.Println("This machine's name is its address on your tailnet: other machines reach it as <name>\n" +
			"(http://<name>:8317, ssh <name>). Lowercase letters, digits and dashes.")
		name = strings.ToLower(askLine(fmt.Sprintf("Name for this machine [%s]: ", current)))
	}
	if name == "" {
		name = current
	}
	if !tailnet.ValidName(name) {
		return "", fmt.Errorf("%q can't be a machine name: use lowercase letters, digits and dashes (e.g. office-pc)", name)
	}
	return name, nil
}

var notNameChars = regexp.MustCompile(`[^a-z0-9-]+`)

// currentMachineName is the Tailscale name when there is one, else the OS hostname made valid.
func currentMachineName(ctx context.Context) string {
	if st, err := (tailnet.CLI{}).Status(ctx); err == nil && st.Self.Name != "" {
		return st.Self.Name
	}
	hostname, _ := os.Hostname()
	return strings.Trim(notNameChars.ReplaceAllString(strings.ToLower(hostname), "-"), "-")
}

func printAddresses(ctx context.Context, name string) {
	if st, err := (tailnet.CLI{}).Status(ctx); err == nil && st.Self.Name != "" {
		name = st.Self.Name
	}
	fmt.Printf("This machine is %s on your tailnet. From your other machines:\n", name)
	fmt.Printf("  any service:  http://%s:<port>\n", name)
	fmt.Printf("  ssh:          ssh <user>@%s\n", name)
	fmt.Println("  all machines: spinup machines")
}
