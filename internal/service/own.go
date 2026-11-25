package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/route"
)

// ownLogins are the providers this machine serves with sign-ins of its own, and the models those sign-ins
// serve as its own CLIProxyAPI last listed them. Each machine signs in for itself, so no two machines ever
// refresh the same token, whatever the provider does on refresh.
type ownLogins struct {
	mu        sync.Mutex
	providers []route.Provider
	models    map[string]bool
}

func (o *ownLogins) get() []route.Provider {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.providers)
}

func (o *ownLogins) set(ps []route.Provider) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.providers = ps
}

func (o *ownLogins) setModels(models map[string]bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.models = models
}

// serves reports whether a model goes to this machine's own sign-ins: one they serve, or, until its
// CLIProxyAPI has said, one whose name belongs to a provider signed in here, so a missing sign-in fails
// loudly instead of using the shared token.
func (o *ownLogins) serves(model string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if model == "" || len(o.providers) == 0 {
		return false
	}
	if o.models[model] {
		return true
	}
	for _, p := range route.ProvidersOf(model) {
		if slices.Contains(o.providers, p) {
			return true
		}
	}
	return false
}

func ownProviders(names []string) ([]route.Provider, error) {
	var ps []route.Provider
	for _, n := range names {
		if !route.Valid(n) {
			return nil, fmt.Errorf("%q isn't a provider name", n)
		}
		p := route.Canonical(n)
		if !slices.Contains(ps, p) {
			ps = append(ps, p)
		}
	}
	return ps, nil
}

func providerNames(ps []route.Provider) []string {
	names := make([]string, len(ps))
	for i, p := range ps {
		names[i] = string(p)
	}
	return names
}

// liveConfig is config.json as it should be saved now: the settings the service started with, plus what
// the user changed while it ran.
func (m *Machine) liveConfig() config.Config {
	c := m.cfg
	c.Hold = m.hold()
	c.OwnModels = providerNames(m.own.get())
	c.HubOnly = m.hubOnly.get()
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
	fs, err := ownProviders(names)
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
	clearDeadline := readBodyDeadline(w, 2*time.Minute)
	defer clearDeadline()
	if err := keepBody(r); err != nil {
		writeError(w, http.StatusBadRequest, "couldn't read the request: "+err.Error())
		return
	}
	m.swapLegacyKey(r)
	model := requestModel(r)
	if !m.own.serves(model) {
		m.forwarder.ServeHTTP(w, r)
		return
	}
	if !m.ownRunning() {
		writeError(w, http.StatusServiceUnavailable, fmt.Sprintf(
			"this machine serves %s with its own sign-in, but its CLIProxyAPI isn't running: open `spinup ui` → Sign-In", model))
		return
	}
	m.ownForwarder.ServeHTTP(w, r)
}

func requestModel(r *http.Request) string {
	var body []byte
	if r.GetBody != nil {
		if rc, err := r.GetBody(); err == nil {
			body, _ = io.ReadAll(rc)
			rc.Close()
		}
	}
	return route.ModelOf(r.URL.Path, body)
}

// ownModelsEvery is how often the service asks its own CLIProxyAPI which models its sign-ins serve.
const ownModelsEvery = time.Minute

// learnOwnModels keeps the list of models this machine's own sign-ins serve, while it has any.
func (m *Machine) learnOwnModels(ctx context.Context) {
	for ctx.Err() == nil {
		if m.ownRunning() {
			if models, err := m.listOwnModels(ctx); err == nil {
				m.own.setModels(models)
			}
		}
		sleep(ctx, ownModelsEvery)
	}
}

func (m *Machine) listOwnModels(ctx context.Context) (map[string]bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	url := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(m.cfg.OwnProxyPort)) + "/v1/models"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+m.o.Secrets.APIKey)
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&list); err != nil {
		return nil, err
	}
	models := map[string]bool{}
	for _, d := range list.Data {
		models[d.ID] = true
	}
	return models, nil
}

// apiKeyHeaders are where clients put the proxy's API key: OpenAI and Claude Code style, Anthropic, Gemini.
var apiKeyHeaders = []string{"Authorization", "X-Api-Key", "X-Goog-Api-Key"}

// swapLegacyKey lets tools set up for an earlier CLIProxyAPI keep their key: on this machine only, an old
// key becomes the shared one before the request goes on.
func (m *Machine) swapLegacyKey(r *http.Request) {
	if isManagement(r.URL.Path) {
		m.swapKeys(r, append([]string{"X-Management-Key"}, apiKeyHeaders...), m.cfg.LegacyManagementKeys, m.o.Secrets.ManagementPassword)
		return
	}
	m.swapKeys(r, apiKeyHeaders, m.cfg.LegacyKeys, m.o.Secrets.APIKey)
}

func isManagement(path string) bool {
	return strings.HasPrefix(path, "/v0/management/") || strings.HasPrefix(path, "/v8/management/")
}

// swapKeys replaces any of old in headers with now, keeping a Bearer prefix.
func (m *Machine) swapKeys(r *http.Request, headers, old []string, now string) {
	if len(old) == 0 || now == "" {
		return
	}
	for _, h := range headers {
		v := r.Header.Get(h)
		token, bearer := strings.CutPrefix(v, "Bearer ")
		if v == "" || !slices.Contains(old, token) {
			continue
		}
		if bearer {
			r.Header.Set(h, "Bearer "+now)
		} else {
			r.Header.Set(h, now)
		}
	}
}
