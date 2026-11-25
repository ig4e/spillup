package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/darkyeg/spinup/internal/sysproc"
	"github.com/darkyeg/spinup/internal/webui"
)

const fleetPreviewLimit = 128 << 10

var fleetPreviewBusy sync.Mutex

func (cliBackend) RolloutPlan(ctx context.Context) (webui.FleetPreview, error) {
	if !fleetPreviewBusy.TryLock() {
		return webui.FleetPreview{}, errors.New("another fleet preview is running")
	}
	defer fleetPreviewBusy.Unlock()
	exe, err := os.Executable()
	if err != nil {
		return webui.FleetPreview{}, errors.New("could not find the Spillup binary")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "rollout")
	sysproc.Hide(cmd)
	return readRolloutPlan(ctx, cmd), nil
}

type fleetPreviewOutput struct {
	buffer    bytes.Buffer
	truncated bool
}

func (b *fleetPreviewOutput) Write(p []byte) (int, error) {
	n := min(len(p), fleetPreviewLimit-b.buffer.Len())
	b.buffer.Write(p[:n])
	if n < len(p) {
		b.truncated = true
	}
	return len(p), nil
}

func readRolloutPlan(ctx context.Context, cmd *exec.Cmd) webui.FleetPreview {
	var output fleetPreviewOutput
	cmd.Stdout = &output
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	plan := webui.FleetPreview{Output: strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, output.buffer.String()))}
	switch {
	case ctx.Err() != nil:
		plan.Problem = "The scan stopped before every machine answered. Run spinup rollout locally for a longer scan."
	case err != nil:
		plan.Problem = "The fleet preview could not finish. Run spinup rollout locally for details."
	case output.truncated:
		plan.Problem = "The dashboard reached its output limit. Run spinup rollout locally for the full plan."
	}
	return plan
}
