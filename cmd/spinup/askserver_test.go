//go:build !windows

package main

import (
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func envOf(cmd *exec.Cmd, key string) string {
	for _, kv := range cmd.Env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			return v
		}
	}
	return ""
}

func TestSudoAsksTheDashboardForThePassword(t *testing.T) {
	t.Setenv("SPINUP_HOME", t.TempDir())
	cmd := exec.Command("true")
	setupRun.Lock()
	setupRun.answers = make(chan string)
	stop, err := askForPasswords(cmd, "/usr/bin/spinup")
	setupRun.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if !strings.HasSuffix(envOf(cmd, "SUDO_ASKPASS"), "askpass.sh") {
		t.Fatalf("SUDO_ASKPASS is %q", envOf(cmd, "SUDO_ASKPASS"))
	}
	ask := func(token string) (int, string) {
		req, _ := http.NewRequest(http.MethodPost, "http://"+envOf(cmd, "SPINUP_ASKPASS_ADDR")+"/ask", strings.NewReader("[sudo] password for me: "))
		req.Header.Set("X-Spinup-Askpass", token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return -1, err.Error()
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	if code, _ := ask("wrong"); code != http.StatusForbidden {
		t.Fatalf("a helper without the token: %d, want 403", code)
	}
	got := make(chan string, 1)
	go func() { _, body := ask(envOf(cmd, "SPINUP_ASKPASS_TOKEN")); got <- body }()
	deadline := time.Now().Add(5 * time.Second)
	for (cliBackend{}).SetupProgress().Prompt == "" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if p := (cliBackend{}).SetupProgress().Prompt; !strings.Contains(p, "password for me") {
		t.Fatalf("the dashboard shows %q, want sudo's question", p)
	}
	if err := (cliBackend{}).AnswerSetup("hunter2", false); err != nil {
		t.Fatal(err)
	}
	if answer := <-got; answer != "hunter2" {
		t.Fatalf("sudo got %q", answer)
	}
	if p := (cliBackend{}).SetupProgress().Prompt; p != "" {
		t.Errorf("the question stayed up after the answer: %q", p)
	}
}
