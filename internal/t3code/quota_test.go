package t3code

import (
	"os"
	"path/filepath"
	"testing"
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
		"elsewhere":{"kind":"cliproxy","url":"http://example.test:8317"},
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
	n, err := t3.FixQuotaKeys(8317, "mgmt")
	must(err)
	if n != 1 {
		t.Fatalf("fixed %d", n)
	}
	if data, _ := os.ReadFile(secret); string(data) != "mgmt" {
		t.Fatalf("secret = %q", data)
	}
	if got := t3.StaleQuotaSources(8317, "mgmt"); len(got) != 0 {
		t.Fatalf("still stale: %v", got)
	}
}
