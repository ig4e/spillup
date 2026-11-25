package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/tailnet"
)

// machineKeys returns the API key for the launcher: the hub makes the keys, a standby copies them from
// the leader, and a machine that only uses the accounts keeps the API key it was given. Keys given
// explicitly replace the stored ones.
func machineKeys(cfg config.Config, ts tailnet.Status, req serviceRequest) (string, error) {
	stored, loadErr := config.LoadSecrets(cfg)
	have := loadErr == nil
	if have && !req.replacesKeys(cfg.Hold) {
		step("Using the keys in %s", cfg.SecretsPath())
		return stored.APIKey, nil
	}
	s, err := obtainSecrets(cfg, ts, req)
	if err != nil {
		return "", err
	}
	if err := config.SaveSecrets(cfg, s); err != nil {
		return "", err
	}
	if have {
		step("Updated the keys in %s", cfg.SecretsPath())
	}
	return s.APIKey, nil
}

func (r serviceRequest) replacesKeys(h config.Hold) bool {
	switch h {
	case config.HoldNever:
		return r.apiKey != ""
	case config.HoldStandby:
		return r.password != ""
	}
	return false
}

func obtainSecrets(cfg config.Config, ts tailnet.Status, req serviceRequest) (config.Secrets, error) {
	if cfg.Hold == config.HoldHub {
		return newSecrets(cfg, ts, req.password)
	}
	if req.password == "" && req.apiKey == "" {
		if s, from, err := joinFleet(ts, cfg.Port, cfg.Hold); err == nil {
			step("Joined: %s gave this machine its keys (it's on your own Tailscale account)", from)
			return s, nil
		}
	}
	if cfg.Hold.CanHold() {
		return newSecrets(cfg, ts, req.password)
	}
	key, err := orAsk(req.apiKey, "API key (on the hub: spinup keys): ", "API key")
	return config.Secrets{APIKey: key}, err
}

// joinFleet asks the holders on the tailnet for this machine's keys. A holder answers only a device on
// its own Tailscale account, so this works without typing anything on your own machines.
func joinFleet(ts tailnet.Status, port int, hold config.Hold) (config.Secrets, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := api.Client{HTTP: directMachineClient}
	for _, p := range ts.Peers {
		if !p.Online || !ts.SameOwner(p) {
			continue
		}
		var s config.Secrets
		base := "http://" + net.JoinHostPort(p.IP, strconv.Itoa(port))
		if err := client.Post(ctx, base+api.PathJoin, api.Join{Hold: hold}, &s); err != nil || s.APIKey == "" {
			continue
		}
		if hold.CanHold() && s.ManagementPassword == "" {
			continue
		}
		return s, p.Name, nil
	}
	return config.Secrets{}, "", errors.New("no holder on your tailnet let this machine join")
}

func newSecrets(cfg config.Config, ts tailnet.Status, password string) (config.Secrets, error) {
	if cfg.Hold == config.HoldHub {
		s := config.NewSecrets()
		if legacy, ok := findLegacyProxy(cfg); ok && len(legacy.APIKeys) > 0 {
			s.APIKey = legacy.APIKeys[0]
			step("Keeping the API key your existing CLIProxyAPI uses (%s), so your tools and T3 Code keep working", legacy.Config)
		} else {
			step("Making new keys in %s", cfg.SecretsPath())
		}
		return s, nil
	}
	password, err := orAsk(password, "Dashboard password (on the hub: spinup keys): ", "dashboard password")
	if err != nil {
		return config.Secrets{}, err
	}
	s, err := fetchSecrets(ts, cfg.Port, password)
	if err == nil {
		step("Copied the keys from the machine that holds the accounts")
	}
	return s, err
}

// holderCandidate is an online device that may be the machine holding the accounts.
type holderCandidate struct{ name, base string }

func fetchSecrets(ts tailnet.Status, port int, password string) (config.Secrets, error) {
	var candidates []holderCandidate
	for _, p := range ts.Peers {
		if p.Online {
			candidates = append(candidates, holderCandidate{p.Name, "http://" + net.JoinHostPort(p.IP, strconv.Itoa(port))})
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return secretsFrom(ctx, candidates, password)
}

// secretsFrom sends the password only to a device that proves it knows it.
func secretsFrom(ctx context.Context, candidates []holderCandidate, password string) (config.Secrets, error) {
	public := api.Client{HTTP: directMachineClient}
	keyed := api.Client{HTTP: directMachineClient, Key: password}
	claimedWithoutProof := ""
	for _, c := range candidates {
		if ctx.Err() != nil {
			return config.Secrets{}, ctx.Err()
		}
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		l, proof, err := public.AskLeader(probeCtx, c.base, c.name, config.Secrets{ManagementPassword: password})
		cancel()
		switch {
		case err != nil || !l.Leading || l.Name != c.name:
			continue
		case proof != api.KnowsPassword:
			claimedWithoutProof = c.name
			continue
		}
		var s config.Secrets
		if err := keyed.Get(ctx, c.base+api.PathSecrets, &s); err != nil {
			return s, fmt.Errorf("%s refused: %w", c.name, err)
		}
		return s, nil
	}
	if claimedWithoutProof != "" {
		return config.Secrets{}, fmt.Errorf("%s holds the accounts but doesn't know that password: wrong password?", claimedWithoutProof)
	}
	return config.Secrets{}, errors.New("no machine on your tailnet holds the accounts with spinup: install the hub first")
}

func orAsk(given, prompt, what string) (string, error) {
	if given == "" {
		given = askSecret(prompt)
	}
	if given == "" {
		return "", fmt.Errorf("no %s given", what)
	}
	return given, nil
}

var directMachineClient = &http.Client{
	Transport: &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 4 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		IdleConnTimeout:     60 * time.Second,
		MaxIdleConnsPerHost: 4,
	},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}
