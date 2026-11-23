package service

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
)

func TestMainMachineOnlyKeepsTheLoginsOnTheHubAndNeverFailsOver(t *testing.T) {
	tn := newTestTailnet(t)
	hub := tn.add("hub", config.HoldHub)
	sb := tn.add("sb", config.HoldStandby)
	tn.start(hub)
	tn.waitFor("the hub leads", tn.leaderIs("hub"))
	tn.start(sb)
	writeLogin(t, hub, "acct.json", "t1")
	tn.waitFor("the standby gets its copy", func() bool { return refreshToken(sb, "acct.json") == "t1" })

	if got := postBody(sb.front, api.PathMode, "m", `{"hub_only":true}`); got != http.StatusOK {
		t.Fatalf("main machine only, asked on the standby: %d, want 200 (passed on to the hub)", got)
	}
	if !hub.m.hubOnly.get() {
		t.Fatal("the hub didn't take the setting the standby passed on")
	}
	tn.waitFor("the standby learns it and drops its copy", func() bool {
		_, err := os.Stat(filepath.Join(sb.m.cfg.AuthDir, "acct.json"))
		return sb.m.hubOnly.get() && os.IsNotExist(err)
	})
	writeLogin(t, hub, "acct.json", "t2")
	time.Sleep(500 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(sb.m.cfg.AuthDir, "acct.json")); !os.IsNotExist(err) {
		t.Fatal("the hub sent a refreshed login to a standby in main machine only")
	}

	tn.crash(hub)
	time.Sleep(2500 * time.Millisecond) // well past FailoverAfter
	if l := tn.leaders(); len(l) != 0 {
		t.Fatalf("with main machine only, %v took the accounts while the hub was gone", l)
	}

	tn.start(hub)
	tn.waitFor("the hub leads again", tn.leaderIs("hub"))
	if token := refreshToken(hub, "acct.json"); token != "t2" {
		t.Fatalf("the hub lost its login: %q", token)
	}
	tn.shutDown(sb)
	tn.shutDown(hub)
	if most := tn.most.Load(); most != 1 {
		t.Fatalf("%d proxies ran at once; never more than 1", most)
	}
}
