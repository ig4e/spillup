package main

import (
	"bufio"
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
func launchTask(exe string, args []string) (io.Reader, func() error, error) {
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
		cmd := exec.Command("schtasks.exe", a...)
		sysproc.Hide(cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, nil, fmt.Errorf("Windows didn't start the task: %s", strings.TrimSpace(string(out)))
		}
	}
	r, w := io.Pipe()
	result := make(chan error, 1)
	go func() { result <- tail(logPath, w) }()
	return r, func() error {
		err := <-result
		cleanup := exec.Command("schtasks.exe", "/Delete", "/F", "/TN", taskName)
		sysproc.Hide(cleanup)
		_ = cleanup.Run()
		return err
	}, nil
}

// tail copies the task's log as it grows, until the done marker, and reports the exit code it carries.
func tail(path string, w *io.PipeWriter) error {
	defer w.Close()
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	deadline := time.Now().Add(2 * time.Hour)
	var partial string
	for time.Now().Before(deadline) {
		chunk, err := reader.ReadString('\n')
		partial += chunk
		if err != nil {
			time.Sleep(300 * time.Millisecond)
			continue
		}
		line := strings.TrimRight(partial, "\r\n")
		partial = ""
		if code, ok := strings.CutPrefix(line, doneMarker); ok {
			if n, _ := strconv.Atoi(strings.TrimSpace(code)); n != 0 {
				return fmt.Errorf("exit status %d", n)
			}
			return nil
		}
		fmt.Fprintln(w, line)
	}
	return errors.New("the task didn't finish within two hours")
}
