package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/shell"
)

type restartCmd struct{}

func (restartCmd) Help() string {
	return `Restarts the accounts service, and the dashboard it hosts, on the spillup installed now. Running
requests finish and the accounts move on first, so nothing is cut off. No administrator rights needed.`
}

func (restartCmd) Run() error {
	cfg, err := config.Load()
	if errors.Is(err, config.ErrNotInstalled) {
		return errNotInstalled
	}
	if err != nil {
		return err
	}
	step("Restarting the accounts service")
	if err := restartServiceNow(cfg); err != nil {
		return err
	}
	l := localService(cfg, "")
	var running string
	answered := pollUntil(time.Minute, func() bool {
		r, err := l.report()
		running = r.Version
		return err == nil
	})
	if !answered {
		return errors.New("the service didn't come back within a minute: run spinup doctor")
	}
	fmt.Printf("The accounts service runs spillup %s.\n", running)
	return nil
}

// serviceVersion is the spillup version the running service reports, or "" when it doesn't answer.
func serviceVersion(cfg config.Config) string {
	r, err := localService(cfg, "").report()
	if err != nil {
		return ""
	}
	return r.Version
}

func runServiceManager(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	_, err := shell.Output(ctx, name, args...)
	if ctx.Err() != nil {
		return fmt.Errorf("restart service with %s: %w", name, ctx.Err())
	}
	return err
}
