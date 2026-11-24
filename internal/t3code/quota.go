package t3code

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/darkyeg/spinup/internal/atomicfile"
)

// StaleQuotaSources are T3 Code's usage-limit sources that read this machine's CLIProxyAPI, at port, with a
// key other than its management key, which CLIProxyAPI refuses with a 401.
func (t T3) StaleQuotaSources(port int, key string) []string {
	var stale []string
	for _, id := range t.localQuotaSources(port) {
		got, err := os.ReadFile(t.quotaSecret(id))
		if err != nil || string(bytes.TrimSpace(got)) != key {
			stale = append(stale, id)
		}
	}
	return stale
}

// FixQuotaKeys gives each stale usage-limit source this machine's management key; it reports how many it fixed.
func (t T3) FixQuotaKeys(port int, key string) (int, error) {
	if key == "" {
		return 0, nil
	}
	stale := t.StaleQuotaSources(port, key)
	for _, id := range stale {
		if err := atomicfile.Write(t.quotaSecret(id), []byte(key), 0o600); err != nil {
			return 0, err
		}
	}
	return len(stale), nil
}

func (t T3) localQuotaSources(port int) []string {
	data, err := os.ReadFile(filepath.Join(t.Home, "userdata", "settings.json"))
	if err != nil {
		return nil
	}
	var s struct {
		Sources map[string]struct {
			Kind string `json:"kind"`
			URL  string `json:"url"`
		} `json:"usageLimitSources"`
	}
	if json.Unmarshal(data, &s) != nil {
		return nil
	}
	var ids []string
	for id, src := range s.Sources {
		if src.Kind == "cliproxy" && isLocalPort(src.URL, port) {
			ids = append(ids, id)
		}
	}
	return ids
}

func isLocalPort(raw string, port int) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Port() != strconv.Itoa(port) {
		return false
	}
	if u.Hostname() == "localhost" {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return ip != nil && ip.IsLoopback()
}

func (t T3) quotaSecret(id string) string {
	name := "usage-limit-source-" + base64.RawURLEncoding.EncodeToString([]byte(id)) + ".bin"
	return filepath.Join(t.Home, "userdata", "secrets", name)
}
