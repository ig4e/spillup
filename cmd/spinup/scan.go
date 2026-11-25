package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/tailnet"
	"github.com/darkyeg/spinup/internal/webui"
)

// ScanDevices asks every online device on the tailnet whether it runs spillup: a holder answers who it
// is, and a machine that only uses the accounts answers its dashboard bridge, without a key, with a refusal.
func (cliBackend) ScanDevices(ctx context.Context) ([]webui.Device, error) {
	cfg, _ := config.Load()
	ts, err := tailnet.CLI{Bin: cfg.Tailscale}.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("Tailscale: %w", err)
	}
	port := cfg.Port
	if port == 0 {
		port = config.Defaults().Port
	}
	devices := make([]webui.Device, len(ts.Peers))
	var wg sync.WaitGroup
	for i, p := range ts.Peers {
		devices[i] = webui.Device{Name: p.Name, IP: p.IP, OS: p.OS, Online: p.Online, Yours: ts.SameOwner(p)}
		if !p.Online {
			continue
		}
		wg.Go(func() { probeDevice(ctx, &devices[i], port) })
	}
	wg.Wait()
	return devices, nil
}

func probeDevice(ctx context.Context, d *webui.Device, port int) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	client := directMachineClient
	base := "http://" + net.JoinHostPort(d.IP, strconv.Itoa(port))
	if req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+api.PathLeader, nil); err == nil {
		if resp, err := client.Do(req); err == nil {
			var l api.Leader
			ok := resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&l) == nil && l.Name != ""
			resp.Body.Close()
			if ok {
				d.Spillup, d.Hold = true, l.Hold
				return
			}
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+webui.BridgePrefix+"/api/overview", nil)
	if err != nil {
		return
	}
	if resp, err := client.Do(req); err == nil {
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusOK {
			d.Spillup, d.Hold = true, config.HoldNever
		}
	}
}

func (cliBackend) SetHubOnly(on bool) error {
	return withKeyedService(func(l local) error {
		return l.call(time.Minute, http.MethodPost, api.PathMode, api.Mode{HubOnly: on}, nil)
	})
}
