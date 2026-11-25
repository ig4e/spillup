package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestFleetPreviewHelper(t *testing.T) {
	switch os.Getenv("SPINUP_TEST_FLEET_PREVIEW") {
	case "success":
		fmt.Fprintln(os.Stdout, "worker ready")
		fmt.Fprintln(os.Stderr, "private stderr")
		os.Exit(0)
	case "slow":
		fmt.Fprintln(os.Stdout, "first machine checked")
		time.Sleep(10 * time.Second)
		os.Exit(0)
	case "long":
		fmt.Fprint(os.Stdout, strings.Repeat("x", fleetPreviewLimit+1))
		os.Exit(0)
	}
}

func TestFleetPreviewBoundsAndKeepsPartialOutput(t *testing.T) {
	for _, c := range []struct {
		mode    string
		timeout time.Duration
	}{
		{"success", 5 * time.Second}, {"slow", time.Second}, {"long", 5 * time.Second},
	} {
		t.Run(c.mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFleetPreviewHelper$")
			cmd.Env = append(os.Environ(), "SPINUP_TEST_FLEET_PREVIEW="+c.mode)
			plan := readRolloutPlan(ctx, cmd)
			switch c.mode {
			case "success":
				if plan.Output != "worker ready" || plan.Problem != "" {
					t.Fatalf("preview = %+v", plan)
				}
			case "slow":
				if plan.Output != "first machine checked" || plan.Problem == "" {
					t.Fatalf("partial preview = %+v", plan)
				}
			case "long":
				if len(plan.Output) != fleetPreviewLimit || plan.Problem == "" {
					t.Fatalf("limit = %d, %s", len(plan.Output), plan.Problem)
				}
			}
		})
	}
}
