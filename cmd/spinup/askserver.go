package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/config"
)

// askForPasswords, called with setupRun locked, lets sudo, run by the dashboard's setup without a terminal, ask for your password in the
// dashboard: sudo runs a helper script, the helper asks this listener on 127.0.0.1 with a one-time token,
// and the dashboard shows the question. Windows asks through its own admin prompt instead.
func askForPasswords(cmd *exec.Cmd, exe string) (stop func(), err error) {
	if runtime.GOOS == "windows" {
		return func() {}, nil
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(raw)
	helper := filepath.Join(config.StateDir(), "askpass.sh")
	script := "#!/bin/sh\nexec '" + strings.ReplaceAll(exe, "'", `'\''`) + "' askpass \"$1\"\n"
	if err := os.MkdirAll(filepath.Dir(helper), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(helper, []byte(script), 0o700); err != nil {
		return nil, err
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	srv := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ask" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Spinup-Askpass")), []byte(token)) != 1 {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		prompt, _ := io.ReadAll(io.LimitReader(r.Body, 1024))
		if answer, ok := waitForPassword(strings.TrimSpace(string(prompt))); ok {
			_, _ = io.WriteString(w, answer)
			return
		}
		http.Error(w, "cancelled", http.StatusGone)
	})}
	go func() { _ = srv.Serve(l) }()
	cmd.Env = append(os.Environ(), "SUDO_ASKPASS="+helper, "SPINUP_ASKPASS_ADDR="+l.Addr().String(), "SPINUP_ASKPASS_TOKEN="+token)
	return func() { _ = srv.Close() }, nil
}

// waitForPassword shows prompt in the dashboard and waits up to five minutes for an answer.
func waitForPassword(prompt string) (string, bool) {
	if prompt == "" {
		prompt = "Your password"
	}
	setupRun.Lock()
	setupRun.prompt = prompt
	answers := setupRun.answers
	setupRun.Unlock()
	defer func() {
		setupRun.Lock()
		setupRun.prompt = ""
		setupRun.Unlock()
	}()
	select {
	case answer := <-answers:
		return answer, answer != ""
	case <-time.After(5 * time.Minute):
		return "", false
	}
}
