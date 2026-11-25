package mcpvault

import (
	"bytes"
	"context"
	"errors"
	"github.com/darkyeg/spinup/internal/library"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPrivateVaultAndSingleUseConsent(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	ctx := t.Context()
	if has, err := s.Has("test", "TOKEN"); has || err != nil {
		t.Fatal(has, err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, ".mcp-vault")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("query created a vault")
	}
	if err := s.Set(ctx, "test", "TOKEN", "previous"); err != nil {
		t.Fatal(err)
	}
	if err := s.Allow(ctx, "test", "TOKEN", "one"); err != nil {
		t.Fatal(err)
	}
	if err := s.Allow(ctx, "test", "ONLY_GRANT", "one"); err != nil {
		t.Fatal(err)
	}
	if keys, err := s.Keys(); err != nil || len(keys) != 1 || len(keys["test"]) != 1 || keys["test"][0] != "TOKEN" {
		t.Fatal("incorrect saved key names", keys, err)
	}
	if _, err := s.Permit("test", "TOKEN", "two"); !errors.Is(err, ErrConsent) {
		t.Fatal("foreign peer approved")
	}
	token, err := s.Permit("test", "TOKEN", "one")
	if err != nil {
		t.Fatal(err)
	}
	for _, server := range []string{"other", "test"} {
		err := s.Receive(ctx, server, "TOKEN", "one", "wrong", "synthetic")
		if !errors.Is(err, ErrConsent) {
			t.Fatal("wrong permit accepted")
		}
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := s.Receive(ctx, "test", "TOKEN", "one", token, "synth\nvalue"); err == nil {
				successes.Add(1)
			} else if !errors.Is(err, ErrConsent) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("%d approvals consumed", successes.Load())
	}
	if got, err := s.Get("test", "TOKEN"); got != "synth\nvalue" || err != nil {
		t.Fatal("missing received value")
	}
	if runtime.GOOS != "windows" {
		file, err := os.Stat(filepath.Join(s.Dir, ".mcp-vault", "secrets.json"))
		if err != nil || file.Mode().Perm() != 0o600 {
			t.Fatal("vault permissions")
		}
	}
	if err := s.Forget(ctx, "test", "TOKEN"); err != nil {
		t.Fatal(err)
	}
	if has, err := s.Has("test", "TOKEN"); has || err != nil {
		t.Fatal(has, err)
	}
}

func TestVaultDeniesExpiredAndRevokedGrantsAndSymlinks(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	ctx := context.Background()
	if err := s.Allow(ctx, "probe", "TOKEN", "src"); err != nil {
		t.Fatal(err)
	}
	if err := s.edit(ctx, "probe", "TOKEN", func(e *entry) error { e.Grant.Until = time.Now().Add(-time.Minute); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Permit("probe", "TOKEN", "src"); !errors.Is(err, ErrConsent) {
		t.Fatal("expired permit")
	}
	if err := s.Allow(ctx, "probe", "TOKEN", "src"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(ctx, "probe", "TOKEN", "local"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Permit("probe", "TOKEN", "src"); !errors.Is(err, ErrConsent) {
		t.Fatal("local edit left grant live")
	}
	for _, k := range []string{"a/b", "../k", "X=Y", ""} {
		if s.Set(ctx, "probe", k, "local") == nil {
			t.Fatal("invalid key")
		}
	}
	if s.Set(ctx, "../peer", "TOKEN", "new") == nil {
		t.Fatal("invalid server")
	}
	if runtime.GOOS == "windows" {
		return
	}
	p := filepath.Join(s.Dir, ".mcp-vault", "secrets.json")
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "unrelated")
	if err := os.Symlink(dest, p); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(ctx, "probe", "TOKEN", "oops"); err == nil {
		t.Fatal("wrote via a symlink")
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("overwrote unrelated target")
	}
}

func TestPrivateVaultIsExcludedEvenInsideASharedSkill(t *testing.T) {
	lib := library.At(t.TempDir())
	s := Store{Dir: lib.Path("skills/example/state")}
	if err := s.Set(t.Context(), "example", "TOKEN", "synthetic-private-secret"); err != nil {
		t.Fatal(err)
	}
	files, err := lib.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if bytes.Contains(f.Data, []byte("synthetic-private-secret")) {
			t.Fatalf("vault entered snapshot under %s", f.Path)
		}
	}
}
