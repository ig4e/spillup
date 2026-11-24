package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
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
var fixable = []string{"skills", "update", "tools", "agents", "packages", "t3", "restart"}

// RunFix runs a doctor fix that is a spinup command, such as `spinup skills sync`, as a button.
func (cliBackend) RunFix(ctx context.Context, fix string) (string, error) {
	args := strings.Fields(fix)
	if len(args) < 2 || args[0] != "spinup" || !slices.Contains(fixable, args[1]) {
		return "", fmt.Errorf("the dashboard runs only spinup's own fixes, not %q", fix)
	}
	switch args[1] {
	case "update":
		return "Updating. spillup restarts in a minute.", startDetachedUpdate()
	case "restart":
		return "Restarting spillup. This page reconnects in a few seconds.", cliBackend{}.RestartService()
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
	// prompt is what sudo asks while it waits for a password; answers carries the reply, "" cancels.
	prompt  string
	answers chan string
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
	return startTask(args)
}

// startTask runs a spinup command for the dashboard, one at a time, and keeps what it prints; sudo's
// questions and Windows' admin prompts reach you while it runs.
func startTask(args []string) error {
	setupRun.Lock()
	defer setupRun.Unlock()
	if setupRun.running {
		return errors.New("a setup is already running")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	setupRun.answers = make(chan string)
	out, wait, err := launchTask(exe, args)
	if err != nil {
		return err
	}
	setupRun.running, setupRun.done, setupRun.failed, setupRun.log = true, false, "", nil
	go followSetup(out, wait)
	return nil
}

// AnswerSetup gives sudo the password it asked for through the dashboard; cancel refuses.
func (cliBackend) AnswerSetup(password string, cancel bool) error {
	setupRun.Lock()
	answers, asked := setupRun.answers, setupRun.prompt != ""
	setupRun.Unlock()
	if !asked || answers == nil {
		return errors.New("setup isn't asking for a password right now")
	}
	if cancel {
		password = ""
	}
	select {
	case answers <- password:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("setup stopped waiting for the password")
	}
}

func followSetup(out io.Reader, wait func() error) {
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		setupRun.Lock()
		setupRun.log = append(setupRun.log, sc.Text())
		if len(setupRun.log) > setupLogLines {
			setupRun.log = setupRun.log[len(setupRun.log)-setupLogLines:]
		}
		setupRun.Unlock()
	}
	err := wait()
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
	return webui.SetupProgress{Running: setupRun.running, Done: setupRun.done, Failed: setupRun.failed, Log: slices.Clone(setupRun.log), Prompt: setupRun.prompt}
}
