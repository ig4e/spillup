package service

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
)

func postBody(addr, path, key, body string) int {
	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+path, strings.NewReader(body))
	req.Header.Set(api.KeyHeader, key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return -1
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestMakingTheStandbyMainMovesTheAccountsWithoutTwoProxies(t *testing.T) {
	tn := newTestTailnet(t)
	hub := tn.add("hub", config.HoldHub)
	sb := tn.add("sb", config.HoldStandby)
	tn.start(hub)
	tn.waitFor("the hub leads", tn.leaderIs("hub"))
	tn.start(sb)
	writeLogin(t, hub, "acct.json", "t1")
	tn.waitFor("the standby is synced", func() bool { return refreshToken(sb, "acct.json") == "t1" })
	tn.waitFor("the hub sees the standby", func() bool { return len(hub.m.peers.holders()) == 1 })

	if got := postBody(hub.peerAPI, api.PathMain, "m", `{"machine":"sb"}`); got != http.StatusNotFound {
		t.Errorf("main from the tailnet: %d, want 404", got)
	}
	if got := postBody(hub.front, api.PathMain, "m", `{"machine":"sb"}`); got != http.StatusOK {
		t.Fatalf("main: %d, want 200", got)
	}
	if hub.m.hold() != config.HoldStandby || sb.m.hold() != config.HoldHub {
		t.Fatalf("after making sb main: hub is %s, sb is %s", hub.m.hold(), sb.m.hold())
	}
	tn.waitFor("the new hub gets the accounts", tn.leaderIs("sb"))
	if token := refreshToken(sb, "acct.json"); token != "t1" {
		t.Fatalf("the new hub leads with token %q, want t1", token)
	}

	tn.shutDown(sb)
	tn.shutDown(hub)
	if most := tn.most.Load(); most != 1 {
		t.Fatalf("%d proxies ran at once; never more than 1", most)
	}
}

func TestARoleChangeIsOnlyBetweenHubAndStandbyAndIsSaved(t *testing.T) {
	tn := newTestTailnet(t)
	hub := tn.add("hub", config.HoldHub)
	tn.start(hub)
	tn.waitFor("the hub leads", tn.leaderIs("hub"))
	var saved config.Config
	hub.m.o.SaveConfig = func(c config.Config) error { saved = c; return nil }

	if got := postBody(hub.front, api.PathRole, "m", `{"hold":"never"}`); got != http.StatusConflict {
		t.Errorf("hub to never: %d, want 409", got)
	}
	if got := postBody(hub.front, api.PathRole, "wrong", `{"hold":"standby"}`); got != http.StatusUnauthorized {
		t.Errorf("role without the key: %d, want 401", got)
	}
	if got := postBody(hub.front, api.PathRole, "m", `{"hold":"standby"}`); got != http.StatusOK {
		t.Fatalf("hub to standby: %d, want 200", got)
	}
	if saved.Hold != config.HoldStandby || hub.m.Report().Hold != config.HoldStandby {
		t.Fatalf("saved %q, reported %q; want standby", saved.Hold, hub.m.Report().Hold)
	}
	time.Sleep(200 * time.Millisecond)
	if !hub.m.Report().Leading {
		t.Fatal("a lone standby must keep the accounts it holds")
	}
	tn.shutDown(hub)
}

func TestAMachineThatNeverHoldsCannotChangeRoles(t *testing.T) {
	tn := newTestTailnet(t)
	laptop := tn.add("laptop", config.HoldNever)
	tn.start(laptop)
	tn.waitFor("the laptop answers", func() bool { return !strings.HasPrefix(get(laptop.front, api.PathLeader), "error") })
	if got := postBody(laptop.front, api.PathRole, "", `{"hold":"hub"}`); got != http.StatusUnauthorized {
		t.Errorf("role on a machine without the password: %d, want 401", got)
	}
	tn.shutDown(laptop)
}
