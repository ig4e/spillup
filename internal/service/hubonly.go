package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
)

// hubOnly is the fleet's choice to keep the logins on the hub alone: the hub decides it, and the others
// learn it from the hub's reports and remember it, so a standby that restarts while the hub is away
// still never takes the accounts over.
type hubOnly struct {
	mu sync.Mutex
	on bool
}

func (h *hubOnly) get() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.on
}

// set reports whether the setting changed.
func (h *hubOnly) set(on bool) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	changed := h.on != on
	h.on = on
	return changed
}

// keepsCopies reports whether this machine may hold copies of the logins it doesn't lead with.
func (m *Machine) keepsCopies() bool { return !m.hubOnly.get() || m.hold() == config.HoldHub }

var errNoHub = errors.New("no main machine answers right now: the main machine decides this, so try again when it's online")

// setHubOnly changes the setting on the hub, wherever the user asked: a standby passes it on once.
func (m *Machine) setHubOnly(ctx context.Context, on bool, forwarded bool) error {
	if m.hold() != config.HoldHub {
		if forwarded {
			return errNoHub
		}
		for _, p := range m.peers.peers() {
			if p.Hold == config.HoldHub {
				return m.callPeer(ctx, p.Name, api.PathMode, api.Mode{HubOnly: on}, nil)
			}
		}
		return errNoHub
	}
	if !m.hubOnly.set(on) {
		return nil
	}
	if err := m.saveLive(); err != nil {
		m.hubOnly.set(!on)
		return err
	}
	m.log.Printf("main machine only: %v", on)
	return nil
}

// learnHubOnly takes the setting from the hub's report; a standby that learns it drops its login copies.
func (m *Machine) learnHubOnly(reports map[string]api.Report) {
	if m.hold() == config.HoldHub {
		return
	}
	for _, r := range reports {
		if r.Hold != config.HoldHub || !m.hubOnly.set(r.HubOnly) {
			continue
		}
		m.log.Printf("the main machine set main machine only: %v", r.HubOnly)
		m.save()
		if err := m.saveLive(); err != nil {
			m.log.Printf("save main machine only: %v", err)
		}
		if r.HubOnly {
			m.dropCopies()
		}
		return
	}
}

// dropCopies deletes this machine's copies of the logins, unless it holds the accounts right now: the hub
// keeps the originals.
func (m *Machine) dropCopies() {
	m.transition.Lock()
	defer m.transition.Unlock()
	if m.ledger.view(time.Now()).claiming() {
		return
	}
	entries, err := os.ReadDir(m.cfg.AuthDir)
	if err != nil {
		return
	}
	dropped := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") && os.Remove(filepath.Join(m.cfg.AuthDir, e.Name())) == nil {
			dropped++
		}
	}
	if dropped > 0 {
		m.log.Printf("main machine only: removed %d login copies; the main machine keeps them", dropped)
	}
}
