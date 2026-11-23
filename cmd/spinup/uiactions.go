package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/darkyeg/spinup/internal/sysproc"
	"github.com/darkyeg/spinup/internal/tailnet"
	"github.com/darkyeg/spinup/internal/webui"
)

// fixable are the spinup commands a doctor fix may name that the dashboard runs with one click: they
// change only this machine's own setup, and none asks a question.
var fixable = []string{"skills", "update", "tools", "agents", "packages", "t3"}

// RunFix runs a doctor fix that is a spinup command, such as `spinup skills sync`, as a button.
func (cliBackend) RunFix(ctx context.Context, fix string) (string, error) {
	args := strings.Fields(fix)
	if len(args) < 2 || args[0] != "spinup" || !slices.Contains(fixable, args[1]) {
		return "", fmt.Errorf("the dashboard runs only spinup's own fixes, not %q", fix)
	}
	if args[1] == "update" {
		return "Updating. spillup restarts in a minute.", startDetachedUpdate()
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args[1:]...)
	sysproc.Hide(cmd)
	out, err := cmd.CombinedOutput()
	last := lastOutputLine(string(out))
	if err != nil {
		return "", fmt.Errorf("%s: %s", strings.Join(args, " "), last)
	}
	return firstNonEmpty(last, "Done."), nil
}

func lastOutputLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	return strings.TrimPrefix(strings.TrimSpace(lines[len(lines)-1]), "==> ")
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// setupRun is the one `spinup setup` the dashboard may run at a time, and what it has printed.
var setupRun struct {
	sync.Mutex
	running bool
	failed  string
	done    bool
	log     []string
}

const setupLogLines = 400

// StartSetup sets this machine up from the dashboard, as `spinup setup <name> [--hub|--standby]` does.
func (cliBackend) StartSetup(name, role string) error {
	if !tailnet.ValidName(name) {
		return fmt.Errorf("%q can't be a machine name: use lowercase letters, digits and dashes", name)
	}
	args := []string{"setup", name}
	switch role {
	case "hub":
		args = append(args, "--hub")
	case "standby":
		args = append(args, "--standby")
	case "use":
	default:
		return fmt.Errorf("no role called %q", role)
	}
	setupRun.Lock()
	defer setupRun.Unlock()
	if setupRun.running {
		return errors.New("setup is already running")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, args...)
	sysproc.Hide(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	setupRun.running, setupRun.done, setupRun.failed, setupRun.log = true, false, "", nil
	go followSetup(cmd, out)
	return nil
}

func followSetup(cmd *exec.Cmd, out interface{ Read([]byte) (int, error) }) {
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		setupRun.Lock()
		setupRun.log = append(setupRun.log, sc.Text())
		if len(setupRun.log) > setupLogLines {
			setupRun.log = setupRun.log[len(setupRun.log)-setupLogLines:]
		}
		setupRun.Unlock()
	}
	err := cmd.Wait()
	setupRun.Lock()
	defer setupRun.Unlock()
	setupRun.running, setupRun.done = false, true
	if err != nil {
		setupRun.failed = err.Error()
	}
}

func (cliBackend) SetupProgress() webui.SetupProgress {
	setupRun.Lock()
	defer setupRun.Unlock()
	return webui.SetupProgress{Running: setupRun.running, Done: setupRun.done, Failed: setupRun.failed, Log: slices.Clone(setupRun.log)}
}
