package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/sshd"
	"github.com/darkyeg/spinup/internal/tailnet"
	"github.com/darkyeg/spinup/internal/webui"
)

// sshSyncEvery is how often each machine refreshes the keys it lets in and the machines ~/.ssh/config names.
const sshSyncEvery = 10 * time.Minute

func tailscaleBin() string {
	cfg, _ := config.Load()
	if cfg.Tailscale != "" {
		return cfg.Tailscale
	}
	return tailnet.Find()
}

func (cliBackend) SSHState(ctx context.Context) webui.SSHState {
	st := webui.SSHState{Server: sshd.Kind(), On: sshd.On(ctx, tailscaleBin()), User: sshd.User(), Hosts: sshHosts()}
	if ts, err := (tailnet.CLI{Bin: tailscaleBin()}).Status(ctx); err == nil {
		st.Address = strings.TrimSuffix(ts.Self.DNSName, ".")
	}
	key, err := sshd.PublicKey(ctx)
	if err != nil {
		st.Problem = err.Error()
	}
	st.Key = key
	return st
}

// SSHEnable turns this machine's SSH server on or off as a dashboard task, which asks for your password or admin.
func (cliBackend) SSHEnable(on bool, actionID string) error {
	return startTask([]string{"ssh", map[bool]string{true: "on", false: "off"}[on]}, actionID)
}

func (cliBackend) SSHSync(ctx context.Context) (string, error) { return syncSSH(ctx) }

// sshHosts are the machines spillup named in ~/.ssh/config.
func sshHosts() []string {
	named.Lock()
	defer named.Unlock()
	return append([]string{}, named.hosts...)
}

// named are the machines the last sync put in ~/.ssh/config.
var named struct {
	sync.Mutex
	hosts []string
}

// syncSSH asks each of your online machines who to sign in as and which key it has, then names them in
// ~/.ssh/config and lets their keys in here.
func syncSSH(ctx context.Context) (string, error) {
	ts, err := tailnet.CLI{Bin: tailscaleBin()}.Status(ctx)
	if err != nil {
		return "", fmt.Errorf("Tailscale: %w", err)
	}
	if _, err := sshd.PublicKey(ctx); err != nil {
		return "", err
	}
	var hosts []sshd.Host
	var keys, unreachable []string
	for _, p := range ts.Peers {
		if !p.Online || !ts.SameOwner(p) {
			continue
		}
		var st webui.SSHState
		if err := bridgeCall(ctx, p.Name, http.MethodGet, "ssh", nil, &st); err != nil {
			unreachable = append(unreachable, p.Name)
			continue
		}
		hosts = append(hosts, sshd.Host{Name: p.Name, Address: strings.TrimSuffix(p.DNSName, "."), User: st.User})
		if st.Key != "" {
			keys = append(keys, st.Key)
		}
	}
	slices.SortFunc(hosts, func(a, b sshd.Host) int { return strings.Compare(a.Name, b.Name) })
	if err := sshd.WriteConfig(hosts); err != nil {
		return "", err
	}
	if len(keys) > 0 {
		if err := sshd.Authorize(keys); err != nil {
			return "", err
		}
	}
	named.Lock()
	named.hosts = named.hosts[:0]
	for _, h := range hosts {
		named.hosts = append(named.hosts, h.Name)
	}
	named.Unlock()
	summary := fmt.Sprintf("`ssh <name>` reaches %d machine(s) from here; %d key(s) let in.", len(hosts), len(keys))
	if len(unreachable) > 0 {
		summary += " Couldn't ask " + strings.Join(unreachable, ", ") + ": it needs this spillup version or newer."
	}
	return summary, nil
}

func keepSSH(ctx context.Context, logger *log.Logger) {
	for sleepCtx(ctx, time.Minute); ctx.Err() == nil; sleepCtx(ctx, sshSyncEvery) {
		if _, err := syncSSH(ctx); err != nil {
			logger.Printf("ssh: %v", err)
		}
	}
}

type sshCmd struct {
	Status sshStatusCmd `cmd:"" default:"1" help:"This machine's SSH server, and the machines ssh reaches by name from here."`
	On     sshOnCmd     `cmd:"" help:"Turn on this machine's SSH server: Tailscale SSH, or OpenSSH on Windows, from the tailnet only."`
	Off    sshOffCmd    `cmd:"" help:"Turn off this machine's SSH server."`
	Sync   sshSyncCmd   `cmd:"" help:"Name your machines in ~/.ssh/config and let their keys in, now."`
}

func (sshCmd) Help() string {
	return "Makes `ssh <machine>` work between all your machines over Tailscale, wherever they are."
}

type sshStatusCmd struct{}

func (sshStatusCmd) Run() error {
	st := cliBackend{}.SSHState(context.Background())
	state := "off"
	if st.On {
		state = "on"
	}
	fmt.Printf("SSH server (%s): %s; sign in as %s@%s\n", st.Server, state, st.User, st.Address)
	if st.Problem != "" {
		fmt.Println("problem:", st.Problem)
	}
	return nil
}

type sshOnCmd struct{}

func (sshOnCmd) Run() error { return turnSSH(true) }

type sshOffCmd struct{}

func (sshOffCmd) Run() error { return turnSSH(false) }

func turnSSH(on bool) error {
	ctx := context.Background()
	if on {
		step("Turning on SSH from your tailnet")
	} else {
		step("Turning off SSH")
	}
	if err := sshd.Enable(ctx, tailscaleBin(), on); err != nil {
		return err
	}
	if !on {
		return nil
	}
	summary, err := syncSSH(ctx)
	if err == nil {
		step(summary)
	}
	return err
}

type sshSyncCmd struct{}

func (sshSyncCmd) Run() error {
	summary, err := syncSSH(context.Background())
	if err != nil {
		return err
	}
	fmt.Println(summary)
	return nil
}
