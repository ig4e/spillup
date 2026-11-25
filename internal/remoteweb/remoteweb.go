// Package remoteweb runs moonlight-web, the browser client for Sunshine hosts, next to spillup: it
// installs it, writes its config, keeps it running on 127.0.0.1, and pairs it with hosts.
package remoteweb

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/release"
	"github.com/darkyeg/spinup/internal/sysproc"
)

// Project is moonlight-web's GitHub project.
const Project = "MrCreativ3001/moonlight-web-stream"

// Prefix is where the dashboard serves moonlight-web; UserHeader names the user the dashboard signs it in as.
const (
	Prefix     = "/remote"
	UserHeader = "X-Spinup-User"
	User       = "spillup"
)

// Install is moonlight-web's folder: its executable, its web files, and its config.
type Install struct{ Dir string }

func (i Install) Exe() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(i.Dir, "web-server.exe")
	}
	return filepath.Join(i.Dir, "web-server")
}

func (i Install) Config() string { return filepath.Join(i.Dir, "server", "config.json") }

// Version is the moonlight-web release installed here, or "" when unknown.
func (i Install) Version() string {
	data, _ := os.ReadFile(filepath.Join(i.Dir, "version.txt"))
	return strings.TrimSpace(string(data))
}

func (i Install) Installed() bool {
	_, err := os.Stat(i.Exe())
	return err == nil
}

func asset() (string, error) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		return "moonlight-web-x86_64-unknown-linux-gnu.tar.gz", nil
	case "linux/arm64":
		return "moonlight-web-aarch64-unknown-linux-gnu.tar.gz", nil
	case "windows/amd64":
		return "moonlight-web-x86_64-pc-windows-gnu.zip", nil
	case "darwin/amd64":
		return "moonlight-web-x86_64-apple-darwin.tar.gz", nil
	case "darwin/arm64":
		return "moonlight-web-aarch64-apple-darwin.tar.gz", nil
	}
	return "", fmt.Errorf("moonlight-web has no build for %s/%s", runtime.GOOS, runtime.GOARCH)
}

