package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/t3code"
	"github.com/darkyeg/spinup/internal/tailnet"
	"github.com/darkyeg/spinup/internal/webui"
)

// t3SyncEvery is how often each machine checks for T3 Code peers.
const t3SyncEvery = 5 * time.Minute

func (cliBackend) T3State(ctx context.Context) webui.T3State {
	t3, err := t3code.Find()
	if err != nil {
		return webui.T3State{App: t3code.AppPresent(t3.Home)}
	}
	linked, err := t3.Linked(ctx)
	st := webui.T3State{Installed: true, App: true, Port: t3.Port(), EnvironmentID: t3.EnvironmentID(), Linked: linked, LinkingOn: t3.LinkingOn(), NetworkAccess: t3.NetworkAccess(), AwaitsRestart: t3.AwaitsRestart(selfTailnetIP(ctx))}
	if err != nil {
		st.Problem = err.Error()
	}
	if ts, err := (tailnet.CLI{Bin: tailscaleBin()}).Status(ctx); err == nil && ts.Running {
		st.Address = t3TailnetURL(ts, ts.Self, st.Port)
	}
	return st
}

func (cliBackend) T3Pair(ctx context.Context, label string) (string, error) {
	t3, err := t3code.Find()
	if err != nil {
		return "", err
	}
	p, err := t3.Pair(ctx, label)
	return p.Credential, err
}

func (cliBackend) T3Link(ctx context.Context, url, code string) error {
	t3, err := t3code.Find()
	if err != nil {
		return err
	}
	return t3.Link(ctx, url, code)
}

func (cliBackend) SyncT3(ctx context.Context) (string, error) { return syncT3Links(ctx) }

func (cliBackend) T3EnableLinking() error {
	t3, err := t3code.Find()
	if err != nil {
		return err
	}
	_, err = t3.EnableLinking()
	return err
}

func (cliBackend) T3Setup(ctx context.Context) error { return setUpT3(ctx) }

// t3Machine is one machine's T3 Code, here or reached through its dashboard over the tailnet.
type t3Machine struct {
	tailnet.Node
	address string
	self    bool
	state   webui.T3State
}

// T3 accepts plain HTTP for MagicDNS and Tailscale IPv6, but rejects CGNAT IPv4 addresses.
func t3TailnetURL(st tailnet.Status, node tailnet.Node, port int) string {
	if port == 0 {
		return ""
	}
	if st.MagicDNS && strings.HasSuffix(strings.TrimSuffix(node.DNSName, "."), ".ts.net") {
		return st.HTTPURL(node, port)
	}
	if node.IPv6 != "" {
		return "http://" + net.JoinHostPort(node.IPv6, strconv.Itoa(port))
	}
	return ""
}

// syncT3Links makes an immediate pass in both directions after setup or on request.
func syncT3Links(ctx context.Context) (string, error) {
	return reconcileT3Links(ctx, false)
}

