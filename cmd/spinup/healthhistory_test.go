package main

import (
	"slices"
	"testing"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/history"
	"github.com/darkyeg/spinup/internal/leadership"
	"github.com/darkyeg/spinup/internal/tailnet"
)

func TestHealthDistinguishesAServiceOutageAndAnOfflinePeer(t *testing.T) {
	ts := tailnet.Status{Running: true, Self: tailnet.Node{Name: "desk", Online: true, UserID: 1}, Peers: []tailnet.Node{
		{Name: "standby", Online: true, UserID: 1}, {Name: "offline", UserID: 1}, {Name: "shared", Online: true, UserID: 2},
	}}
	rep := api.Report{Name: "desk", Hold: config.HoldHub, Leading: true, Peers: []leadership.Peer{
		{Name: "standby", State: leadership.Silent}, {Name: "offline", State: leadership.Offline},
	}}
	sigs := machineHealth("desk", ts, true, rep, true)
	for _, wanted := range []history.Signal{
		{Machine: "standby", Check: "Tailscale", State: "Online"}, {Machine: "standby", Check: "Spillup", State: "No response"},
		{Machine: "offline", Check: "Tailscale", State: "Offline"}, {Machine: "desk", Check: "Account sync", State: "Holds the accounts"},
	} {
		if !slices.Contains(sigs, wanted) {
			t.Fatalf("missing %v from %+v", wanted, sigs)
		}
	}
	for _, s := range sigs {
		if s.Machine == "shared" {
			t.Fatal("shared device recorded")
		}
	}
	ts.Running = false
	for _, s := range machineHealth("desk", ts, true, rep, true) {
		if s.Machine != "desk" {
			t.Fatal("remote observation recorded while disconnected")
		}
	}
}
