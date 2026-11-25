package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/tailnet"
	"github.com/darkyeg/spinup/internal/webui"
)

func (cliBackend) T3DesktopLink(ctx context.Context, machine string) (string, error) {
	if !tailnet.ValidName(machine) {
		return "", errors.New("choose a Tailnet machine name")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	secrets, err := config.LoadSecrets(cfg)
	if err != nil {
		return "", err
	}
	st, err := (tailnet.CLI{Bin: cfg.Tailscale}).Status(ctx)
	if err != nil {
		return "", err
	}
	base, err := verifiedFleetPeer(ctx, st, machine, cfg.Port, secrets.APIKey)
	if err != nil {
		return "", err
	}
	node, _ := st.Peer(machine)
	return desktopPairing(ctx, st, node, base, secrets.APIKey, directMachineClient)
}

func desktopPairing(ctx context.Context, st tailnet.Status, node tailnet.Node, base, apiKey string, httpClient *http.Client) (string, error) {
	bridge := base + webui.BridgePrefix + "/api/t3"
	client := api.Client{HTTP: httpClient, APIKey: apiKey, MaxResponseBytes: 1 << 20}
	var info webui.T3State
	if err := client.Get(ctx, bridge, &info); err != nil || !info.Installed || info.EnvironmentID == "" || info.Port <= 0 || info.Port > 65535 {
		return "", errors.New("the machine didn't report a T3 environment; run spinup t3 setup there")
	}
	address := t3TailnetURL(st, node, info.Port)
	if address == "" {
		return "", errors.New("T3 desktop pairing needs MagicDNS or a Tailscale IPv6 address")
	}
	var descriptor struct {
		EnvironmentID string `json:"environmentId"`
	}
	probe := api.Client{HTTP: httpClient, MaxResponseBytes: 64 << 10}
	if err := probe.Get(ctx, address+"/.well-known/t3/environment", &descriptor); err != nil || descriptor.EnvironmentID != info.EnvironmentID {
		return "", errors.New("the expected T3 environment didn't answer at its Tailnet address; check Network access on that machine")
	}
	var pair struct {
		Code string `json:"code"`
	}
	request := struct {
		Label string `json:"label"`
	}{Label: "Spillup desktop on " + st.Self.Name}
	if err := client.Post(ctx, bridge+"/pair", request, &pair); err != nil || len(pair.Code) == 0 || len(pair.Code) > 2048 {
		return "", errors.New("T3 couldn't issue a desktop pairing code")
	}
	return desktopAppLink(address, pair.Code), nil
}

func desktopAppLink(address, code string) string {
	pairingURL := address + "/pair#token=" + url.QueryEscape(code)
	return "t3code://pair?pairingUrl=" + url.QueryEscape(pairingURL)
}

type t3ConnectCmd struct {
	Machine string `arg:"" help:"The source machine's Tailnet name."`
}

func (c t3ConnectCmd) Run() error {
	link, err := (cliBackend{}).T3DesktopLink(context.Background(), c.Machine)
	if err != nil {
		return err
	}
	if !openT3Desktop(link) {
		return errors.New("couldn't open the T3 Code desktop app's registered protocol handler")
	}
	fmt.Printf("Confirm the Tailnet address for %s in T3 Code. Then use Add Project or choose the machine in a new projectless thread.\n", c.Machine)
	return nil
}
