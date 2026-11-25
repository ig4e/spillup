package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/darkyeg/spinup/internal/atomicfile"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/webui"
)

const defaultUIPort = 8320

type uiCmd struct {
	Port      int  `help:"Serve a dashboard of its own on this port instead of opening the one the service hosts."`
	NoBrowser bool `help:"Print the link instead of opening a browser."`
}

func (uiCmd) Help() string {
	return `Opens spinup's dashboard in your browser, signed in: your machines, who holds the accounts and which
machine is the hub, your logins, skills, CLIProxyAPI's own dashboard and the doctor's checks.

The accounts service hosts it all the time at http://localhost:8320 (ui_port in config.json; 0 turns
it off), so this only opens it. Without a running service, it serves one itself until Ctrl-C. It
answers only on this machine, and only to a browser that opened a link from this command.`
}

func (c uiCmd) Run() error {
	token, err := uiToken()
	if err != nil {
		return err
	}
	port := c.Port
	if port == 0 {
		cfg, _ := config.Load()
		port = cfg.UIPort
		if port == 0 {
			port = defaultUIPort
		}
		if dashboardAnswers(port) {
			c.open(port, token)
			fmt.Println("The accounts service hosts it all the time; this link signs this browser in.")
			return nil
		}
	}
	l, err := net.Listen("tcp", loopback(port))
	if err != nil {
		return fmt.Errorf("can't open the dashboard on port %d (try --port): %w", port, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	served := make(chan error, 1)
	go func() { served <- serveDashboard(ctx, l, webui.New(cliBackend{}, token).Handler()) }()
	c.open(port, token)
	fmt.Println("The accounts service isn't hosting it, so this serves it until Ctrl-C.")
	return <-served
}

func (c uiCmd) open(port int, token string) {
	link := fmt.Sprintf("http://localhost:%d/?token=%s", port, token)
	fmt.Printf("spinup dashboard: %s\n", link)
	if !c.NoBrowser && !openBrowser(link) {
		fmt.Println("Open the link above in your browser.")
	}
}

// hostDashboard keeps the dashboard up for as long as the service runs, waiting out a port that is
// taken (an older `spinup ui` still open) instead of giving up.
func hostDashboard(ctx context.Context, port int, logger *log.Logger) {
	if port == 0 {
		return
	}
	token, err := uiToken()
	if err != nil {
		logger.Printf("dashboard: %v", err)
		return
	}
	for ctx.Err() == nil {
		l, err := net.Listen("tcp", loopback(port))
		if err != nil {
			logger.Printf("dashboard: can't listen on %s: %v (retrying)", loopback(port), err)
			sleepCtx(ctx, 30*time.Second)
			continue
		}
		logger.Printf("dashboard on http://localhost:%d", port)
		if err := serveDashboard(ctx, l, webui.New(cliBackend{}, token).Handler()); err != nil {
			logger.Printf("dashboard: %v", err)
			sleepCtx(ctx, 5*time.Second)
		}
	}
}

// serveDashboard serves until ctx ends, then stops within five seconds.
func serveDashboard(ctx context.Context, l net.Listener, h http.Handler) error {
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(l) }()
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

// dashboardAnswers reports whether spinup's dashboard is already up on port.
func dashboardAnswers(port int) bool {
	client := *directMachineClient
	client.Timeout = 2 * time.Second
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	page, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	return strings.Contains(string(page), "<title>spillup</title>") || strings.Contains(string(page), "dashboard isn't in this build")
}

func loopback(port int) string { return net.JoinHostPort("127.0.0.1", fmt.Sprint(port)) }

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// uiToken is the dashboard's sign-in secret, shared by the service and this command and kept between runs,
// so a signed-in browser stays signed in.
func uiToken() (string, error) {
	path := filepath.Join(config.StateDir(), "ui-token")
	if data, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(data))) >= 32 {
		return strings.TrimSpace(string(data)), nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	return token, atomicfile.Write(path, []byte(token+"\n"), 0o600)
}
