package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/filelock"
	"github.com/darkyeg/spinup/internal/sysproc"
	"github.com/darkyeg/spinup/internal/tailnet"
	"github.com/darkyeg/spinup/internal/webui"
)

// fixable are the spinup commands a doctor fix may name that the dashboard runs with one click: they
// change only this machine's own setup, and none asks a question.
var fixable = []string{"skills sync", "update", "tools", "agents", "packages sync", "t3 setup", "t3 link", "restart"}

// RunFix runs a doctor fix that is a spinup command, such as `spinup skills sync`, as a button.
func (cliBackend) RunFix(ctx context.Context, fix string) (string, error) {
	args := strings.Fields(fix)
	if len(args) < 2 || args[0] != "spinup" || !slices.Contains(fixable, strings.Join(args[1:], " ")) {
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
	cmd.WaitDelay = 5 * time.Second
	sysproc.Hide(cmd)
	out, err := cmd.CombinedOutput()
	last := lastOutputLine(string(out))
	if ctx.Err() != nil {
		return "", fmt.Errorf("%s: %w", fix, ctx.Err())
	}
	if err != nil {
		return "", fmt.Errorf("%s: %s", fix, firstNonEmpty(last, err.Error()))
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
	task    string
	phase   string
	step    int
	total   int
	started time.Time
	updated time.Time
	// prompt is what sudo asks while it waits for a password; answers carries the reply, "" cancels.
	prompt  string
	answers chan string
}

const (
	setupLogLines    = 200
	taskTimeout      = 2 * time.Hour
	taskLogLineLimit = 8 << 10
)

var progressLine = regexp.MustCompile(`^\[(\d+)/(\d+)\] (.+)$`)

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
	lockCtx, stop := context.WithTimeout(context.Background(), time.Second)
	lock, err := filelock.Acquire(lockCtx, filepath.Join(config.StateDir(), "task.lock"))
	stop()
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("another dashboard task is already running")
	}
	if err != nil {
		return err
	}
	setupRun.answers = make(chan string)
	ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
	out, wait, err := launchTask(ctx, exe, args)
	if err != nil {
		lock.Close()
		cancel()
		return err
	}
	setupRun.running, setupRun.done, setupRun.failed, setupRun.log = true, false, "", nil
	setupRun.task, setupRun.phase, setupRun.prompt, setupRun.step, setupRun.total = args[0], "", "", 0, 0
	setupRun.started = time.Now()
	setupRun.updated = setupRun.started
	go func() {
		defer cancel()
		defer lock.Close()
		followSetup(ctx, out, wait)
	}()
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

func followSetup(ctx context.Context, out io.Reader, wait func() error) {
	reader := bufio.NewReaderSize(out, taskLogLineLimit)
	for {
		line, partial, err := reader.ReadLine()
		if len(line) != 0 {
			text := string(line)
			if partial {
				text += "…"
			}
			recordTaskLine(text)
		}
		if err != nil {
			break
		}
	}
	err := wait()
	setupRun.Lock()
	defer setupRun.Unlock()
	setupRun.running, setupRun.done, setupRun.prompt = false, true, ""
	if ctx.Err() != nil {
		setupRun.failed = "The task exceeded its two-hour time limit"
	} else if err != nil {
		setupRun.failed = err.Error()
	}
}

func recordTaskLine(line string) {
	setupRun.Lock()
	defer setupRun.Unlock()
	setupRun.updated = time.Now()
	if setupRun.total == 0 && strings.HasPrefix(line, "==> ") {
		phase := strings.TrimPrefix(line, "==> ")
		setupRun.phase = phase[:min(len(phase), 160)]
	}
	if match := progressLine.FindStringSubmatch(line); match != nil {
		step, _ := strconv.Atoi(match[1])
		total, _ := strconv.Atoi(match[2])
		if step > 0 && step <= total && total <= 100 {
			setupRun.step, setupRun.total, setupRun.phase = step, total, match[3]
		}
	}
	setupRun.log = append(setupRun.log, line)
	if len(setupRun.log) > setupLogLines {
		setupRun.log = setupRun.log[len(setupRun.log)-setupLogLines:]
	}
}

func (cliBackend) SetupProgress() webui.SetupProgress {
	setupRun.Lock()
	defer setupRun.Unlock()
	progress := webui.SetupProgress{
		Running: setupRun.running, Done: setupRun.done, Failed: setupRun.failed,
		Log: slices.Clone(setupRun.log), Prompt: setupRun.prompt, Task: setupRun.task,
		Phase: setupRun.phase, Step: setupRun.step, Total: setupRun.total,
	}
	if !setupRun.started.IsZero() && setupRun.running {
		progress.ElapsedSeconds = int(time.Since(setupRun.started).Seconds())
		progress.QuietSeconds = int(time.Since(setupRun.updated).Seconds())
	}
	return progress
}
