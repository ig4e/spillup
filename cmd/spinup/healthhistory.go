package main

import (
	"context"
	"log"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/history"
	"github.com/darkyeg/spinup/internal/leadership"
	"github.com/darkyeg/spinup/internal/tailnet"
)

func keepHealthHistory(ctx context.Context, cfg config.Config, logger *log.Logger) {
	var self string
	for ; ctx.Err() == nil; sleepCtx(ctx, time.Minute) {
		ts, tsErr := (tailnet.CLI{Bin: cfg.Tailscale}).Status(ctx)
		probe, cancel := context.WithTimeout(ctx, 5*time.Second)
		var rep api.Report
		local := localService(cfg, "")
		err := local.client.Get(probe, local.base+api.PathState, &rep)
		cancel()
		if ts.Self.Name != "" {
			self = ts.Self.Name
		} else if self == "" {
			self = rep.Name
		}
		if ctx.Err() != nil {
			return
		}
		signals := machineHealth(self, ts, tsErr == nil, rep, err == nil)
		if err := history.Local().Observe(time.Now().UTC(), signals); err != nil {
			logger.Printf("machine health history: %v", err)
		}
	}
}

func machineHealth(self string, ts tailnet.Status, tsKnown bool, rep api.Report, answered bool) []history.Signal {
	var signals []history.Signal
	add := func(machine, check, state string) {
		signals = append(signals, history.Signal{Machine: machine, Check: check, State: state})
	}
	netState := func(n tailnet.Node) string {
		switch {
		case n.Expired:
			return "Login expired"
		case n.Online:
			return "Online"
		default:
			return "Offline"
		}
	}
	switch {
	case !tsKnown:
		add(self, "Tailscale", "Status unavailable")
	case !ts.Running:
		add(self, "Tailscale", "Disconnected")
	default:
		add(self, "Tailscale", netState(ts.Self))
	}
	if answered {
		add(self, "Spillup", "Responding")
		switch {
		case rep.Leading:
			add(self, "Account sync", "Holds the accounts")
		case rep.Hold == config.HoldNever:
			add(self, "Account sync", "Worker")
		case rep.HubOnly:
			add(self, "Account sync", "Copies disabled")
		case rep.Synced.Fresh(rep.Epoch):
			add(self, "Account sync", "In sync")
		default:
			add(self, "Account sync", "Awaiting fresh copy")
		}
	} else {
		add(self, "Spillup", "No response")
	}
	if !tsKnown || !ts.Running || !ts.Self.Online {
		return signals
	}
	holders := make(map[string]leadership.Peer)
	for _, p := range rep.Peers {
		holders[p.Name] = p
	}
	for _, node := range ts.Peers {
		if !ts.SameOwner(node) {
			continue
		}
		add(node.Name, "Tailscale", netState(node))
		p, ok := holders[node.Name]
		if !answered || !ok {
			if answered && rep.Hold == config.HoldNever && rep.Leader == node.Name && rep.LeaderAddr != "" && node.Online {
				add(node.Name, "Spillup", "Responding")
			}
			continue
		}
		switch {
		case !node.Online || node.Expired || p.State == leadership.Offline:
			add(node.Name, "Spillup", "Offline")
		case p.State == leadership.Silent:
			add(node.Name, "Spillup", "No response")
		default:
			add(node.Name, "Spillup", "Responding")
			switch {
			case p.State == leadership.Leading:
				add(node.Name, "Account sync", "Holds the accounts")
			case p.State == leadership.Starting:
				add(node.Name, "Account sync", "Starting proxy")
			case rep.HubOnly:
				add(node.Name, "Account sync", "Copies disabled")
			case p.State == leadership.Synced:
				add(node.Name, "Account sync", "In sync")
			default:
				add(node.Name, "Account sync", "Awaiting fresh copy")
			}
		}
	}
	return signals
}
