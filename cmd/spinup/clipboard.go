package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/remoteweb"
	"github.com/darkyeg/spinup/internal/sunshine"
	"github.com/darkyeg/spinup/internal/sysproc"
	"github.com/darkyeg/spinup/internal/tailnet"
	"github.com/darkyeg/spinup/internal/webui"
)

const clipboardLimit = 64 << 10

func (cliBackend) RemoteClipboard(ctx context.Context, id uint32) (webui.ClipboardText, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var out webui.ClipboardText
	var name string
	for machine, known := range loadRemoteHosts() {
		if known == id {
			name = machine
			break
		}
	}
	if name == "" {
		return out, errors.New("select a machine paired by Spillup")
	}
	h, err := (remoteweb.Client{Port: remotePort()}).Host(ctx, id)
	if err != nil {
		return out, err
	}
	if h.Paired != "Paired" || h.State != "Busy" {
		return out, errors.New("start a stream to this paired machine before copying")
	}
	cfg, err := config.Load()
	if err != nil {
		return out, err
	}
	s, err := config.LoadSecrets(cfg)
	if err != nil {
		return out, err
	}
	ts, err := (tailnet.CLI{Bin: cfg.Tailscale}).Status(ctx)
	if err != nil {
		return out, err
	}
	base, err := verifiedFleetPeer(ctx, ts, name, cfg.Port, s.APIKey)
	if err != nil {
		return out, errors.New("the paired machine could not verify its Spillup identity")
	}
	err = (api.Client{HTTP: directMachineClient, APIKey: s.APIKey, MaxResponseBytes: 6*clipboardLimit + 1024}).Post(ctx, base+webui.BridgePrefix+"/api/host/clipboard", struct{}{}, &out)
	if err != nil {
		return webui.ClipboardText{}, err
	}
	if len(out.Text) > clipboardLimit {
		return webui.ClipboardText{}, errors.New("clipboard text exceeds 64 KiB")
	}
	return out, nil
}

var clipboardRead sync.Mutex

func (cliBackend) HostClipboard(ctx context.Context, addr string) (webui.ClipboardText, error) {
	ctx, cancel := context.WithTimeout(ctx, 16*time.Second)
	defer cancel()
	var out webui.ClipboardText
	cfg, err := config.Load()
	if err != nil || !cfg.DeviceAccess {
		return out, errors.New("device access is off on the host")
	}
	st, err := (tailnet.CLI{Bin: cfg.Tailscale}).Status(ctx)
	if err != nil {
		return out, err
	}
	ip, _, _ := net.SplitHostPort(addr)
	peer, ok := st.PeerByIP(ip)
	if !ok || !st.Running || !st.SameOwner(peer) || !peer.Online || peer.Expired {
		return out, errors.New("clipboard access requires a device on your Tailscale account")
	}
	data, err := os.ReadFile(sunshineLoginPath())
	if err != nil {
		return out, errors.New("Sunshine is not configured by Spillup")
	}
	var login sunshine.Login
	if json.Unmarshal(data, &login) != nil || login.Password == "" {
		return out, errors.New("Sunshine login is unavailable")
	}
	if err := sunshine.ClipboardPeer(ctx, login, peer.Name); err != nil {
		return out, err
	}
	if !clipboardRead.TryLock() {
		return out, errors.New("a host clipboard read is already running")
	}
	defer clipboardRead.Unlock()
	value, err := desktopClipboard(ctx)
	return webui.ClipboardText{Text: value}, err
}

func readClipboard(ctx context.Context) (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-STA", "-Command", `$ErrorActionPreference='Stop';[Console]::OutputEncoding=New-Object Text.UTF8Encoding($false);[Console]::Write((Get-Clipboard -Raw -Format Text))`)
	case "darwin":
		cmd = exec.CommandContext(ctx, "/usr/bin/pbpaste", "-Prefer", "txt")
	default:
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			cmd = exec.CommandContext(ctx, "wl-paste", "--no-newline", "--type", "text")
		} else {
			cmd = exec.CommandContext(ctx, "xclip", "-selection", "clipboard", "-out", "-target", "UTF8_STRING")
		}
	}
	sysproc.Hide(cmd)
	cmd.WaitDelay = time.Second
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err = cmd.Start(); err != nil {
		return "", errors.New("the host clipboard helper could not start")
	}
	data, err := io.ReadAll(io.LimitReader(pipe, clipboardLimit+1))
	if len(data) > clipboardLimit {
		_ = cmd.Process.Kill()
	}
	_ = pipe.Close()
	waitErr := cmd.Wait()
	if len(data) > clipboardLimit {
		return "", errors.New("clipboard text exceeds 64 KiB")
	}
	if err != nil || waitErr != nil {
		return "", errors.New("no text clipboard is available in the host desktop session")
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return "", errors.New("the host clipboard does not contain plain text")
	}
	return string(data), nil
}
