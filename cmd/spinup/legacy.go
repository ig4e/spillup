package main

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/host"
	"github.com/darkyeg/spinup/internal/legacyproxy"
	"github.com/darkyeg/spinup/internal/logins"
)

// findLegacyProxy is a CLIProxyAPI this machine ran before spillup, never spillup's own.
func findLegacyProxy(cfg config.Config) (legacyproxy.Found, bool) {
	return legacyproxy.Find(host.Home(), cfg.ProxyConfig())
}

func keysBesides(keys []string, shared string) []string {
	var out []string
	for _, k := range keys {
		if k != shared && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

// importLegacyLogins takes an earlier CLIProxyAPI's logins into the main machine, once, after setup has
// stopped that proxy: copied while it still ran, a login it refreshed afterwards would be stale here.
func importLegacyLogins(legacy legacyproxy.Found, cfg config.Config) {
	if len(logins.Summarize(cfg.AuthDir)) > 0 || same(legacy.AuthDir, cfg.AuthDir) {
		return
	}
	names := legacyproxy.Logins(legacy.AuthDir)
	if len(names) == 0 {
		return
	}
	if err := os.MkdirAll(cfg.AuthDir, 0o700); err != nil {
		step("Couldn't take in your old logins: %v", err)
		return
	}
	copied := 0
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(legacy.AuthDir, n))
		if err == nil && os.WriteFile(filepath.Join(cfg.AuthDir, n), data, 0o600) == nil {
			copied++
		}
	}
	step("Took in %d login(s) from your old CLIProxyAPI (%s); the old copies stay where they were", copied, legacy.AuthDir)
}

func same(a, b string) bool {
	ca, _ := filepath.Abs(a)
	cb, _ := filepath.Abs(b)
	return filepath.Clean(ca) == filepath.Clean(cb)
}
