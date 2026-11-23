package service

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
)

// ErrRoleNeedsSetup refuses a change into or out of "never": only setup gives a machine the keys and the peer API.
var ErrRoleNeedsSetup = errors.New("only hub and standby can switch while running; run `spinup setup <name> --hub|--standby` to make a machine hold the accounts")

// role is this machine's hold. The user may switch it between hub and standby while the service runs:
// both may hold, so only the preference for who holds changes, never the safety rules.
type role struct {
	mu   sync.Mutex
	hold config.Hold
}

func (r *role) get() config.Hold {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hold
}

// change reports the previous hold, or refuses a switch that isn't between hub and standby.
func (r *role) change(to config.Hold) (from config.Hold, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.hold.CanHold() || !to.CanHold() {
		return r.hold, ErrRoleNeedsSetup
	}
	from, r.hold = r.hold, to
	return from, nil
}

func (m *Machine) hold() config.Hold { return m.role.get() }

// setHold switches this machine between hub and standby, and saves it so a restart keeps it.
func (m *Machine) setHold(to config.Hold) error {
	m.transition.Lock()
	defer m.transition.Unlock()
	from, err := m.role.change(to)
	if err != nil || from == to {
		return err
	}
	if err := m.saveLive(); err != nil {
		_, _ = m.role.change(from)
		return fmt.Errorf("save the new role: %w", err)
	}
	m.log.Printf("the user made this machine %s (was %s)", to, from)
	return nil
}

// makeMain makes name the only hub. It promotes first, so there is always a hub, then demotes the others;
// the usual hand-back then moves the accounts to it between requests.
func (m *Machine) makeMain(ctx context.Context, name string) error {
	if err := m.setHoldOn(ctx, name, config.HoldHub); err != nil {
		return fmt.Errorf("make %s the hub: %w", name, err)
	}
	var errs []error
	for _, other := range m.otherHubs(name) {
		if err := m.setHoldOn(ctx, other, config.HoldStandby); err != nil {
			errs = append(errs, fmt.Errorf("%s is still a hub too: %w", other, err))
		}
	}
	return errors.Join(errs...)
}

func (m *Machine) setHoldOn(ctx context.Context, name string, to config.Hold) error {
	if name == m.tail.selfName() {
		return m.setHold(to)
	}
	return m.callPeer(ctx, name, api.PathRole, api.Role{Hold: to}, nil)
}

// otherHubs are the hubs this machine knows of besides keep, itself included.
func (m *Machine) otherHubs(keep string) []string {
	var hubs []string
	if self := m.tail.selfName(); self != keep && m.hold() == config.HoldHub {
		hubs = append(hubs, self)
	}
	for _, p := range m.peers.peers() {
		if p.Name != keep && p.Hold == config.HoldHub {
			hubs = append(hubs, p.Name)
		}
	}
	return hubs
}
