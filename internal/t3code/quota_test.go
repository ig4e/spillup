package t3code

import (
	"os"
	"path/filepath"
	"slices"
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
	secret := t3.quotaSecret("local-cliproxyapi")
	if filepath.Base(secret) != "usage-limit-source-bG9jYWwtY2xpcHJveHlhcGk.bin" {
		t.Fatalf("secret file %s", secret)
	}
	must(os.WriteFile(secret, []byte("sk-api-key"), 0o600))

	if got := t3.StaleQuotaSources(8317, "mgmt"); len(got) != 1 || got[0] != "local-cliproxyapi" {
		t.Fatalf("stale = %v", got)
	}
	before := time.Unix(1, 0)
	must(os.Chtimes(t3.settingsPath(), before, before))
	n, err := t3.FixQuotaKeys(8317, "mgmt")
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
	if got := t3.StaleQuotaSources(8317, "mgmt"); len(got) != 0 {
		t.Fatalf("still stale: %v", got)
	}
	hosts := []string{"hub.tail1234.ts.net", "100.64.0.12"}
	got := t3.StaleQuotaSources(8317, "mgmt", hosts...)
	slices.Sort(got)
	if !slices.Equal(got, []string{"hub-dns", "hub-ip"}) {
		t.Fatalf("stale Tailnet sources = %v", got)
	}
	n, err = t3.FixQuotaKeys(8317, "mgmt", hosts...)
	must(err)
	if n != 2 || len(t3.StaleQuotaSources(8317, "mgmt", hosts...)) != 0 {
		t.Fatalf("fixed %d Tailnet sources", n)
	}
	for _, id := range []string{"elsewhere", "other-port-on-hub"} {
		if _, err := os.Stat(t3.quotaSecret(id)); !os.IsNotExist(err) {
			t.Fatalf("unrelated quota source %q was touched: %v", id, err)
		}
	}

}
