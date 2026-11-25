package service

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/proxy"
)

type quotaCache struct {
	mu      sync.Mutex
	summary api.QuotaSummary
	err     error
	until   time.Time
	epoch   int64
}

func (m *Machine) serveQuotaView(w http.ResponseWriter, r *http.Request) {
	if m.ledger.view(time.Now()).Leading {
		m.serveHolderQuotas(w, r)
		return
	}
	_, leader, addr := m.notes.get()
	node, ok := m.tail.ownedPeer(leader)
	if !ok || node.IP == "" || addr == "" || addr != m.o.PeerAddr(leader, node.IP) {
		writeError(w, http.StatusServiceUnavailable, "no account holder is available on your Tailscale account")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 19*time.Second)
	defer cancel()
	client := api.Client{HTTP: m.httpClient, APIKey: m.o.Secrets.APIKey}
	var summary api.QuotaSummary
	if err := client.Get(ctx, m.peerURL(leader, node.IP, api.PathQuotas), &summary); err != nil {
		writeError(w, http.StatusBadGateway, "the account holder could not provide a quota summary; check its service and Spillup version")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, summary)
}

func (m *Machine) serveHolderQuotas(w http.ResponseWriter, r *http.Request) {
	st := m.ledger.view(time.Now())
	if !st.Leading || m.o.Proxy == nil || !m.o.Proxy.Running() || m.o.Secrets.ManagementPassword == "" {
		writeError(w, http.StatusServiceUnavailable, "this machine is not serving the shared accounts")
		return
	}
	// One bounded read serves all dashboards. Back off after a refusal to avoid a proxy key ban.
	cache := &m.quotas
	cache.mu.Lock()
	if cache.epoch != st.Epoch || time.Now().After(cache.until) {
		end, open := m.activity.begin()
		if !open {
			cache.mu.Unlock()
			writeError(w, http.StatusServiceUnavailable, "the account holder is handing off")
			return
		}
		cache.summary, cache.err = proxy.ReadQuotas(r.Context(), m.cfg.ProxyPort, m.o.Secrets.ManagementPassword)
		end()
		cache.epoch = st.Epoch
		cache.until = time.Now().Add(5 * time.Minute)
		if cache.err != nil {
			cache.until = time.Now().Add(2 * time.Minute)
		}
		if r.Context().Err() != nil {
			cache.until = time.Time{}
		}
	}
	summary, err := cache.summary, cache.err
	cache.mu.Unlock()
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, summary)
}
