package t3code

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestFixQuotaKeys(t *testing.T) {
	home := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(home, "userdata", "secrets"), 0o700))
	must(os.WriteFile(filepath.Join(home, "userdata", "settings.json"), []byte(`{"usageLimitSources":{
		"local-cliproxyapi":{"kind":"cliproxy","url":"http://127.0.0.1:8317","managementKey":"••••••"},
		"hub-dns":{"kind":"cliproxy","url":"http://HUB.tail1234.ts.net.:8317"},
        "hub-ip":{"kind":"cliproxy","url":"http://100.64.0.12:8317"},
        "elsewhere":{"kind":"cliproxy","url":"http://other.tail1234.ts.net:8317"},
        "other-port-on-hub":{"kind":"cliproxy","url":"http://hub.tail1234.ts.net:9999"},
		"other-port":{"kind":"cliproxy","url":"http://localhost:9999"}}}`), 0o600))
	t3 := T3{Home: home}
	secrets := func(id string) string { return filepath.Join(home, "userdata", "secrets", quotaSecretName(id)) }
	secret := secrets("local-cliproxyapi")
	endpoints := []QuotaEndpoint{{Port: 8317, Hosts: []string{"127.0.0.1"}}}
	if filepath.Base(secret) != "usage-limit-source-bG9jYWwtY2xpcHJveHlhcGk.bin" {
		t.Fatalf("secret file %s", secret)
	}
	must(os.WriteFile(secret, []byte("sk-api-key"), 0o600))

	if got, err := t3.StaleQuotaSources(endpoints, "mgmt"); err != nil || got != 1 {
		t.Fatalf("stale = %v", got)
	}
	before := time.Unix(1, 0)
	must(os.Chtimes(t3.settingsPath(), before, before))
	n, err := t3.FixQuotaKeys(endpoints, "mgmt")
	must(err)
	info, err := os.Stat(t3.settingsPath())
	must(err)
	if !info.ModTime().After(before) {
		t.Fatal("T3 settings watcher was not notified")
	}
	if n != 1 {
		t.Fatalf("fixed %d", n)
	}
	if data, _ := os.ReadFile(secret); string(data) != "mgmt" {
		t.Fatalf("secret = %q", data)
	}
	if got, err := t3.StaleQuotaSources(endpoints, "mgmt"); err != nil || got != 0 {
		t.Fatalf("still stale: %v", got)
	}
	endpoints[0].Hosts = append(endpoints[0].Hosts, "hub.tail1234.ts.net", "100.64.0.12")
	got, err := t3.StaleQuotaSources(endpoints, "mgmt")
	must(err)
	if got != 2 {
		t.Fatalf("stale Tailnet sources = %d", got)
	}
	n, err = t3.FixQuotaKeys(endpoints, "mgmt")
	must(err)
	got, err = t3.StaleQuotaSources(endpoints, "mgmt")
	must(err)
	if n != 2 || got != 0 {
		t.Fatalf("fixed %d Tailnet sources", n)
	}
	for _, id := range []string{"elsewhere", "other-port-on-hub"} {
		if _, err := os.Stat(secrets(id)); !os.IsNotExist(err) {
			t.Fatalf("unrelated quota source %q was touched: %v", id, err)
		}
	}

}

func TestDirectQuotaOriginAndSettingsStayInsideTheirTrustBoundary(t *testing.T) {
	q := QuotaEndpoint{Port: 8327, Hosts: []string{"127.0.0.1"}}
	if !q.matches("http://127.0.0.1:8327/") {
		t.Fatal("missed the managed direct proxy")
	}
	for _, raw := range []string{"http://hub.tail1234.ts.net:8327", "http://127.0.0.2:8327", "http://[::1]:8327", "http://user@127.0.0.1:8327", "http://127.0.0.1:8327/external", "http://127.0.0.1:8327/%2f", "http://127.0.0.1:8327/?override=x", "http://127.0.0.1:8327/#x", "http://127.0.0.1:8328", "https://127.0.0.1:8327"} {
		if q.matches(raw) {
			t.Fatalf("unsafe origin %s", raw)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	home := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "userdata"), 0o700); err != nil {
		t.Fatal(err)
	}
	t3 := T3{Home: home}
	if err := os.WriteFile(t3.settingsPath(), []byte(`{"usageLimitSources":{"direct":{"kind":"cliproxy","url":"http://127.0.0.1:8327"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "userdata", "secrets")
	if err := os.Symlink(outside, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := t3.FixQuotaKeys([]QuotaEndpoint{q}, "management"); err == nil {
		t.Fatal("followed an external secrets directory")
	}
	if _, err := os.Stat(filepath.Join(outside, quotaSecretName("direct"))); !os.IsNotExist(err) {
		t.Fatal("wrote outside T3")
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(outside, "untouched")
	if err := os.WriteFile(foreign, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, filepath.Join(dir, quotaSecretName("direct"))); err != nil {
		t.Fatal(err)
	}
	if _, err := t3.FixQuotaKeys([]QuotaEndpoint{q}, "management"); err == nil {
		t.Fatal("followed an external key file")
	}
	if raw, _ := os.ReadFile(foreign); string(raw) != "original" {
		t.Fatal("overwrote external file")
	}
}
