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
	"strconv"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/sysproc"
)

const (
	taskName   = "spinup-dashboard-task"
	doneMarker = "__spinup_task_done__"
)

// launchTask runs a spinup command in your desktop session, where Windows can show its admin prompts: the
// service itself runs without a desktop. A one-off scheduled task runs it; its output comes through a file.
func launchTask(ctx context.Context, exe string, args []string) (io.Reader, func() error, error) {
	logPath := filepath.Join(config.StateDir(), "task.log")
	_ = os.Remove(logPath)
	if err := os.WriteFile(logPath, nil, 0o600); err != nil {
		return nil, nil, err
	}
	line := `"` + exe + `"`
	for _, a := range args {
		line += ` "` + strings.ReplaceAll(a, `"`, ``) + `"`
	}
	script := filepath.Join(config.StateDir(), "task.cmd")
	body := "@echo off\r\n" + line + " >> \"" + logPath + "\" 2>&1\r\necho " + doneMarker + " %errorlevel% >> \"" + logPath + "\"\r\n"
	if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
		return nil, nil, err
	}
	for _, a := range [][]string{
		{"/Create", "/F", "/TN", taskName, "/TR", `cmd.exe /c "` + script + `"`, "/SC", "ONCE", "/ST", "23:59", "/IT"},
		{"/Run", "/TN", taskName},
	} {
		if err := windowsTaskCommand(ctx, a...); err != nil {
			cleanWindowsTask()
			return nil, nil, err
		}
	}
	r, w := io.Pipe()
	result := make(chan error, 1)
	go func() { result <- tail(ctx, logPath, w) }()
	return r, func() error {
		err := <-result
		if ctx.Err() != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = windowsTaskCommand(cleanup, "/End", "/TN", taskName)
			cancel()
		}
		cleanWindowsTask()
		return err
	}, nil
}

func windowsTaskCommand(ctx context.Context, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "schtasks.exe", args...)
	cmd.WaitDelay = 3 * time.Second
	sysproc.Hide(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("Windows task: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func cleanWindowsTask() {
	_ = windowsTaskCommand(context.Background(), "/Delete", "/F", "/TN", taskName)
}

// tail copies the task's log as it grows, until the done marker, and reports the exit code it carries.
func tail(ctx context.Context, path string, w *io.PipeWriter) error {
	defer w.Close()
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	reader := bufio.NewReaderSize(f, taskLogLineLimit)
	var partial string
	for ctx.Err() == nil {
		chunk, err := reader.ReadSlice('\n')
		partial += string(chunk)
		if errors.Is(err, io.EOF) && len(partial) < taskLogLineLimit {
			sleepCtx(ctx, 300*time.Millisecond)
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
			return err
		}
		line := strings.TrimRight(partial, "\r\n")
		partial = ""
		if code, ok := strings.CutPrefix(line, doneMarker); ok {
			n, err := strconv.Atoi(strings.TrimSpace(code))
			if err != nil {
				return errors.New("Windows task returned an invalid exit status")
			}
			if n != 0 {
				return fmt.Errorf("exit status %d", n)
			}
			return nil
		}
		fmt.Fprintln(w, line)
	}
	return ctx.Err()
}
