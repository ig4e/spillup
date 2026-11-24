package main

import (
	"context"
	"fmt"
)

type remoteCmd struct {
	Status remoteStatusCmd `cmd:"" default:"1" help:"Sunshine, Moonlight and moonlight-web on this machine, and the machines it can reach."`
	Setup  remoteSetupCmd  `cmd:"" help:"Install Sunshine, Moonlight and moonlight-web, and start Sunshine at boot with spillup's login."`
	Pair   remotePairCmd   `cmd:"" help:"Pair this machine's moonlight-web with every other machine's Sunshine now."`
}

func (remoteCmd) Help() string {
	return `Remote desktop between your machines: Sunshine on each one to be reached, and Moonlight or the
dashboard's Remote page to reach the others. spillup pairs them over Tailscale, so nobody types a PIN.`
}

type remoteStatusCmd struct{}

func (remoteStatusCmd) Run() error {
	st := cliBackend{}.RemoteState(context.Background())
	yes := func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	}
	fmt.Printf("Sunshine installed: %s, running: %s\nMoonlight installed: %s\nmoonlight-web installed: %s, running: %s\n",
		yes(st.SunshineInstalled), yes(st.SunshineRunning), yes(st.MoonlightInstalled), yes(st.WebInstalled), yes(st.WebRunning))
	for _, h := range st.Hosts {
		fmt.Printf("  %s (%s) paired: %s\n", h.Machine, h.Name, yes(h.Paired))
	}
	return nil
}

type remoteSetupCmd struct{}

func (remoteSetupCmd) Run() error { return setUpRemote(context.Background()) }

type remotePairCmd struct{}

func (remotePairCmd) Run() error {
	summary, err := syncRemote(context.Background())
	if err != nil {
		return err
	}
	fmt.Println(summary)
	return nil
}
