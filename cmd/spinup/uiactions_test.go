package main

import (
	"context"
	"strings"
	"testing"
)

func TestTheDashboardRunsOnlySpinupsOwnFixes(t *testing.T) {
	for _, fix := range []string{"rm -rf /", "spinup setup pc --hub", "spinup uninstall", "spinup", "tailscale up", "spinup takeover -y", "spinup skills import example", "spinup packages add example", "spinup tools --help", "spinup t3 setup extra"} {
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

func TestTaskProgressTracksStepsAndKeepsBoundedOutput(t *testing.T) {
	setupRun.Lock()
	oldLog, oldPhase, oldStep, oldTotal, oldUpdated := setupRun.log, setupRun.phase, setupRun.step, setupRun.total, setupRun.updated
	setupRun.log, setupRun.phase, setupRun.step, setupRun.total = nil, "", 0, 0
	setupRun.Unlock()
	t.Cleanup(func() {
		setupRun.Lock()
		setupRun.log, setupRun.phase, setupRun.step, setupRun.total, setupRun.updated = oldLog, oldPhase, oldStep, oldTotal, oldUpdated
		setupRun.Unlock()
	})
	recordTaskLine("[3/9] Dev tools")
	for range setupLogLines + 1 {
		recordTaskLine("output")
	}
	p := (cliBackend{}).SetupProgress()
	if p.Step != 3 || p.Total != 9 || p.Phase != "Dev tools" || len(p.Log) != setupLogLines {
		t.Fatalf("progress after rolling output: step=%d total=%d phase=%q lines=%d", p.Step, p.Total, p.Phase, len(p.Log))
	}
}
