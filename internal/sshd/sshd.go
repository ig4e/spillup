// Package sshd makes every machine reachable with plain `ssh <machine>` over the tailnet: an SSH server
// (Tailscale SSH, or OpenSSH where Tailscale has none), this machine's key, your machines' keys let in,
// and a Host entry per machine in ~/.ssh/config.
package sshd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/darkyeg/spinup/internal/atomicfile"
	"github.com/darkyeg/spinup/internal/sysproc"
)

// Server names the SSH server a machine runs.
type Server string

const (
	// Tailscale SSH signs you in with your Tailscale account; it runs on Linux and macOS.
	Tailscale Server = "tailscale"
	// OpenSSH is Windows' own server, let in through the firewall from the tailnet only.
	OpenSSH Server = "openssh"
)

// Kind is the SSH server spillup runs on this OS.
func Kind() Server {
	if runtime.GOOS == "windows" {
		return OpenSSH
	}
	return Tailscale
}

// Host is another machine as ~/.ssh/config names it.
type Host struct {
	Name, Address, User string
}

// User is the account name to sign in to this machine with.
func User() string {
	u, err := user.Current()
	if err != nil {
		return os.Getenv("USER")
	}
	name := u.Username
	if i := strings.LastIndex(name, `\`); i >= 0 {
		name = name[i+1:]
	}
	return name
}

func sshDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ssh")
}

// PublicKey is this machine's SSH public key, made once with ssh-keygen when it has none.
func PublicKey(ctx context.Context) (string, error) {
	path := filepath.Join(sshDir(), "id_ed25519")
	if _, err := os.Stat(path + ".pub"); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(sshDir(), 0o700); err != nil {
			return "", err
		}
		host, _ := os.Hostname()
		cmd := exec.CommandContext(ctx, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", User()+"@"+host+" (spillup)", "-f", path)
		sysproc.Hide(cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("ssh-keygen: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	data, err := os.ReadFile(path + ".pub")
	return strings.TrimSpace(string(data)), err
}

// Authorize lets in exactly these keys, your other machines', keeping every key you added yourself.
func Authorize(keys []string) error {
	keys = slices.Clone(keys)
	slices.Sort(keys)
	return writeBlock(filepath.Join(sshDir(), "authorized_keys"), strings.Join(slices.Compact(keys), "\n"))
}

// WriteConfig names each machine in ~/.ssh/config, so `ssh <machine>` reaches it on the tailnet as the right user.
func WriteConfig(hosts []Host) error {
	return writeBlock(filepath.Join(sshDir(), "config"), configEntries(hosts))
}

// configEntries trusts each machine's host key on first contact: the tailnet already authenticates the peer,
// and without it scripts and agents (BatchMode) can never reach a machine nobody has opened by hand.
func configEntries(hosts []Host) string {
	var b strings.Builder
	for _, h := range hosts {
		fmt.Fprintf(&b, "Host %s\n  HostName %s\n", h.Name, h.Address)
		if h.User != "" {
			fmt.Fprintf(&b, "  User %s\n", h.User)
		}
		b.WriteString("  StrictHostKeyChecking accept-new\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

const (
	blockBegin = "# >>> spillup: your machines (managed, edits here are replaced)"
	blockEnd   = "# <<< spillup"
)

// writeBlock replaces spillup's block in a file, or adds it, keeping everything else.
func writeBlock(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	next := replaceBlock(string(old), body)
	if next == string(old) {
		return nil
	}
	return atomicfile.Write(path, []byte(next), 0o600)
}

func replaceBlock(text, body string) string {
	block := ""
	if body != "" {
		block = blockBegin + "\n" + body + "\n" + blockEnd + "\n"
	}
	start := strings.Index(text, blockBegin)
	end := strings.Index(text, blockEnd)
	if start >= 0 && end > start {
		rest := text[end+len(blockEnd):]
		rest = strings.TrimPrefix(strings.TrimPrefix(rest, "\r"), "\n")
		return text[:start] + block + rest
	}
	if block == "" {
		return text
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" {
		text += "\n"
	}
	return text + block
}
