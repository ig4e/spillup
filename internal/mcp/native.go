package mcp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/atomicfile"
	"github.com/darkyeg/spinup/internal/host"
	"github.com/darkyeg/spinup/internal/shell"
	"github.com/darkyeg/spinup/internal/sysproc"
)

var (
	errNativeRead  = errors.New("could not inspect the agent's user configuration")
	errNativeWrite = errors.New("the agent could not apply the MCP change; check its version and retry")
	errCollision   = errors.New("this MCP alias is already configured or was edited in the agent; review it there before continuing")
)

const maxNativeJSON = 16 << 20

type nativeTool struct {
	path   string
	prefix []string
}

func findNative(target string) (nativeTool, bool) {
	if !slices.Contains(targets, target) {
		return nativeTool{}, false
	}
	name := target
	if host.Windows {
		name += ".exe"
	}
	cmd := shell.CommandContext(context.Background(), name)
	if cmd.Err == nil && filepath.IsAbs(cmd.Path) {
		return nativeTool{path: cmd.Path}, true
	}
	if !host.Windows {
		return nativeTool{}, false
	}

	// npm's Windows shims are batch files. Invoke their installed package entry
	// directly so URLs and paths never pass through cmd.exe.
	shim, err := exec.LookPath(target + ".cmd")
	if err != nil || !filepath.IsAbs(shim) {
		return nativeTool{}, false
	}
	pkg := "@openai/codex"
	if target == "claude" {
		pkg = "@anthropic-ai/claude-code"
	}
	dir := filepath.Join(filepath.Dir(shim), "node_modules", filepath.FromSlash(pkg))
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	var meta struct{ Bin map[string]string }
	if err != nil || json.Unmarshal(data, &meta) != nil {
		return nativeTool{}, false
	}
	entry := filepath.Clean(meta.Bin[target])
	if entry == "." || !filepath.IsLocal(entry) {
		return nativeTool{}, false
	}
	launcher := filepath.Join(dir, entry)
	info, err := os.Stat(launcher)
	if err != nil || !info.Mode().IsRegular() {
		return nativeTool{}, false
	}
	switch strings.ToLower(filepath.Ext(entry)) {
	case ".exe":
		return nativeTool{path: launcher}, true
	case ".js", ".cjs", ".mjs":
		node, err := exec.LookPath("node.exe")
		if err == nil && filepath.IsAbs(node) {
			return nativeTool{path: node, prefix: []string{launcher}}, true
		}
	}
	return nativeTool{}, false
}

// Node's Windows distribution installs npm.cmd/npx.cmd alongside a native
// node.exe and npm's bin directory. Run their fixed JS entry directly so MCP
// arguments keep their literal values. Other batch shims have no such layout.
func npmShellShim(shim string) (nativeTool, bool) {
	name := strings.ToLower(filepath.Base(shim))
	if !filepath.IsAbs(shim) || name != "npx.cmd" && name != "npm.cmd" {
		return nativeTool{}, false
	}
	dir := filepath.Dir(shim)
	script := filepath.Join(dir, "node_modules", "npm", "bin", strings.TrimSuffix(name, ".cmd")+"-cli.js")
	info, err := os.Stat(script)
	if err != nil || !info.Mode().IsRegular() {
		return nativeTool{}, false
	}
	node := filepath.Join(dir, "node.exe")
	if info, err := os.Stat(node); err != nil || !info.Mode().IsRegular() {
		node, err = exec.LookPath("node.exe")
		if err != nil || !filepath.IsAbs(node) {
			return nativeTool{}, false
		}
	}
	return nativeTool{path: node, prefix: []string{script}}, true
}

func AvailableTargets() []string {
	available := []string{}
	for _, target := range targets {
		if _, ok := findNative(target); ok {
			available = append(available, target)
		}
	}
	return available
}

func (tool nativeTool) command(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, tool.path, append(slices.Clone(tool.prefix), args...)...)
	cmd.Dir = dir
	cmd.WaitDelay = time.Second
	sysproc.Hide(cmd)
	return cmd
}

type nativeConfig struct {
	current json.RawMessage
	apply   func(context.Context, map[string]any) error
	close   func()
}

func registration(target string, d Definition, exe string) map[string]any {
	if d.Transport == "http" {
		entry := map[string]any{"url": d.URL}
		if target == "claude" {
			entry["type"] = "http"
		}
		return entry
	}
	entry := map[string]any{"command": exe, "args": []string{"mcp", "run", d.Name, target}}
	if target == "claude" {
		entry["type"] = "stdio"
		entry["env"] = map[string]string{}
	}
	return entry
}

