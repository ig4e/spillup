package main

import (
	"context"
	"strings"
	"testing"
)

func TestTheDashboardRunsOnlySpinupsOwnFixes(t *testing.T) {
	for _, fix := range []string{"rm -rf /", "spinup setup pc --hub", "spinup uninstall", "spinup", "tailscale up", "spinup takeover -y"} {
		if _, err := (cliBackend{}).RunFix(context.Background(), fix); err == nil || !strings.Contains(err.Error(), "only spinup's own fixes") {
			t.Errorf("%q: %v, want a refusal", fix, err)
		}
	}
}

func TestSetupFromTheDashboardChecksTheNameAndRole(t *testing.T) {
	if err := (cliBackend{}).StartSetup("My PC", "hub"); err == nil {
		t.Error("a name with spaces and capitals was accepted")
	}
	if err := (cliBackend{}).StartSetup("desk", "boss"); err == nil {
		t.Error("an unknown role was accepted")
	}
}
