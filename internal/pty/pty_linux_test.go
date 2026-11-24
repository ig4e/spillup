package pty

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func readUntil(t *testing.T, term *Term, want ...string) string {
	t.Helper()
	got := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		chunk := make([]byte, 1024)
		for {
			n, err := term.Read(chunk)
			buf.Write(chunk[:n])
			if containsAll(buf.String(), want) || err != nil {
				got <- buf.String()
				return
			}
		}
	}()
	select {
	case out := <-got:
		if !containsAll(out, want) {
			t.Fatalf("output %q lacks %q", out, want)
		}
		return out
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %q", want)
		return ""
	}
}

func containsAll(s string, want []string) bool {
	for _, w := range want {
		if !strings.Contains(s, w) {
			return false
		}
	}
	return true
}

func TestStartSizesTheTerminal(t *testing.T) {
	term, err := Start([]string{"/bin/sh", "-c", "stty size; echo hello"}, t.TempDir(), nil, 100, 30)
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	readUntil(t, term, "30 100", "hello")
	if err := term.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(term); err != nil {
		t.Fatalf("reading after exit: %v, want EOF", err)
	}
}

func TestResize(t *testing.T) {
	term, err := Start([]string{"/bin/sh"}, "", nil, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	if err := term.Resize(120, 40); err != nil {
		t.Fatal(err)
	}
	if _, err := term.Write([]byte("stty size; exit\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, term, "40 120")
	if err := term.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestCloseKillsTheShell(t *testing.T) {
	term, err := Start([]string{"sleep", "100"}, "", nil, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if err := term.Close(); err != nil {
		t.Fatal(err)
	}
	if err := term.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- term.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("sleep exited cleanly; want killed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not kill the shell")
	}
}
