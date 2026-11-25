package t3code

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/atomicfile"
)

// StaleQuotaSources are T3 Code's usage-limit sources that read the local router or a trusted Tailnet endpoint, at port, with a
// key other than its management key, which CLIProxyAPI refuses with a 401.
func (t T3) StaleQuotaSources(port int, key string, hosts ...string) []string {
	var stale []string
	for _, id := range t.quotaSources(port, hosts) {
		got, err := os.ReadFile(t.quotaSecret(id))
		if err != nil || string(bytes.TrimSpace(got)) != key {
			stale = append(stale, id)
		}
	}
	return stale
}

// FixQuotaKeys gives each stale usage-limit source the shared management key; it reports how many it fixed.
func (t T3) FixQuotaKeys(port int, key string, hosts ...string) (int, error) {
	if key == "" {
		return 0, nil
	}
	stale := t.StaleQuotaSources(port, key, hosts...)
	fixed := 0
	var writeErr error
	for _, id := range stale {
		if writeErr = atomicfile.Write(t.quotaSecret(id), []byte(key), 0o600); writeErr != nil {
			break
		}
		fixed++
	}
	if fixed > 0 {
		// T3 watches its settings file and caches the resolved secrets until it reloads.
		now := time.Now()
		if err := os.Chtimes(t.settingsPath(), now, now); err != nil {
			return fixed, errors.Join(writeErr, fmt.Errorf("notify T3 Code of updated quota keys: %w", err))
		}
	}
	return fixed, writeErr
}

func (t T3) settingsPath() string {
	return filepath.Join(t.Home, "userdata", "settings.json")
}

func (t T3) quotaSources(port int, hosts []string) []string {
	data, err := os.ReadFile(t.settingsPath())
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
		if src.Kind == "cliproxy" && matchesQuotaEndpoint(src.URL, port, hosts) {
			ids = append(ids, id)
		}
	}
	return ids
}

func matchesQuotaEndpoint(raw string, port int, hosts []string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Port() != strconv.Itoa(port) {
		return false
	}
	host := strings.TrimSuffix(u.Hostname(), ".")
	for _, trusted := range hosts {
		if trusted != "" && strings.EqualFold(host, strings.TrimSuffix(trusted, ".")) {
			return true
		}
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (t T3) quotaSecret(id string) string {
	name := "usage-limit-source-" + base64.RawURLEncoding.EncodeToString([]byte(id)) + ".bin"
	return filepath.Join(t.Home, "userdata", "secrets", name)
}
