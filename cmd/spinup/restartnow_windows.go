package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
)

// Ask a holder to hand off first. Only the named boot task is ended if it stays up.
func restartServiceNow(cfg config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 145*time.Second)
	defer cancel()
	l := localService(cfg, "")
	report := func(ctx context.Context) (api.Report, error) {
		query, stop := context.WithTimeout(ctx, 2*time.Second)
		defer stop()
		var r api.Report
		err := l.client.Get(query, l.base+api.PathState, &r)
		return r, err
	}
	old, err := report(ctx)
	if err == nil && old.PID > 0 {
		if cfg.Hold.CanHold() {
			secrets, err := config.LoadSecrets(cfg)
			if err == nil && secrets.ManagementPassword != "" {
				client := l.client
				client.Key = secrets.ManagementPassword
				grace, done := context.WithTimeout(ctx, 105*time.Second)
				post, cancelPost := context.WithTimeout(grace, 8*time.Second)
				stopErr := client.Post(post, l.base+api.PathStop, struct{}{}, nil)
				cancelPost()
				for stopErr == nil && grace.Err() == nil {
					next, err := report(grace)
					if err != nil || next.PID != old.PID {
						break
					}
					sleepCtx(grace, 300*time.Millisecond)
				}
				done()
			}
		}
		next, err := report(ctx)
		if err == nil && next.PID == old.PID {
			if err := windowsTaskCommand(ctx, "/End", "/TN", "spinup"); err != nil {
				return fmt.Errorf("stop the Spillup boot task: %w", err)
			}
			for ctx.Err() == nil {
				next, err = report(ctx)
				if err != nil || next.PID != old.PID {
					break
				}
				sleepCtx(ctx, 300*time.Millisecond)
			}
			if ctx.Err() != nil {
				return errors.New("the old service is still running; check its boot task")
			}
		}
	} else {
		// Ignore "not running". An unresponsive boot task would otherwise ignore /Run.
		_ = windowsTaskCommand(ctx, "/End", "/TN", "spinup")
	}
	// Task Scheduler can acknowledge /Run while the previous instance is finishing.
	start, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	for start.Err() == nil {
		if err := windowsTaskCommand(start, "/Run", "/TN", "spinup"); err != nil {
			return fmt.Errorf("start the Spillup boot task: %w", err)
		}
		sleepCtx(start, 2*time.Second)
		r, err := report(start)
		if err == nil && r.PID > 0 && r.PID != old.PID {
			return nil
		}
	}
	return errors.New("the restarted Spillup boot task did not answer within 30 seconds")
}
