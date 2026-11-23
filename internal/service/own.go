package service

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"slices"
	"strconv"
	"sync"

	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/route"
)

// ownLogins are the model families this machine serves with logins of its own. Each machine signs in for
// itself, so no two machines ever refresh the same token, whatever the provider does on refresh.
type ownLogins struct {
	mu       sync.Mutex
	families []route.Family
}

func (o *ownLogins) get() []route.Family {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.families)
}

func (o *ownLogins) serves(f route.Family) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return f != "" && slices.Contains(o.families, f)
}

func (o *ownLogins) set(fs []route.Family) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.families = fs
}

func ownFamilies(names []string) ([]route.Family, error) {
	var fs []route.Family
	for _, n := range names {
		f := route.Family(n)
		if !route.Known(f) {
			return nil, fmt.Errorf("unknown model family %q (known: %v)", n, route.Families)
		}
		if !slices.Contains(fs, f) {
			fs = append(fs, f)
		}
	}
	return fs, nil
}

func familyNames(fs []route.Family) []string {
	names := make([]string, len(fs))
	for i, f := range fs {
		names[i] = string(f)
	}
	return names
}

// liveConfig is config.json as it should be saved now: the settings the service started with, plus what
// the user changed while it ran.
func (m *Machine) liveConfig() config.Config {
	c := m.cfg
	c.Hold = m.hold()
	c.OwnModels = familyNames(m.own.get())
	return c
}

func (m *Machine) saveLive() error {
	if m.o.SaveConfig == nil {
		return nil
	}
	return m.o.SaveConfig(m.liveConfig())
}

// setOwn changes the families this machine serves itself, saves them, and starts or stops their proxy.
func (m *Machine) setOwn(ctx context.Context, names []string) error {
	fs, err := ownFamilies(names)
	if err != nil {
		return err
	}
	before := m.own.get()
	m.own.set(fs)
	if err := m.saveLive(); err != nil {
		m.own.set(before)
		return fmt.Errorf("save: %w", err)
	}
	m.log.Printf("own logins now serve %v (were %v)", fs, before)
	if len(fs) == 0 {
		m.stopOwn()
		return nil
	}
	return m.startOwn(ctx)
}

func (m *Machine) startOwn(ctx context.Context) error {
	if m.o.OwnProxy == nil {
		return fmt.Errorf("this machine has no CLIProxyAPI for its own logins")
	}
	if m.o.OwnProxy.Running() {
		return nil
	}
	if m.o.PrepareOwnProxy != nil {
		if err := m.o.PrepareOwnProxy(); err != nil {
			return fmt.Errorf("own CLIProxyAPI config: %w", err)
		}
	}
	return m.o.OwnProxy.Start(ctx)
}

func (m *Machine) stopOwn() {
	if m.o.OwnProxy != nil {
		m.o.OwnProxy.Stop(nil)
	}
}

func (m *Machine) ownRunning() bool { return m.o.OwnProxy != nil && m.o.OwnProxy.Running() }

func (m *Machine) newOwnForwarder() *httputil.ReverseProxy {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(m.cfg.OwnProxyPort))
	return &httputil.ReverseProxy{
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme, pr.Out.URL.Host, pr.Out.Host = "http", addr, ""
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			m.log.Printf("own logins %s: %v", r.URL.Path, err)
			writeError(w, http.StatusBadGateway, "this machine's own CLIProxyAPI didn't answer: "+err.Error())
		},
	}
}

// forwardLocal is the router for this machine's own programs: families it serves itself go to its own
// logins, never to the shared accounts, so a missing sign-in fails loudly instead of sharing a token.
func (m *Machine) forwardLocal(w http.ResponseWriter, r *http.Request) {
	if err := keepBody(r); err != nil {
		writeError(w, http.StatusBadRequest, "couldn't read the request: "+err.Error())
		return
	}
	f := requestFamily(r)
	if !m.own.serves(f) {
		m.forwarder.ServeHTTP(w, r)
		return
	}
	if !m.ownRunning() {
		writeError(w, http.StatusServiceUnavailable, fmt.Sprintf(
			"this machine uses its own %s sign-in, but its CLIProxyAPI isn't running: open `spinup ui` → Accounts", f))
		return
	}
	m.ownForwarder.ServeHTTP(w, r)
}

func requestFamily(r *http.Request) route.Family {
	var body []byte
	if r.GetBody != nil {
		if rc, err := r.GetBody(); err == nil {
			body, _ = io.ReadAll(rc)
			rc.Close()
		}
	}
	return route.FamilyOf(route.ModelOf(r.URL.Path, body))
}