// Fingerprint the complete alias entry, including unknown future fields. A
// changed env, tool policy, or header is a user edit even when the URL matches.
func registrationDigest(raw json.RawMessage) string {
	var actual map[string]any
	if json.Unmarshal(raw, &actual) != nil || actual == nil {
		return ""
	}
	data, err := json.Marshal(actual)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func expectedDigest(entry map[string]any) string {
	if entry == nil {
		return ""
	}
	raw, _ := json.Marshal(entry)
	return registrationDigest(raw)
}

func matchesRegistration(raw json.RawMessage, expected map[string]any) bool {
	return raw != nil && expected != nil && registrationDigest(raw) == expectedDigest(expected)
}

func NativeTarget(ctx context.Context, target string, next Definition, enable bool, previous *Definition) error {
	return nativeTarget(ctx, target, next, enable, previous, "", nil)
}

func nativeTarget(ctx context.Context, target string, next Definition, enable bool, previous *Definition, pending string, launcher func(string) string) error {
	if enable {
		if err := next.Validate(); err != nil {
			return err
		}
	} else if !namePattern.MatchString(next.Name) {
		return errors.New("invalid MCP server name")
	}
	if previous != nil && (previous.Name != next.Name || previous.Validate() != nil) {
		return errors.New("invalid previous MCP approval")
	}
	tool, ok := findNative(target)
	if !ok {
		if !enable && slices.Contains(targets, target) {
			path := nativeConfigPath(target)
			if filepath.IsAbs(path) {
				if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
					return nil
				}
				if target == "claude" {
					entry, err := readClaude(path, "spillup-"+next.Name)
					if err == nil && entry == nil {
						return nil
					}
				}
			}
		}
		return errors.New("the agent CLI is unavailable on this machine")
	}
	exe, err := os.Executable()
	if err != nil || !filepath.IsAbs(exe) {
		return errors.New("could not locate the spillup executable")
	}
	if launcher != nil && (enable && next.Transport == "stdio" || previous != nil && previous.Transport == "stdio") {
		exe = launcher(exe)
	}
	if !filepath.IsAbs(exe) {
		return errors.New("the MCP launcher must have an absolute path")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	// Both CLIs can discover workspace config from cwd. This operation is user-scoped.
	dir, err := os.MkdirTemp("", "spillup-mcp-")
	if err != nil {
		return errNativeRead
	}
	defer os.RemoveAll(dir)
	alias := "spillup-" + next.Name
	var config nativeConfig
	if target == "claude" {
		config, err = openClaude(tool, dir, alias)
	} else {
		config, err = openCodex(ctx, tool, dir, alias)
	}
	if err != nil {
		return err
	}
	if config.close != nil {
		defer config.close()
	}

	return configureAlias(ctx, config, target, next, enable, previous, exe, pending)
}

func registeredPath(pending string) string {
	return strings.TrimSuffix(pending, ".pending") + ".registered"
}

func configureAlias(ctx context.Context, config nativeConfig, target string, next Definition, enable bool, previous *Definition, exe, pending string) error {
	desired := registration(target, next, exe)
	var prior map[string]any
	if previous != nil {
		prior = registration(target, *previous, exe)
	}
	savePending := func() error {
		if pending == "" {
			return nil
		}
		if err := atomicfile.Write(pending, []byte(expectedDigest(desired)), 0600); err != nil {
			return errors.New("could not record the pending MCP change")
		}
		return nil
	}
	if config.current != nil {
		hash := registrationDigest(config.current)
		owned := matchesRegistration(config.current, prior)
		if pending != "" {
			// A registration can outlive a moved CLI, or commit just before
			// the approval is saved. A saved receipt overrides any guess
			// about which executable originally performed the registration.
			if previous != nil {
				data, err := os.ReadFile(registeredPath(pending))
				if err == nil {
					owned = hash != "" && string(data) == hash
				} else if !errors.Is(err, os.ErrNotExist) {
					return errNativeRead
				}
			}
			data, err := os.ReadFile(pending)
			if err == nil && hash != "" && string(data) == hash {
				owned = true
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return errNativeRead
			}
		}
		matches := matchesRegistration(config.current, desired)
		if enable && matches && (owned || previous == nil && next.Transport == "stdio") {
			return savePending()
		}
		if !owned {
			return errCollision
		}
	} else if !enable {
		return nil
	}

	if !enable {
		desired = nil
	}
	if enable {
		if err := savePending(); err != nil {
			return err
		}
	}
	return config.apply(ctx, desired)
}

func readClaude(path, alias string) (json.RawMessage, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errNativeRead
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxNativeJSON+1))
	var config struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err != nil || len(data) > maxNativeJSON || json.Unmarshal(data, &config) != nil || len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '{' {
		return nil, errNativeRead
	}
	return config.Servers[alias], nil
}

func nativeConfigPath(target string) string {
	if target == "codex" {
		return filepath.Join(host.CodexHome(), "config.toml")
	}
	path := filepath.Join(host.Home(), ".claude.json")
	if configDir := os.Getenv("CLAUDE_CONFIG_DIR"); configDir != "" {
		path = filepath.Join(configDir, ".claude.json")
	}
	return path
}

func openClaude(tool nativeTool, dir, alias string) (nativeConfig, error) {
	path := nativeConfigPath("claude")
	if !filepath.IsAbs(path) {
		return nativeConfig{}, errNativeRead
	}
	current, err := readClaude(path, alias)
	if err != nil {
		return nativeConfig{}, err
	}
	apply := func(ctx context.Context, entry map[string]any) error {
		// Claude does not expose conditional MCP edits. Re-read immediately
		// before removal, then rely on native add's existing-alias refusal.
		now, err := readClaude(path, alias)
		if err != nil {
			return err
		}
		if !bytes.Equal(now, current) {
			return errCollision
		}
		if current != nil {
			if tool.command(ctx, dir, "mcp", "remove", "--scope", "user", alias).Run() != nil {
				return errNativeWrite
			}
		}
		if entry != nil {
			var args []string
			if url, ok := entry["url"].(string); ok {
				args = []string{"mcp", "add", "--scope", "user", "--transport", "http", alias, url}
			} else {
				args = []string{"mcp", "add", "--scope", "user", "--transport", "stdio", alias, "--", entry["command"].(string)}
				args = append(args, entry["args"].([]string)...)
			}
			if tool.command(ctx, dir, args...).Run() != nil {
				return errNativeWrite
			}
		}
		result, err := readClaude(path, alias)
		if err != nil {
			return err
		}
		if entry == nil && result == nil || matchesRegistration(result, entry) {
			return nil
		}
		return errNativeWrite
	}
	return nativeConfig{current: current, apply: apply}, nil
}
