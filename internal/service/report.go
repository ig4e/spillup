package service

import (
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/logins"
)

// Report describes this machine.
func (m *Machine) Report() api.Report {
	now := time.Now()
	st := m.ledger.view(now)
	waiting, leaderAddr := m.notes.get()
	r := api.Report{
		Name: m.tail.selfName(), Hold: m.hold(), Version: m.o.Version, Leading: st.Leading, Starting: st.Starting,
		LeaderAddr: leaderAddr, Waiting: waiting, Peers: m.peers.peers(), Synced: m.replica.synced(now),
		InFlight: m.activity.inFlight(), Own: providerNames(m.own.get()), OwnRunning: m.ownRunning(),
	}
	if m.hold().CanHold() {
		r.Epoch, r.Leader = st.Epoch, st.Leader
	}
	if m.o.Proxy != nil {
		r.ProxyRunning = m.o.Proxy.Running()
	}
	if m.hold().CanHold() {
		r.Accounts = logins.Summarize(m.cfg.AuthDir)
	}
	return r
}
