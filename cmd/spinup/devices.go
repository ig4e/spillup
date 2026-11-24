package main

import (
	"context"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/webui"
)

// deviceCheckEvery is how often the service checks the Tailscale address and the setting it serves your
// devices' dashboard by.
const deviceCheckEvery = 30 * time.Second

// hostDeviceDashboard serves the dashboard on this machine's Tailscale address, at the dashboard's port, to
// your own devices, while config.json allows it, following the address when Tailscale changes it.
func hostDeviceDashboard(ctx context.Context, port int, logger *log.Logger) {
	if port == 0 {
		return
	}
	h := webui.New(cliBackend{}, "").DeviceHandler()
	for ctx.Err() == nil {
		ip := deviceAddress(ctx)
		if ip == "" {
			sleepCtx(ctx, deviceCheckEvery)
			continue
		}
		addr := net.JoinHostPort(ip, strconv.Itoa(port))
		l, err := net.Listen("tcp", addr)
		if err != nil {
			logger.Printf("dashboard for your devices: can't listen on %s: %v (retrying)", addr, err)
			sleepCtx(ctx, deviceCheckEvery)
			continue
		}
		logger.Printf("dashboard for your devices on http://%s", addr)
		serving, stop := context.WithCancel(ctx)
		go func() {
			for sleepCtx(serving, deviceCheckEvery); serving.Err() == nil; sleepCtx(serving, deviceCheckEvery) {
				if deviceAddress(serving) != ip {
					stop()
				}
			}
		}()
		if err := serveDashboard(serving, l, h); err != nil {
			logger.Printf("dashboard for your devices: %v", err)
		}
		stop()
	}
}

// deviceAddress is the Tailscale address to serve your devices on, or "" while that's off or Tailscale is down.
func deviceAddress(ctx context.Context) string {
	cfg, err := config.Load()
	if err != nil || !cfg.DeviceAccess {
		return ""
	}
	return selfTailnetIP(ctx)
}
