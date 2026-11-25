// Package t3code drives T3 Code through its own command line: its background service, its pairing codes
// and the links between machines that let agents on one hand work to another.
package t3code

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/atomicfile"
	"github.com/darkyeg/spinup/internal/filelock"
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

// Find locates `t3`: on PATH, or the launcher the desktop app writes into T3's home, which isn't on PATH.
func Find() (T3, error) {
	home, err := Home()
	if err != nil {
		return T3{}, err
	}
	if bin, err := exec.LookPath("t3"); err == nil {
		return T3{Bin: bin, Home: home}, nil
	}
	for _, bin := range launchers(home) {
		if fi, err := os.Stat(bin); err == nil && !fi.IsDir() {
			return T3{Bin: bin, Home: home}, nil
		}
	}
	return T3{Home: home}, ErrNotInstalled
}

// Home is T3's home folder, normally ~/.t3.
func Home() (string, error) {
	if home := os.Getenv("T3CODE_HOME"); home != "" {
		return home, nil
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ".t3"), nil
}

func launchers(home string) []string {
	user := filepath.Dir(home)
	names := []string{"t3"}
	if runtime.GOOS == "windows" {
		names = []string{"t3.exe", "t3.cmd", "t3.bat"}
	}
	var out []string
	for _, dir := range []string{filepath.Join(home, "bin"), filepath.Join(user, ".local", "bin")} {
		for _, n := range names {
			out = append(out, filepath.Join(dir, n))
		}
	}
	return out
}

// AppPresent reports whether T3 Code has run on this machine, desktop app or CLI, from the data it keeps.
func AppPresent(home string) bool {
	_, err := os.Stat(filepath.Join(home, "userdata"))
	return err == nil
}

// InstallCLI installs the `t3` command with T3's own installer.
func InstallCLI(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
			"-Command", "irm https://t3.codes/install.ps1 | iex")
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", "curl -fsSL https://t3.codes/install.sh | sh")
	}
	sysproc.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("T3's installer: %w: %s", err, strings.TrimSpace(lastLine(string(out))))
	}
	return nil
}

func (t T3) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, t.Bin, args...)
	cmd.Env = append(os.Environ(), "T3CODE_HOME="+t.Home)
	switch args[0] {
	case "auth", "environment", "status", "up":
		// T3 otherwise tries to reserve a new port even for commands that only open its local database.
		cmd.Env = append(cmd.Env, "T3CODE_PORT="+strconv.Itoa(t.Port()))
	}
	sysproc.Hide(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		why := reason(stderr.String())
		if why == "" {
			why = reason(string(out))
		}
		if why == "" {
			why = "no reason given"
		}
		return out, fmt.Errorf("t3 %s: %s (%w)", strings.Join(args[:min(2, len(args))], " "), why, err)
	}
	return out, nil
}

// reason is the line of T3's output that says what went wrong: its error line, not the stack under it.
func reason(out string) string {
	var last string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "at ") {
			continue
		}
		if i := strings.Index(line, "Error: "); i >= 0 {
			return strings.TrimSpace(line[i+len("Error: "):])
		}
		last = line
	}
	return last
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
		return Pairing{}, errors.New("t3 gave no pairing code")
	}
	return p, nil
}

// Linked is `t3 environment list --json` as T3 prints it, for matching addresses in.
func (t T3) Linked(ctx context.Context) (string, error) {
	out, err := t.run(ctx, "environment", "list", "--json")
	if err != nil {
		return "", err
	}
	var links []environmentLink
	if err := json.Unmarshal(out, &links); err != nil {
		return "", fmt.Errorf("t3 environment list returned invalid JSON: %w", err)
	}
	return string(out), nil
}

type environmentLink struct {
	EnvironmentID string   `json:"environmentId"`
	URL           string   `json:"url"`
	URLs          []string `json:"urls"`
	Access        string   `json:"access"`
	Status        string   `json:"status"`
}

