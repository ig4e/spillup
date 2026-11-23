package main

import (
	"context"
	"fmt"
	"strings"
)

type t3Cmd struct {
	Status t3StatusCmd `cmd:"" default:"1" help:"T3 Code on this machine and the machines it's linked to."`
	Setup  t3SetupCmd  `cmd:"" help:"Run T3 Code's background service at boot, on your LAN and tailnet, with linked environments on."`
	Link   t3LinkCmd   `cmd:"" help:"Link every machine's T3 Code to every other's now (the machine holding the accounts does it every few minutes)."`
}

func (t3Cmd) Help() string {
	return `spillup keeps T3 Code on all your machines linked, so an agent on any of them can hand work to any
other ("delegate this to my desktop"). Setup runs T3's background service at boot; the machine holding
the accounts then links every machine's T3 to every other's with full access, and links new machines as
they appear. Change a link's access in T3 → Settings → Sharing & access.`
}

type t3StatusCmd struct{}

func (t3StatusCmd) Run() error {
	st := cliBackend{}.T3State(context.Background())
	if !st.Installed {
		fmt.Println("T3 Code isn't installed on this machine.")
		return nil
	}
	fmt.Printf("T3 Code answers on port %d.\n", st.Port)
	if st.Problem != "" {
		fmt.Println("Linked environments:", st.Problem)
		return nil
	}
	if strings.TrimSpace(st.Linked) == "[]" || strings.TrimSpace(st.Linked) == "" {
		fmt.Println("No linked machines yet: run `spinup t3 link` on the main machine.")
		return nil
	}
	fmt.Println("Linked:", strings.TrimSpace(st.Linked))
	return nil
}

type t3SetupCmd struct{}

func (t3SetupCmd) Run() error { return setUpT3(context.Background(), installAlways) }

type t3LinkCmd struct{}

func (t3LinkCmd) Run() error {
	summary, err := syncT3Links(context.Background())
	if err != nil {
		return err
	}
	fmt.Println(summary)
	return nil
}
