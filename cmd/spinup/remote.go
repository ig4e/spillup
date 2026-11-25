package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/darkyeg/spinup/internal/atomicfile"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/remoteweb"
	"github.com/darkyeg/spinup/internal/sunshine"
	"github.com/darkyeg/spinup/internal/tailnet"
	"github.com/darkyeg/spinup/internal/webui"
)

func remoteInstall() remoteweb.Install {
	return remoteweb.Install{Dir: filepath.Join(config.StateDir(), "moonlight-web")}
}

func remotePort() int {
	cfg, _ := config.Load()
	if cfg.RemotePort != 0 {
		return cfg.RemotePort
	}
	return config.Defaults().RemotePort
}

func sunshineLoginPath() string { return filepath.Join(config.StateDir(), "sunshine.json") }

// sunshineLogin is the login spillup set on this machine's Sunshine, made on first use.
func sunshineLogin() (login sunshine.Login, fresh bool, err error) {
	if data, err := os.ReadFile(sunshineLoginPath()); err == nil && json.Unmarshal(data, &login) == nil && login.Password != "" {
		return login, false, nil
	}
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return login, false, err
	}
	login = sunshine.Login{User: "spillup", Password: hex.EncodeToString(raw)}
	data, _ := json.Marshal(login)
	if err := os.MkdirAll(config.StateDir(), 0o700); err != nil {
		return login, false, err
	}
	return login, true, atomicfile.Write(sunshineLoginPath(), data, 0o600)
}

// setUpRemote makes this machine reachable and able to reach the others: Sunshine as the host, started at
// boot with spillup's login; Moonlight as the app; and moonlight-web, the browser client the dashboard shows.
func setUpRemote(ctx context.Context) error {
	if sunshine.Exe() == "" || sunshine.MoonlightExe() == "" {
		step("Installing Sunshine and Moonlight")
		if err := installRemotePackages(ctx); err != nil {
			return err
		}
	}
	login, _, err := sunshineLogin()
	if err != nil {
		return err
	}
	if exe := sunshine.Exe(); exe != "" {
		step("Giving Sunshine spillup's login and starting it at boot")
		if err := startSunshine(ctx, exe, login); err != nil {
			return err
		}
	} else {
		step("Sunshine isn't available for this system; this machine can reach the others but not be reached")
	}
	in := remoteInstall()
	step("Installing moonlight-web, the browser client")
	version, err := in.Download(ctx)
	if err != nil {
		return err
	}
	if err := in.WriteConfig(remotePort()); err != nil {
		return err
	}
	step("moonlight-web %s is ready; the accounts service runs it", version)
	return nil
}

// remoteHosts remembers which moonlight-web host each machine is, by machine name.
type remoteHosts map[string]uint32

func remoteHostsPath() string { return filepath.Join(config.StateDir(), "remote-hosts.json") }

func loadRemoteHosts() remoteHosts {
	h := remoteHosts{}
	if data, err := os.ReadFile(remoteHostsPath()); err == nil {
		_ = json.Unmarshal(data, &h)
	}
	return h
}

func (h remoteHosts) save() error {
	data, _ := json.Marshal(h)
	return atomicfile.Write(remoteHostsPath(), data, 0o600)
}

var remoteSyncing sync.Mutex