// LinksTo matches T3's environment ID or recorded addresses. An expired full-access link can be paired again.
func LinksTo(linked, environmentID string, hosts ...string) bool {
	var links []environmentLink
	if json.Unmarshal([]byte(linked), &links) != nil {
		return false
	}
	for _, link := range links {
		if link.Status == "expired" && link.Access == LinkAccess {
			continue
		}
		if environmentID != "" && link.EnvironmentID != "" {
			if link.EnvironmentID == environmentID {
				return true
			}
			continue
		}
		for _, raw := range append(link.URLs, link.URL) {
			endpoint, err := url.Parse(raw)
			if err != nil || endpoint.Hostname() == "" {
				continue
			}
			for _, host := range hosts {
				if strings.EqualFold(strings.TrimSuffix(endpoint.Hostname(), "."), strings.TrimSuffix(host, ".")) {
					return true
				}
			}
		}
	}
	return false
}

func (t T3) Link(ctx context.Context, address, code string) error {
	endpoint, err := url.Parse(address)
	if err != nil || endpoint.Hostname() == "" || code == "" {
		return errors.New("T3 linking needs an address and pairing code")
	}
	lock, err := filelock.Acquire(ctx, filepath.Join(t.Home, ".spillup-link.lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	listed, err := t.Linked(ctx)
	if err != nil || LinksTo(listed, "", endpoint.Hostname()) {
		return err
	}
	_, err = t.run(ctx, "environment", "link", address, "--pairing-code", code, "--access", LinkAccess)
	if err != nil && code != "" && strings.Contains(err.Error(), code) {
		return errors.New(strings.ReplaceAll(err.Error(), code, "[redacted]"))
	}
	return err
}

// EnvironmentID identifies this T3 environment independently of its network addresses.
func (t T3) EnvironmentID() string {
	data, _ := os.ReadFile(filepath.Join(t.Home, "userdata", "environment-id"))
	return strings.TrimSpace(string(data))
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

// ServiceAtBoot reports whether T3's background service is installed to start at boot.
func (t T3) ServiceAtBoot(ctx context.Context) bool {
	out, err := t.run(ctx, "status", "--json")
	if err != nil {
		return false
	}
	var st struct {
		Service struct {
			Installed bool `json:"installed"`
		} `json:"service"`
	}
	return json.Unmarshal(out, &st) == nil && st.Service.Installed
}

// LinkingOn reports whether T3's linked environments are on.
func (t T3) LinkingOn() bool {
	data, err := os.ReadFile(filepath.Join(t.Home, "userdata", "settings.json"))
	if err != nil {
		return false
	}
	var s struct {
		Linked bool `json:"linkedEnvironmentsEnabled"`
	}
	return json.Unmarshal(data, &s) == nil && s.Linked
}

// LinkCount is how many environments a `t3 environment list --json` names.
func LinkCount(linked string) int {
	var list []json.RawMessage
	if json.Unmarshal([]byte(linked), &list) != nil {
		return 0
	}
	return len(list)
}

func (t T3) desktopSettings() string {
	return filepath.Join(t.Home, "userdata", "desktop-settings.json")
}

// NetworkAccess reports whether the desktop app answers other machines, not only this one; true where
// there is no desktop app, since its background service follows `t3 up --lan` instead.
func (t T3) NetworkAccess() bool {
	data, err := os.ReadFile(t.desktopSettings())
	if err != nil {
		return true
	}
	var s struct {
		Mode string `json:"serverExposureMode"`
	}
	return json.Unmarshal(data, &s) == nil && s.Mode == "network-accessible"
}

// AwaitsRestart reports whether Network access is on but the running T3 Code still answers only itself,
// which it does until it restarts: it refuses this machine's own tailnet address while it answers locally.
func (t T3) AwaitsRestart(tailnetIP string) bool {
	if tailnetIP == "" || !t.NetworkAccess() {
		return false
	}
	port := strconv.Itoa(t.Port())
	if !answers(net.JoinHostPort("127.0.0.1", port)) {
		return false
	}
	return !answers(net.JoinHostPort(tailnetIP, port))
}

func answers(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// EnableNetworkAccess turns on the desktop app's Network access, which it reads when it starts, keeping
// every other setting; it reports whether it changed anything, in which case T3 Code needs a restart.
func (t T3) EnableNetworkAccess() (changed bool, err error) {
	path := t.desktopSettings()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	settings := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	if string(settings["serverExposureMode"]) == `"network-accessible"` {
		return false, nil
	}
	settings["serverExposureMode"] = json.RawMessage(`"network-accessible"`)
	out, _ := json.MarshalIndent(settings, "", "  ")
	return true, atomicfile.Write(path, append(out, '\n'), 0o600)
}
