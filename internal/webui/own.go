package webui

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/darkyeg/spinup/internal/logins"
	"github.com/darkyeg/spinup/internal/route"
)

func (s *Server) serveOwnState(w http.ResponseWriter, r *http.Request) {
	out := OwnState{Families: []string{}, Available: familyNames(), Logins: []logins.Summary{}}
	if f := s.b.Facts(); f.Installed {
		out.Families = orEmpty(f.Config.OwnModels)
		if rep, err := s.b.Report(r.Context()); err == nil {
			out.Running = rep.OwnRunning
		}
	}
	if own, err := s.b.OwnLogins(); err == nil && own != nil {
		out.Logins = own
	}
	writeJSON(w, out)
}

func familyNames() []string {
	names := make([]string, len(route.Families))
	for i, f := range route.Families {
		names[i] = string(f)
	}
	return names
}

func (s *Server) serveSetOwn(w http.ResponseWriter, r *http.Request) {
	var req ownRequest
	if !decodeBody(w, r, &req) {
		return
	}
	msg := "Every model uses the shared accounts again."
	if len(req.Families) > 0 {
		msg = fmt.Sprintf("This machine now uses its own sign-in for %s models. Sign in below.", strings.Join(req.Families, ", "))
	}
	answer(w, s.b.SetOwn(r.Context(), orEmpty(req.Families)), msg)
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
	link, err := m.StartSignIn(r.Context(), route.Family(req.Family))
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
