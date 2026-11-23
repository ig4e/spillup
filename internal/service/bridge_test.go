package service

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
)

func bridgeCall(addr, header, key string) (int, string) {
	req, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/spinup/ui/api/overview", nil)
	if header != "" {
		req.Header.Set(header, key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return -1, err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestTheMainMachineReachesEveryMachinesDashboardWithTheRightKey(t *testing.T) {
	tn := newTestTailnet(t)
	hub := tn.add("hub", config.HoldHub)
	hub.dashboard = true
	laptop := tn.add("laptop", config.HoldNever)
	laptop.dashboard = true
	tn.start(hub)
	tn.waitFor("the hub leads", tn.leaderIs("hub"))
	tn.start(laptop)
	tn.waitFor("the laptop's dashboard answers on the tailnet", func() bool {
		code, _ := bridgeCall(laptop.peerAPI, api.APIKeyHeader, "k")
		return code == http.StatusOK
	})

	if code, body := bridgeCall(laptop.peerAPI, api.APIKeyHeader, "k"); !strings.Contains(body, "dashboard of laptop at /spinup/ui/api/overview") {
		t.Errorf("laptop dashboard with the API key: %d %q", code, body)
	}
	if code, _ := bridgeCall(laptop.peerAPI, "", ""); code != http.StatusUnauthorized {
		t.Errorf("laptop dashboard without a key: %d, want 401", code)
	}
	if body := get(laptop.peerAPI, "/v1/models"); strings.Contains(body, "on hub") {
		t.Errorf("a machine that only uses the accounts routed tailnet traffic: %q", body)
	}

	if code, _ := bridgeCall(hub.peerAPI, api.APIKeyHeader, "k"); code != http.StatusUnauthorized {
		t.Errorf("hub dashboard with only the API key: %d, want 401: a machine that only uses the accounts must not steer a holder", code)
	}
	if code, body := bridgeCall(hub.peerAPI, api.KeyHeader, "m"); code != http.StatusOK || !strings.Contains(body, "dashboard of hub") {
		t.Errorf("hub dashboard with the dashboard password: %d %q", code, body)
	}
	if code, _ := bridgeCall(hub.front, api.KeyHeader, "m"); code == http.StatusOK {
		t.Error("the dashboard bridge must be on the tailnet only, not localhost")
	}

	tn.shutDown(laptop)
	tn.shutDown(hub)
	if most := tn.most.Load(); most != 1 {
		t.Fatalf("%d proxies ran at once; never more than 1", most)
	}
}