// Download installs the latest moonlight-web, checked against GitHub's digest, and keeps its data.
func (i Install) Download(ctx context.Context) (string, error) {
	name, err := asset()
	if err != nil {
		return "", err
	}
	rel, err := release.Latest(ctx, Project)
	if err != nil {
		return "", err
	}
	data, err := rel.Fetch(ctx, name)
	if err != nil {
		return "", err
	}
	files, err := unpack(name, data)
	if err != nil {
		return "", err
	}
	for rel, content := range files {
		path := filepath.Join(i.Dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		mode := os.FileMode(0o644)
		if filepath.Base(path) == filepath.Base(i.Exe()) {
			mode = 0o755
		}
		if err := os.WriteFile(path+".new", content, mode); err != nil {
			return "", err
		}
		if err := os.Rename(path+".new", path); err != nil {
			return "", err
		}
	}
	return rel.Version, os.WriteFile(filepath.Join(i.Dir, "version.txt"), []byte(rel.Version+"\n"), 0o644)
}

// unpack returns the archive's files under its package/ folder, by their path inside it.
func unpack(name string, data []byte) (map[string][]byte, error) {
	out := map[string][]byte{}
	remaining := int64(256 << 20)
	add := func(name string, r io.Reader) error {
		rel, ok := strings.CutPrefix(filepath.ToSlash(name), "package/")
		if !ok || rel == "" {
			return nil
		}
		if rel == "." || !filepath.IsLocal(filepath.FromSlash(rel)) || path.Clean(rel) != rel || strings.ContainsAny(rel, `\:`) {
			return fmt.Errorf("invalid moonlight-web archive path %q", rel)
		}
		if _, exists := out[rel]; exists {
			return fmt.Errorf("duplicate moonlight-web archive path %q", rel)
		}
		content, err := io.ReadAll(io.LimitReader(r, remaining+1))
		if int64(len(content)) > remaining {
			return errors.New("moonlight-web archive exceeds its expanded size limit")
		}
		remaining -= int64(len(content))
		out[rel] = content
		return err
	}
	if strings.HasSuffix(name, ".zip") {
		z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		for _, f := range z.File {
			if !f.Mode().IsRegular() {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			err = add(f.Name, rc)
			rc.Close()
			if err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	limited := &io.LimitedReader{R: gz, N: (256 << 20) + 1}
	tr := tar.NewReader(limited)
	for {
		h, err := tr.Next()
		if limited.N == 0 {
			return nil, errors.New("moonlight-web archive exceeds its expanded size limit")
		}
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg {
			if err := add(h.Name, tr); err != nil {
				return nil, err
			}
		}
	}
}

// WriteConfig has moonlight-web generate its config, then answers only on 127.0.0.1 under the
// dashboard's prefix, and signs in whoever the dashboard names in UserHeader.
func (i Install) WriteConfig(port int) error {
	if err := os.MkdirAll(filepath.Dir(i.Config()), 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(i.Config()); err != nil {
		cmd := exec.Command(i.Exe(), "-c", i.Config(), "config", "generate")
		cmd.Dir = i.Dir
		sysproc.Hide(cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("moonlight-web config generate: %v: %s", err, strings.TrimSpace(string(out)))
		}
	}
	data, err := os.ReadFile(i.Config())
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	web, _ := cfg["web_server"].(map[string]any)
	if web == nil {
		web = map[string]any{}
	}
	web["bind_address"] = fmt.Sprintf("127.0.0.1:%d", port)
	web["url_path_prefix"] = Prefix
	web["forwarded_header"] = map[string]any{"username_header": UserHeader, "auto_create_missing_user": true}
	cfg["web_server"] = web
	out, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(i.Config(), out, 0o600)
}

// Keep runs moonlight-web until ctx ends, starting it again whenever it stops.
func (i Install) Keep(ctx context.Context, logger *log.Logger) {
	for ctx.Err() == nil {
		cmd := exec.CommandContext(ctx, i.Exe(), "-c", i.Config(), "run")
		cmd.Dir = i.Dir
		wait, err := sysproc.StartBound(cmd)
		if err != nil {
			logger.Printf("moonlight-web: %v", err)
		} else {
			if err := wait(); err != nil && ctx.Err() == nil {
				logger.Printf("moonlight-web stopped: %v", err)
			}
		}
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
		}
	}
}

// Client calls moonlight-web's API as the dashboard's user.
type Client struct{ Port int }

// Host is a Sunshine host moonlight-web knows.
type Host struct {
	ID     uint32 `json:"host_id"`
	Name   string `json:"name"`
	Paired string `json:"paired"`
	State  string `json:"server_state"`
}

func (c Client) url(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s/api%s", c.Port, Prefix, path)
}

var localClient = &http.Client{
	Transport:     &http.Transport{IdleConnTimeout: 30 * time.Second},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func (c Client) do(ctx context.Context, method, path string, in any) (*http.Response, error) {
	var body io.Reader
	if in != nil {
		data, _ := json.Marshal(in)
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.url(path), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set(UserHeader, User)
	req.Header.Set("Content-Type", "application/json")
	resp, err := localClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("moonlight-web isn't answering: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("moonlight-web: %s %s", resp.Status, strings.TrimSpace(string(data)))
	}
	return resp, nil
}

// Host reads the current state of the paired Sunshine host.
func (c Client) Host(ctx context.Context, id uint32) (Host, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resp, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/host?host_id=%d", id), nil)
	if err != nil {
		return Host{}, err
	}
	defer resp.Body.Close()
	var reply struct {
		Host Host `json:"host"`
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 128<<10)).Decode(&reply)
	if err == nil && reply.Host.ID != id {
		return Host{}, errors.New("unexpected Moonlight host")
	}
	return reply.Host, err
}

// Hosts reads the initial list and any fresh state streamed by Moonlight.
func (c Client) Hosts(ctx context.Context) ([]Host, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	resp, err := c.do(ctx, http.MethodGet, "/hosts", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Hosts []json.RawMessage `json:"hosts"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 4<<20))
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	hosts := make([]Host, 0, len(out.Hosts))
	for _, raw := range out.Hosts {
		var h Host
		if json.Unmarshal(raw, &h) == nil {
			hosts = append(hosts, h)
		}
	}
	index := make(map[uint32]int, len(hosts))
	for i, h := range hosts {
		index[h.ID] = i
	}
	for {
		var live Host
		err := decoder.Decode(&live)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			return nil, err
		}
		if i, ok := index[live.ID]; ok {
			hosts[i] = live
		}
	}
	return hosts, nil
}

// AddHost adds a Sunshine host by address and returns it.
func (c Client) AddHost(ctx context.Context, address string) (Host, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	resp, err := c.do(ctx, http.MethodPost, "/host", map[string]any{"address": address})
	if err != nil {
		return Host{}, err
	}
	defer resp.Body.Close()
	var out struct {
		Host Host `json:"host"`
	}
	return out.Host, json.NewDecoder(resp.Body).Decode(&out)
}

// Pair starts pairing with a host: it returns the PIN the host must confirm, and a wait that reports
// whether the host accepted it.
func (c Client) Pair(ctx context.Context, hostID uint32) (pin string, wait func() error, err error) {
	resp, err := c.do(ctx, http.MethodPost, "/pair", map[string]any{"host_id": hostID})
	if err != nil {
		return "", nil, err
	}
	lines := bufio.NewScanner(resp.Body)
	if !lines.Scan() {
		resp.Body.Close()
		return "", nil, errors.New("moonlight-web gave no pairing PIN")
	}
	var first struct {
		Pin *string `json:"Pin"`
	}
	if json.Unmarshal(lines.Bytes(), &first) != nil || first.Pin == nil {
		resp.Body.Close()
		return "", nil, fmt.Errorf("moonlight-web couldn't start pairing: %s", lines.Text())
	}
	wait = func() error {
		defer resp.Body.Close()
		if !lines.Scan() {
			return errors.New("moonlight-web stopped before the host answered")
		}
		if strings.Contains(lines.Text(), "Paired") {
			return nil
		}
		return errors.New("the host refused to pair")
	}
	return *first.Pin, wait, nil
}