// reconcileT3Links pairs discovered T3 environments. Each daemon owns its outgoing links.
func reconcileT3Links(ctx context.Context, outgoingOnly bool) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 70*time.Second)
	defer cancel()
	cfg, err := config.Load()
	if err != nil {
		return "", errNotInstalled
	}
	ts, err := tailnet.CLI{Bin: cfg.Tailscale}.Status(ctx)
	if err != nil {
		return "", fmt.Errorf("Tailscale: %w", err)
	}
	if !ts.Running || ts.Self.IP == "" {
		return "", errors.New("Tailscale has no local Tailnet address")
	}
	local := cliBackend{}.T3State(ctx)
	if outgoingOnly && (!local.Installed || !local.LinkingOn) {
		return "", nil
	}
	machines := []t3Machine{{Node: ts.Self, address: t3TailnetURL(ts, ts.Self, local.Port), self: true, state: local}}
	for _, p := range ts.Peers {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if !p.Online || p.Expired || !ts.SameOwner(p) {
			continue
		}
		var st webui.T3State
		probe, done := context.WithTimeout(ctx, 12*time.Second)
		err := bridgeCall(probe, p.Name, http.MethodGet, "t3", nil, &st)
		done()
		if err == nil && st.Installed {
			machines = append(machines, t3Machine{Node: p, address: t3TailnetURL(ts, p, st.Port), state: st})
		}
	}
	linked, failed := 0, []string{}
	var ready []t3Machine
	for _, m := range machines {
		switch {
		case !m.state.Installed:
			continue
		case !m.state.LinkingOn:
			failed = append(failed, m.Name+": Linked environments is off; run spinup t3 setup there")
		case !m.state.NetworkAccess || m.state.AwaitsRestart:
			failed = append(failed, m.Name+": enable T3 Code Network access and reopen T3 Code")
		case m.state.Problem != "":
			failed = append(failed, fmt.Sprintf("%s: %s", m.Name, m.state.Problem))
		case m.address == "":
			failed = append(failed, m.Name+": T3 linking needs MagicDNS or a Tailscale IPv6 address")
		default:
			ready = append(ready, m)
		}
	}
	for _, from := range ready {
		if outgoingOnly && !from.self {
			continue
		}
		for _, to := range ready {
			if err := ctx.Err(); err != nil {
				return "", fmt.Errorf("T3 linking after %d new links: %w", linked, err)
			}
			if from.IP == to.IP || t3code.LinksTo(from.state.Linked, to.state.EnvironmentID, to.IP, to.IPv6, to.Name, to.DNSName) {
				continue
			}
			if err := linkT3(ctx, from, to); err != nil {
				failed = append(failed, fmt.Sprintf("%s → %s: %v", from.Name, to.Name, err))
				continue
			}
			linked++
		}
	}
	summary := fmt.Sprintf("T3 Code on %d machine(s); %d new link(s).", len(ready), linked)
	if len(failed) > 0 {
		return summary, errors.New(summary + " Not linked: " + strings.Join(failed, "; "))
	}
	return summary, nil
}

// linkT3 has from's T3 link to to's with a pairing code minted on to.
func linkT3(ctx context.Context, from, to t3Machine) error {
	label := "spillup:" + from.Name
	var code string
	if to.self {
		c, err := cliBackend{}.T3Pair(ctx, label)
		if err != nil {
			return err
		}
		code = c
	} else {
		var answer struct {
			Code string `json:"code"`
		}
		if err := bridgeCall(ctx, to.Name, http.MethodPost, "t3/pair", map[string]string{"label": label}, &answer); err != nil {
			return err
		}
		code = answer.Code
	}
	if code == "" {
		return fmt.Errorf("%s returned no pairing code", to.Name)
	}
	if from.self {
		return cliBackend{}.T3Link(ctx, to.address, code)
	}
	return bridgeCall(ctx, from.Name, http.MethodPost, "t3/link", map[string]string{"url": to.address, "code": code}, nil)
}

