//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClipboardPlainTextHasAPrivateBoundedRead(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input")
	helper := filepath.Join(dir, "wl-paste")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nexec /bin/cat \"$SPILLUP_TEST_CLIPBOARD\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPILLUP_TEST_CLIPBOARD", input)
	t.Setenv("WAYLAND_DISPLAY", "test")
	t.Setenv("PATH", dir)
	for _, tc := range []struct {
		value string
		valid bool
	}{{"hello\nβ", true}, {strings.Repeat("x", clipboardLimit), true}, {strings.Repeat("x", clipboardLimit+1), false}, {"a\x00b", false}} {
		if err := os.WriteFile(input, []byte(tc.value), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := readClipboard(t.Context())
		if tc.valid != (err == nil) || (tc.valid && got != tc.value) || (!tc.valid && got != "") {
			t.Errorf("clipboard length %d, expected valid %v: %v", len(tc.value), tc.valid, err)
		}
	}
}