// syncRemote adds every other machine that runs Sunshine to this machine's moonlight-web and pairs it: the
// host's spillup confirms the PIN with its own Sunshine, so nobody types one.
func syncRemote(ctx context.Context) (string, error) {
	if !remoteSyncing.TryLock() {
		return "", errors.New("remote pairing is already running")
	}
	defer remoteSyncing.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cfg, err := config.Load()
	if err != nil {
		return "", errNotInstalled
	}
	ts, err := tailnet.CLI{Bin: cfg.Tailscale}.Status(ctx)
	if err != nil {
		return "", fmt.Errorf("Tailscale: %w", err)
	}
	client := remoteweb.Client{Port: remotePort()}
	known, err := client.Hosts(ctx)
	if err != nil {
		return "", err
	}
	paired := map[uint32]bool{}
	for _, h := range known {
		paired[h.ID] = h.Paired == "Paired"
	}
	hosts := loadRemoteHosts()
	added, failed := 0, []string{}
	for _, p := range ts.Peers {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if !p.Online || p.Expired || !ts.SameOwner(p) {
			continue
		}
		var st webui.RemoteState
		if err := bridgeCall(ctx, p.Name, http.MethodGet, "remote", nil, &st); err != nil || !st.SunshineRunning {
			continue
		}
		id, ok := hosts[p.Name]
		if _, still := paired[id]; !ok || !still {
			address := p.DNSName
			if address == "" {
				address = p.IP
			}
			h, err := client.AddHost(ctx, address)
			if err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", p.Name, err))
				continue
			}
			id, hosts[p.Name] = h.ID, h.ID
			_ = hosts.save()
		}
		if paired[id] {
			continue
		}
		if err := pairRemote(ctx, client, id, p.Name, ts.Self.Name); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", p.Name, err))
			continue
		}
		added++
	}
	summary := fmt.Sprintf("%d machine(s) newly paired for remote desktop.", added)
	if len(failed) > 0 {
		return summary, errors.New(summary + " Not paired: " + strings.Join(failed, "; "))
	}
	return summary, nil
}

func pairRemote(ctx context.Context, client remoteweb.Client, id uint32, machine, self string) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	pin, wait, err := client.Pair(ctx, id)
	if err != nil {
		return err
	}
	if err := bridgeCall(ctx, machine, http.MethodPost, "sunshine/pin", map[string]string{"pin": pin, "name": "spillup on " + self}, nil); err != nil {
		return err
	}
	return wait()
}

// keepRemoteWeb runs moonlight-web for the dashboard whenever it is installed, and pairs new machines.
func keepRemoteWeb(ctx context.Context, logger *log.Logger) {
	in := remoteInstall()
	for ctx.Err() == nil && !in.Installed() {
		sleepCtx(ctx, time.Minute)
	}
	if ctx.Err() != nil {
		return
	}
	if err := in.WriteConfig(remotePort()); err != nil {
		logger.Printf("moonlight-web config: %v", err)
	}
	go func() {
		for sleepCtx(ctx, 30*time.Second); ctx.Err() == nil; sleepCtx(ctx, 10*time.Minute) {
			if summary, err := syncRemote(ctx); err != nil {
				logger.Printf("remote desktop: %v", err)
			} else if !strings.HasPrefix(summary, "0 ") {
				logger.Printf("remote desktop: %s", summary)
			}
		}
	}()
	in.Keep(ctx, logger)
}

func (cliBackend) RemoteState(ctx context.Context) webui.RemoteState {
	st := webui.RemoteState{
		SunshineInstalled: sunshine.Exe() != "", SunshineRunning: sunshine.Running(),
		MoonlightInstalled: sunshine.MoonlightExe() != "", WebInstalled: remoteInstall().Installed(), Hosts: []webui.RemoteHost{},
	}
	if !st.WebInstalled {
		return st
	}
	known, err := remoteweb.Client{Port: remotePort()}.Hosts(ctx)
	if err != nil {
		st.Problem = err.Error()
		return st
	}
	st.WebRunning = true
	byID := map[uint32]string{}
	for name, id := range loadRemoteHosts() {
		byID[id] = name
	}
	for _, h := range known {
		st.Hosts = append(st.Hosts, webui.RemoteHost{ID: h.ID, Name: h.Name, Machine: byID[h.ID], Paired: h.Paired == "Paired", State: h.State})
	}
	return st
}

func (cliBackend) SunshinePIN(ctx context.Context, pin, name, fromIP string) error {
	login, fresh, err := sunshineLogin()
	if err != nil {
		return err
	}
	if fresh {
		return errors.New("this machine's Sunshine has no spillup login yet: set remote desktop up here first")
	}
	return sunshine.ConfirmPIN(ctx, login, pin, name, fromIP)
}

func (cliBackend) RemoteSync(ctx context.Context) (string, error) { return syncRemote(ctx) }

func (cliBackend) RemoteSetup(actionID string) error {
	return startTask([]string{"remote", "setup"}, actionID)
}

func (cliBackend) RemoteWebPort() int { return remotePort() }
