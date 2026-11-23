// Package proxy runs CLIProxyAPI: its config, its process, its release downloads and its
// management API.
package proxy

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/darkyeg/spinup"
	"github.com/darkyeg/spinup/internal/atomicfile"
	"github.com/darkyeg/spinup/internal/config"
)

func render(template, host string, port int, authDir string, s config.Secrets) string {
	return strings.NewReplacer(
		"{{HOST}}", host,
		"{{AUTH_DIR}}", filepath.ToSlash(authDir), // YAML needs no escapes for forward slashes
		"{{PORT}}", strconv.Itoa(port),
		"{{API_KEY}}", s.APIKey,
		"{{SECRET_KEY}}", s.ManagementPassword,
	).Replace(template)
}

// WriteConfig keeps CLIProxyAPI on 127.0.0.1: spinup is the only way in.
func WriteConfig(c config.Config, s config.Secrets) error {
	text := render(spinup.ProxyConfigTemplate, "127.0.0.1", c.ProxyPort, c.AuthDir, s)
	return atomicfile.Write(c.ProxyConfig(), []byte(text), 0o600)
}

// WriteOwnConfig is the CLIProxyAPI for this machine's own logins. Its management key is the API key,
// which every machine has, since it answers only on 127.0.0.1 and holds only this machine's logins.
func WriteOwnConfig(c config.Config, s config.Secrets) error {
	own := config.Secrets{APIKey: s.APIKey, ManagementPassword: s.APIKey}
	text := render(spinup.ProxyConfigTemplate, "127.0.0.1", c.OwnProxyPort, c.OwnAuthDir, own)
	if err := os.MkdirAll(c.OwnAuthDir, 0o700); err != nil {
		return err
	}
	return atomicfile.Write(c.OwnProxyConfig(), []byte(text), 0o600)
}
