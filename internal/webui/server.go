// Package webui is spinup's dashboard: the built web app and the JSON API it calls, on localhost only.
package webui

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"sync"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/doctor"
	"github.com/darkyeg/spinup/internal/skills"
	"github.com/darkyeg/spinup/internal/tailnet"
)

// Backend is what the dashboard reads and changes; the command line implements it with the units its own commands use.
type Backend interface {
	Facts() Facts
	Report(ctx context.Context) (api.Report, error)
	Tailnet(ctx context.Context) (tailnet.Status, error)
	Doctor(ctx context.Context) []doctor.Section
	Keys() (config.Secrets, error)

	HandOff(to string) error
	TakeOver() error
	MakeMain(machine string) error
	RestartProxy() error

	// Skills is a manager over your library; CatchUpLibrary takes in a newer one from your other machines first.
	Skills() skills.Manager
	CatchUpLibrary()
}

// Facts are this machine's settings, read once per request.
type Facts struct {
	Installed bool
	Config    config.Config
	Version   string
	OS        string
	// CanManage: this machine keeps the dashboard password.
	CanManage bool
}

type Server struct {
	b      Backend
	token  string
	assets fs.FS
	// changing serializes edits, so two clicks never edit the library at once.
	changing sync.Mutex
}

func New(b Backend, token string) *Server { return &Server{b: b, token: token, assets: assets()} }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	read := func(path string, h http.HandlerFunc) { mux.HandleFunc("GET /api/"+path, s.needsSession(h)) }
	change := func(path string, h http.HandlerFunc) {
		mux.HandleFunc("POST /api/"+path, s.needsSession(s.serialized(h)))
	}

	read("overview", s.serveOverview)
	read("machines", s.serveMachines)
	read("doctor", s.serveDoctor)
	read("skills", s.serveSkills)
	read("keys", s.serveKeys)

	change("accounts/handoff", s.serveHandoff)
	change("accounts/takeover", s.serveTakeover)
	change("accounts/restart", s.serveRestart)
	change("machines/main", s.serveMain)

	change("skills/mode", s.serveSkillMode)
	change("skills/remove", s.serveSkillRemove)
	change("skills/restore", s.serveSkillRestore)
	change("skills/adopt", s.serveSkillAdopt)
	change("skills/add", s.serveSkillAdd)
	change("skills/sync", s.serveSkillSync)

	unknown := func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "no such dashboard call")
	}
	mux.HandleFunc("GET /api/", unknown)
	mux.HandleFunc("POST /api/", unknown)
	mux.HandleFunc("GET /", s.serveApp)
	return s.guard(securityHeaders(mux))
}

func (s *Server) serialized(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.changing.Lock()
		defer s.changing.Unlock()
		h(w, r)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "bad request: "+err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	var e api.Error
	e.Error.Message = message
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(e)
}

// answer writes done, or the error as a 409 the dashboard shows as is.
func answer(w http.ResponseWriter, err error, done string) {
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, Done{Message: done})
}
