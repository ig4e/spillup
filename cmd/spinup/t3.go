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

// t3SyncEvery is how often the machine holding the accounts checks that every T3 Code is linked to every other.
const t3SyncEvery = 5 * time.Minute

func (cliBackend) T3State(ctx context.Context) webui.T3State {
	t3, err := t3code.Find()
	if err != nil {
		return webui.T3State{App: t3code.AppPresent(t3.Home)}
	}
	linked, err := t3.Linked(ctx)
	st := webui.T3State{Installed: true, App: true, Port: t3.Port(), Linked: linked, LinkingOn: t3.LinkingOn(), NetworkAccess: t3.NetworkAccess()}
	if err != nil {
		st.Problem = err.Error()
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
	name, ip, dns string
	self          bool
	state         webui.T3State
}

// url is where other machines link to this T3: its Tailscale name, which T3 accepts over plain http,
// unlike a bare address.
func (m t3Machine) url() string {
	host := m.dns
	if host == "" {
		host = m.ip
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(m.state.Port))
}

// syncT3Links links every machine's T3 Code to every other's, so agents on any of them can hand work to
// any other. It runs on a machine that keeps the dashboard password, which every holder's dashboard asks
// for; links that exist are left alone.
func syncT3Links(ctx context.Context) (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", errNotInstalled
	}
	ts, err := tailnet.CLI{Bin: cfg.Tailscale}.Status(ctx)
	if err != nil {
		return "", fmt.Errorf("Tailscale: %w", err)
	}
	b := cliBackend{}
	machines := []t3Machine{{name: ts.Self.Name, ip: ts.Self.IP, dns: ts.Self.DNSName, self: true, state: b.T3State(ctx)}}
	for _, p := range ts.Peers {
		if !p.Online {
			continue
		}
		var st webui.T3State
		if err := bridgeCall(ctx, p.Name, http.MethodGet, "t3", nil, &st); err == nil {
			machines = append(machines, t3Machine{name: p.Name, ip: p.IP, dns: p.DNSName, state: st})
		}
	}
	var withT3 []t3Machine
	for _, m := range machines {
		if m.state.Installed && m.ip != "" {
			withT3 = append(withT3, m)
		}
	}
	linked, failed := 0, []string{}
	for _, m := range withT3 {
		if !m.state.NetworkAccess {
			failed = append(failed, fmt.Sprintf("%s's T3 Code answers only itself: open T3 Code there, Settings → Sharing & access → Network access (or set T3 Code up again there), then restart it", m.name))
		}
		if m.state.LinkingOn {
			continue
		}
		var err error
		if m.self {
			err = cliBackend{}.T3EnableLinking()
		} else {
			err = bridgeCall(ctx, m.name, http.MethodPost, "t3/linking", struct{}{}, nil)
		}
		if err != nil {
			failed = append(failed, fmt.Sprintf("turn on linking on %s: %v", m.name, err))
		}
	}
	if len(failed) == 0 {
		time.Sleep(2 * time.Second)
	}
	for _, from := range withT3 {
		for _, to := range withT3 {
			if from.name == to.name || t3code.LinksTo(from.state.Linked, to.ip, to.name, to.dns) {
				continue
			}
			err := linkT3(ctx, from, to)
			if errors.Is(err, t3code.ErrAlreadyLinked) || err != nil && strings.Contains(err.Error(), t3code.ErrAlreadyLinked.Error()) {
				continue
			}
			if err != nil {
				failed = append(failed, fmt.Sprintf("%s → %s: %v", from.name, to.name, err))
				continue
			}
			linked++
		}
	}
	summary := fmt.Sprintf("T3 Code on %d machine(s); %d new link(s).", len(withT3), linked)
	if len(failed) > 0 {
		return summary, errors.New(summary + " Not linked: " + strings.Join(failed, "; "))
	}
	return summary, nil
}

// linkT3 has from's T3 link to to's with a pairing code minted on to.
func linkT3(ctx context.Context, from, to t3Machine) error {
	label := "spillup:" + from.name
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
		if err := bridgeCall(ctx, to.name, http.MethodPost, "t3/pair", map[string]string{"label": label}, &answer); err != nil {
			return err
		}
		code = answer.Code
	}
	if from.self {
		return cliBackend{}.T3Link(ctx, to.url(), code)
	}
	return bridgeCall(ctx, from.name, http.MethodPost, "t3/link", map[string]string{"url": to.url(), "code": code}, nil)
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
	resp, err := (&http.Client{Transport: &http.Transport{Proxy: nil}}).Do(req)
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

// keepT3Linked links new machines' T3 Code as they come, from the machine holding the accounts only, so
// two machines never mint codes for the same link.
func keepT3Linked(ctx context.Context, cfg config.Config, logger *log.Logger) {
	if !cfg.Hold.CanHold() {
		return
	}
	for sleepCtx(ctx, t3SyncEvery); ctx.Err() == nil; sleepCtx(ctx, t3SyncEvery) {
		rep, err := localService(cfg, "").report()
		if err != nil || !rep.Leading {
			continue
		}
		if summary, err := syncT3Links(ctx); err != nil {
			logger.Printf("T3 Code: %v", err)
		} else if !strings.HasSuffix(summary, "0 new link(s).") {
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
	step("Starting T3 Code's background service at boot, on your LAN and tailnet (a pairing code guards it)")
	return t3.Up(ctx)
}
