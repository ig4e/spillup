// Package t3code drives T3 Code through its own command line: its background service, its pairing codes
// and the links between machines that let agents on one hand work to another.
package t3code

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/atomicfile"
	"github.com/darkyeg/spinup/internal/sysproc"
)

// DefaultPort is where T3 Code's server listens unless it says otherwise.
const DefaultPort = 3773

// LinkAccess is what agents on a linked machine may do here: the user chose full access for their own machines.
const LinkAccess = "full-access"

// ErrNotInstalled means this machine has no `t3` command.
var ErrNotInstalled = errors.New("T3 Code's `t3` command isn't installed here")

// T3 is T3 Code on this machine.
type T3 struct {
	Bin  string
	Home string // T3's home folder, normally ~/.t3
}

// Find locates `t3` and T3's home folder.
func Find() (T3, error) {
	bin, err := exec.LookPath("t3")
	if err != nil {
		return T3{}, ErrNotInstalled
	}
	home := os.Getenv("T3CODE_HOME")
	if home == "" {
		dir, err := os.UserHomeDir()
		if err != nil {
			return T3{}, err
		}
		home = filepath.Join(dir, ".t3")
	}
	return T3{Bin: bin, Home: home}, nil
}

func (t T3) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, t.Bin, args...)
	cmd.Stdin = nil
	sysproc.Hide(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("t3 %s: %w: %s", args[0], err, strings.TrimSpace(lastLine(stderr.String())))
	}
	return out, nil
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// Up installs T3's background service, started at boot, reachable on the LAN and the tailnet with a
// pairing code; it asks nothing and leaves T3 Connect as it is.
func (t T3) Up(ctx context.Context) error {
	_, err := t.run(ctx, "up", "--lan")
	return err
}

// Pairing is a one-time code another machine links with.
type Pairing struct {
	ID         string `json:"id"`
	Credential string `json:"credential"`
}

func (t T3) Pair(ctx context.Context, label string) (Pairing, error) {
	out, err := t.run(ctx, "auth", "pairing", "create", "--json", "--ttl", "10m", "--label", label)
	if err != nil {
		return Pairing{}, err
	}
	var p Pairing
	if err := json.Unmarshal(out, &p); err != nil || p.Credential == "" {
		return Pairing{}, fmt.Errorf("t3 gave no pairing code: %s", strings.TrimSpace(string(out)))
	}
	return p, nil
}

// Linked is `t3 environment list --json` as T3 prints it, for matching addresses in.
func (t T3) Linked(ctx context.Context) (string, error) {
	out, err := t.run(ctx, "environment", "list", "--json")
	return string(out), err
}

// LinksTo reports whether a linked environment answers at host (an IP or name).
func LinksTo(linked, host string) bool {
	return host != "" && strings.Contains(linked, "//"+host+":")
}

func (t T3) Link(ctx context.Context, url, code string) error {
	_, err := t.run(ctx, "environment", "link", url, "--pairing-code", code, "--access", LinkAccess)
	return err
}

// Port is where this machine's T3 server listens, from the runtime file it writes.
func (t T3) Port() int {
	data, err := os.ReadFile(filepath.Join(t.Home, "userdata", "server-runtime.json"))
	if err != nil {
		return DefaultPort
	}
	var rt struct {
		Port int `json:"port"`
	}
	if json.Unmarshal(data, &rt) != nil || rt.Port == 0 {
		return DefaultPort
	}
	return rt.Port
}

// EnableLinking turns on T3's Linked environments in its settings.json, which a running server picks up;
// every other setting is kept as it is.
func (t T3) EnableLinking() (changed bool, err error) {
	path := filepath.Join(t.Home, "userdata", "settings.json")
	settings := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return false, err
	default:
		if err := json.Unmarshal(data, &settings); err != nil {
			return false, fmt.Errorf("%s: %w", path, err)
		}
	}
	if string(settings["linkedEnvironmentsEnabled"]) == "true" {
		return false, nil
	}
	settings["linkedEnvironmentsEnabled"] = json.RawMessage("true")
	out, _ := json.MarshalIndent(settings, "", "  ")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	return true, atomicfile.Write(path, append(out, '\n'), 0o600)
}
