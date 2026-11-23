// Package legacyproxy finds a CLIProxyAPI someone ran before spillup, so setup can keep the API key their
// tools already use and, on the main machine, take its logins in.
package legacyproxy

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Found is an earlier CLIProxyAPI's config.
type Found struct {
	Config  string
	APIKeys []string
	AuthDir string
}

// Find looks in the places people put CLIProxyAPI, skipping spillup's own copy at skip.
func Find(home, skip string) (Found, bool) {
	for _, path := range candidates(home) {
		if same(path, skip) {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		f := Parse(string(data), home)
		f.Config = path
		if len(f.APIKeys) > 0 || f.AuthDir != "" {
			if f.AuthDir == "" {
				f.AuthDir = filepath.Join(home, ".cli-proxy-api")
			}
			return f, true
		}
	}
	return Found{}, false
}

func candidates(home string) []string {
	paths := []string{
		filepath.Join(home, ".config", "cliproxyapi", "config.yaml"),
		filepath.Join(home, ".cli-proxy-api", "config.yaml"),
		filepath.Join(home, "cliproxyapi", "config.yaml"),
		filepath.Join(home, "CLIProxyAPI", "config.yaml"),
	}
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			paths = append(paths, filepath.Join(appData, "CLIProxyAPI", "config.yaml"))
		}
	}
	return paths
}

func same(a, b string) bool {
	if b == "" {
		return false
	}
	ca, _ := filepath.Abs(a)
	cb, _ := filepath.Abs(b)
	return strings.EqualFold(filepath.Clean(ca), filepath.Clean(cb))
}

// Parse reads the API keys and the auth folder from a CLIProxyAPI config.yaml; it needs no YAML library
// because those keys are a plain list and a plain string.
func Parse(text, home string) Found {
	var f Found
	inKeys, keysIndent := false, 0
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), " \t\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if inKeys {
			if strings.HasPrefix(trimmed, "- ") && indent >= keysIndent {
				if key := unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))); key != "" {
					f.APIKeys = append(f.APIKeys, key)
				}
				continue
			}
			inKeys = false
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "api-keys":
			inKeys, keysIndent = true, indent
		case "auth-dir":
			f.AuthDir = expand(unquote(strings.TrimSpace(stripComment(value))), home)
		}
	}
	return f
}

func stripComment(v string) string {
	if i := strings.Index(v, " #"); i >= 0 {
		return v[:i]
	}
	return v
}

func unquote(v string) string {
	v = strings.TrimSpace(stripComment(v))
	if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
		return v[1 : len(v)-1]
	}
	return v
}

func expand(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		return filepath.Join(home, path[2:])
	}
	return path
}

// Logins are the login files in an auth folder: CLIProxyAPI's JSON files, without its locks and database.
func Logins(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	return names
}
