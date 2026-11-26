package webui

import (
	"net/http"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/proxy"
)

// App is an AI app the dashboard can give hints for, and whether this machine has it.
type App struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
}

func (s *Server) serveApps(w http.ResponseWriter, _ *http.Request) { writeJSON(w, s.b.Apps()) }

// LoginStatus is how the shared CLIProxyAPI sees each login right now.
type LoginStatus struct {
	Logins  []proxy.LoginState `json:"logins"`
	Problem string             `json:"problem,omitempty"`
}

// serveProxyLogins asks the shared CLIProxyAPI how each login is doing, through the same guards as its
// dashboard, so a refused key is never retried into a ban.
func (s *Server) serveProxyLogins(w http.ResponseWriter, r *http.Request) {
	out := LoginStatus{Logins: []proxy.LoginState{}}
	if wait := s.refusals.wait(panelShared, time.Now()); wait > 0 {
		out.Problem = "CLIProxyAPI refused spillup's key a moment ago; spillup is waiting before asking again"
		writeJSON(w, out)
		return
	}
	p, err := s.b.Panel(panelShared)
	if err != nil {
		out.Problem = err.Error()
		writeJSON(w, out)
		return
	}
	states, err := proxy.LoginStates(r.Context(), p.Port, p.Key)
	switch {
	case err != nil && (contains(err.Error(), "401") || contains(err.Error(), "403")):
		s.refusals.refused(panelShared, time.Now())
		out.Problem = "CLIProxyAPI refused spillup's key"
	case err != nil:
		out.Problem = err.Error()
	default:
		out.Logins = states
	}
	writeJSON(w, out)
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func (s *Server) serveSharedQuotas(w http.ResponseWriter, r *http.Request) {
	q, err := s.b.SharedQuotas(r.Context(), r.URL.Query().Get("fresh") == "1")
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, q)
}
