package webui

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/darkyeg/spinup/internal/logins"
	"github.com/darkyeg/spinup/internal/route"
)

func (s *Server) serveOwnState(w http.ResponseWriter, r *http.Request) {
	out := OwnState{Providers: []string{}, Logins: []logins.Summary{}}
	var types []string
	if f := s.b.Facts(); f.Installed {
		out.Providers = orEmpty(f.Config.OwnModels)
		if rep, err := s.b.Report(r.Context()); err == nil {
			out.Running = rep.OwnRunning
			for _, a := range rep.Accounts {
				types = append(types, a.Type)
			}
		}
	}
	if own, err := s.b.OwnLogins(); err == nil && own != nil {
		out.Logins = own
		for _, a := range own {
			types = append(types, a.Type)
		}
	}
	out.Available = availableProviders(append(types, out.Providers...))
	writeJSON(w, out)
}

// availableProviders are the providers CLIProxyAPI signs in to, then any other kind of login found here,
// so a new provider shows up on the dashboard by itself.
func availableProviders(found []string) []ProviderInfo {
	var out []ProviderInfo
	seen := map[route.Provider]bool{}
	for _, s := range route.SignIns {
		out = append(out, ProviderInfo{ID: string(s.Provider), Name: s.Name, SignIn: true})
		seen[s.Provider] = true
	}
	for _, t := range found {
		p := route.Canonical(strings.ToLower(strings.TrimSpace(t)))
		if p == "" || seen[p] || !route.Valid(string(p)) {
			continue
		}
		seen[p] = true
		out = append(out, ProviderInfo{ID: string(p), Name: titled(string(p))})
	}
	return out
}

func titled(id string) string {
	words := strings.Split(id, "-")
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func (s *Server) serveSetOwn(w http.ResponseWriter, r *http.Request) {
	var req ownRequest
	if !decodeBody(w, r, &req) {
		return
	}
	msg := "Every provider uses the shared accounts again."
	if len(req.Providers) > 0 {
		msg = fmt.Sprintf("This machine now signs in to %s by itself.", strings.Join(req.Providers, ", "))
	}
	answer(w, s.b.SetOwn(r.Context(), orEmpty(req.Providers)), msg)
}

func (s *Server) serveSignIn(w http.ResponseWriter, r *http.Request) {
	var req signInRequest
	if !decodeBody(w, r, &req) {
		return
	}
	m, err := s.b.OwnManagement()
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	link, err := m.StartSignIn(r.Context(), route.Canonical(req.Provider))
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, link)
}

func (s *Server) serveSignInStatus(w http.ResponseWriter, r *http.Request) {
	m, err := s.b.OwnManagement()
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	st, err := m.SignInStatus(r.Context(), r.URL.Query().Get("state"))
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, st)
}

func (s *Server) serveSignOut(w http.ResponseWriter, r *http.Request) {
	var req nameRequest
	if !decodeBody(w, r, &req) || !named(w, []string{req.Name}) {
		return
	}
	m, err := s.b.OwnManagement()
	if err == nil {
		err = m.SignOut(r.Context(), req.Name)
	}
	answer(w, err, "Signed out on this machine.")
}
