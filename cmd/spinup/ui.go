package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
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

type uiCmd struct {
	Port      int  `default:"8320" help:"Port for the dashboard, on localhost only."`
	NoBrowser bool `help:"Print the link instead of opening a browser."`
}

func (uiCmd) Help() string {
	return `Opens spinup's dashboard in your browser: your machines, who holds the accounts and which machine
is the hub, your skills, and the doctor's checks. It answers only on this machine, and only to the
browser that opened its link. Ctrl-C stops it; the accounts service keeps running either way.`
}

func (c uiCmd) Run() error {
	token, err := uiToken()
	if err != nil {
		return err
	}
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprint(c.Port)))
	if err != nil {
		return fmt.Errorf("can't open the dashboard on port %d (is it already open? try --port): %w", c.Port, err)
	}
	srv := &http.Server{Handler: webui.New(cliBackend{}, token).Handler(), ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	served := make(chan error, 1)
	go func() { served <- srv.Serve(l) }()

	link := fmt.Sprintf("http://localhost:%d/?token=%s", c.Port, token)
	fmt.Printf("spinup dashboard: %s\nCtrl-C stops it.\n", link)
	if !c.NoBrowser && !openBrowser(link) {
		fmt.Println("Open the link above in your browser.")
	}
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

// uiToken is the dashboard's sign-in secret, kept between runs so an open tab stays signed in.
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
