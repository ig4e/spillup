// Package sunshine drives Sunshine, the remote desktop host on each machine: its login, and confirming
// the PIN of a client that asks to pair.
package sunshine

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// StreamPort is where Moonlight clients reach a host; WebPort is Sunshine's own web API.
	StreamPort = 47989
	WebPort    = 47990
)

// Exe finds Sunshine, or "" when it isn't installed.
func Exe() string {
	if runtime.GOOS == "windows" {
		for _, dir := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramW6432")} {
			if p := filepath.Join(dir, "Sunshine", "sunshine.exe"); dir != "" && exists(p) {
				return p
			}
		}
	}
	if p, err := exec.LookPath("sunshine"); err == nil {
		return p
	}
	return ""
}

// MoonlightExe finds the Moonlight app, or "" when it isn't installed.
func MoonlightExe() string {
	if runtime.GOOS == "windows" {
		for _, dir := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramW6432"), os.Getenv("LOCALAPPDATA")} {
			if p := filepath.Join(dir, "Moonlight Game Streaming", "Moonlight.exe"); dir != "" && exists(p) {
				return p
			}
		}
	}
	for _, name := range []string{"moonlight", "moonlight-qt"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

func exists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// Running reports whether a Sunshine host answers on this machine.
func Running() bool {
	c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprint(StreamPort)), time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// Login is the user Sunshine's web API takes; spillup makes it, and nobody types it.
type Login struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

type pending struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

var client = &http.Client{
	Timeout:       10 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	Transport:     &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, // Sunshine's own self-signed certificate, on 127.0.0.1
}

// ErrNoPairing means no client is waiting to pair with this host.
var ErrNoPairing = errors.New("no client is waiting to pair with this machine's Sunshine")

// ConfirmPIN accepts the client waiting to pair from fromIP, or the only one waiting when fromIP is
// empty or matches none, with the PIN it showed. It waits a few seconds for the request to arrive.
func ConfirmPIN(ctx context.Context, login Login, pin, name, fromIP string) error {
	var waiting []pending
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(500 * time.Millisecond) {
		var out struct {
			Pairings []pending `json:"pairings"`
		}
		if err := call(ctx, login, http.MethodGet, "/api/pin", nil, &out); err != nil {
			return err
		}
		waiting = out.Pairings
		if len(waiting) > 0 || time.Now().After(deadline) {
			break
		}
	}
	target, err := choose(waiting, fromIP)
	if err != nil {
		return err
	}
	var result struct {
		Status bool `json:"status"`
	}
	body := map[string]string{"pairing_id": target.ID, "pin": pin, "name": name}
	if err := call(ctx, login, http.MethodPost, "/api/pin", body, &result); err != nil {
		return err
	}
	if !result.Status {
		return errors.New("Sunshine refused the PIN")
	}
	return nil
}

// choose picks the pairing from fromIP, or the only one waiting.
func choose(waiting []pending, fromIP string) (pending, error) {
	for _, p := range waiting {
		if fromIP != "" && normalize(p.Address) == normalize(fromIP) {
			return p, nil
		}
	}
	if len(waiting) == 1 {
		return waiting[0], nil
	}
	if len(waiting) == 0 {
		return pending{}, ErrNoPairing
	}
	return pending{}, fmt.Errorf("%d clients are waiting to pair; pair one at a time", len(waiting))
}

func normalize(addr string) string {
	addr = strings.TrimPrefix(addr, "::ffff:")
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

func call(ctx context.Context, login Login, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		data, _ := json.Marshal(in)
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("https://127.0.0.1:%d%s", WebPort, path), body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(login.User, login.Password)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Sunshine isn't answering: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return errors.New("Sunshine refused spillup's login: set remote desktop up again")
	case resp.StatusCode/100 != 2:
		return fmt.Errorf("Sunshine: %s %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}
