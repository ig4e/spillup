package service

import (
	"os"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/logins"
)

// Report describes this machine.
func (m *Machine) Report() api.Report {
	now := time.Now()
	st := m.ledger.view(now)
	waiting, leaderName, leaderAddr := m.notes.get()
	r := api.Report{
		Rollout: true, Name: m.tail.selfName(), Hold: m.hold(), Version: m.o.Version, Leading: st.Leading, Starting: st.Starting,
		Leader: leaderName, LeaderAddr: leaderAddr, Waiting: waiting, Peers: m.peers.peers(), Synced: m.replica.synced(now),
		InFlight: m.activity.inFlight(), Own: providerNames(m.own.get()), OwnRunning: m.ownRunning(), HubOnly: m.hubOnly.get(), PID: os.Getpid(),
	}
	if m.hold().CanHold() {
		r.Epoch, r.Leader = st.Epoch, st.Leader
	}
	if m.o.Proxy != nil {
		r.ProxyRunning = m.o.Proxy.Running()
		if r.Leading && r.ProxyRunning {
			r.ManagedProxyPort = m.cfg.ProxyPort
		}
	}
	if m.hold().CanHold() {
		r.Accounts = logins.Summarize(m.cfg.AuthDir)
	}
	return r
}