// bridgeCall calls another machine's dashboard over the tailnet, with this machine's keys.
func bridgeCall(ctx context.Context, machine, method, path string, in, out any) error {
	rm, err := cliBackend{}.Remote(machine)
	if err != nil {
		return err
	}
	var body io.Reader
	if in != nil {
		data, _ := json.Marshal(in)
		body = bytes.NewReader(data)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, rm.Base.String()+webui.BridgePrefix+"/api/"+path, body)
	if err != nil {
		return err
	}
	for k, v := range rm.Header {
		req.Header[k] = v
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := directMachineClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		var e api.Error
		_ = json.Unmarshal(data, &e)
		return fmt.Errorf("%s: %s", machine, strings.TrimSpace(e.Error.Message+" "+http.StatusText(resp.StatusCode)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// keepT3Linked picks up new peers and retries after a machine comes back online.
func keepT3Linked(ctx context.Context, logger *log.Logger) {
	for ; ctx.Err() == nil; sleepCtx(ctx, t3SyncEvery) {
		if n, err := fixT3QuotaKeys(ctx); err != nil {
			logger.Printf("T3 Code quota: %v", err)
		} else if n > 0 {
			logger.Printf("T3 Code quota: gave %d source(s) the management key", n)
		}
		if summary, err := reconcileT3Links(ctx, true); err != nil {
			logger.Printf("T3 Code: %v", err)
		} else if summary != "" && !strings.HasSuffix(summary, "0 new link(s).") {
			logger.Printf("T3 Code: %s", summary)
		}
	}
}

// setUpT3 installs T3 Code's command line where it's missing, and runs its background service at boot, reachable on the LAN and tailnet, with linked
// environments on, so the machine holding the accounts can link it to the others.
func setUpT3(ctx context.Context) error {
	t3, err := t3code.Find()
	if errors.Is(err, t3code.ErrNotInstalled) {
		step("Installing T3 Code's command line")
		if err := t3code.InstallCLI(ctx); err != nil {
			return err
		}
		if t3, err = t3code.Find(); err != nil {
			return fmt.Errorf("T3's installer ran, but its command isn't where spillup looks: %w", err)
		}
	}
	if err != nil {
		return err
	}
	if changed, err := t3.EnableLinking(); err != nil {
		return err
	} else if changed {
		step("Turned on T3 Code's linked environments")
	}
	if changed, err := t3.EnableNetworkAccess(); err != nil {
		return err
	} else if changed {
		step("Turned on the T3 Code app's Network access: restart T3 Code once so your other machines can reach it")
	}
	if n, err := fixT3QuotaKeys(ctx); err != nil {
		return err
	} else if n > 0 {
		step("Gave T3 Code's usage limits CLIProxyAPI's management key, which they need to read quotas")
	}
	step("Starting T3 Code's background service at boot, on your LAN and tailnet (a pairing code guards it)")
	if err := t3.Up(ctx); err != nil {
		return err
	}
	if summary, err := syncT3Links(ctx); err != nil {
		step("T3 Code linking will be retried automatically: %v", err)
	} else {
		step("%s", summary)
	}
	return nil
}

// fixT3QuotaKeys gives T3 Code's usage-limit sources that read the local router or the shared hub its management
// key, in place of the API key they're often given, which CLIProxyAPI refuses for quotas.
func fixT3QuotaKeys(ctx context.Context) (int, error) {
	t3, err := t3code.Find()
	if err != nil {
		return 0, nil
	}
	cfg, err := config.Load()
	if err != nil || !cfg.Hold.CanHold() {
		return 0, nil
	}
	s, err := config.LoadSecrets(cfg)
	if err != nil {
		return 0, nil
	}
	return t3.FixQuotaKeys(cfg.Port, s.ManagementPassword, quotaHosts(ctx, cfg)...)
}

// staleT3QuotaSources are those fixT3QuotaKeys would fix.
func staleT3QuotaSources(ctx context.Context, t3 t3code.T3) int {
	cfg, err := config.Load()
	if err != nil || !cfg.Hold.CanHold() {
		return 0
	}
	s, err := config.LoadSecrets(cfg)
	if err != nil {
		return 0
	}
	return len(t3.StaleQuotaSources(cfg.Port, s.ManagementPassword, quotaHosts(ctx, cfg)...))
}

func quotaHosts(ctx context.Context, cfg config.Config) []string {
	st, err := (tailnet.CLI{Bin: cfg.Tailscale}).Status(ctx)
	if err != nil || !st.Running {
		return nil
	}
	hosts := []string{st.Self.IP, st.Self.DNSName}
	if st.MagicDNS {
		hosts = append(hosts, st.Self.Name)
	}
	rep, err := localService(cfg, "").report()
	if err != nil || rep.Leading {
		return hosts
	}
	peer, ok := st.Peer(rep.Leader)
	if ok && st.SameOwner(peer) && !peer.Expired {
		hosts = append(hosts, peer.IP, peer.DNSName)
		if st.MagicDNS {
			hosts = append(hosts, peer.Name)
		}
	}
	return hosts
}

// selfTailnetIP is this machine's Tailscale address, or "" while Tailscale isn't up.
func selfTailnetIP(ctx context.Context) string {
	cfg, _ := config.Load()
	st, err := tailnet.CLI{Bin: cfg.Tailscale}.Status(ctx)
	if err != nil {
		return ""
	}
	return st.Self.IP
}
