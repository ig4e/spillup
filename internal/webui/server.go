// Package webui is spinup's dashboard: the built web app and the JSON API it calls, on localhost only.
package webui

import (
	"context"
	"encoding/json"
	"io/fs"
	"mime"
	"net/http"
	"strings"
	"sync"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/doctor"
	"github.com/darkyeg/spinup/internal/logins"
	"github.com/darkyeg/spinup/internal/proxy"
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

	// OwnLogins are this machine's own logins; SetOwn picks the model families they serve, installing
	// CLIProxyAPI when needed; OwnManagement signs in and out on their proxy.
	OwnLogins() ([]logins.Summary, error)
	SetOwn(ctx context.Context, families []string) error
	OwnManagement() (proxy.Management, error)
	// Remote is how to reach another machine's dashboard over the tailnet: holders only.
	Remote(machine string) (Remote, error)
	// T3State, T3Pair and T3Link drive T3 Code here; SyncT3 links every machine's T3 to every other's.
	T3State(ctx context.Context) T3State
	T3Pair(ctx context.Context, label string) (string, error)
	T3Link(ctx context.Context, url, code string) error
	SyncT3(ctx context.Context) (string, error)

	// Apps are the AI apps installed here, for hints that fit this machine.
	Apps() []App

	// UpdateState, StartUpdate and SetAutoUpdate keep spillup itself up to date.
	UpdateState(ctx context.Context) Update
	StartUpdate() error
	SetAutoUpdate(auto bool) error

	// Panel is the CLIProxyAPI behind a Management Center view: "shared" (holders only) or "own".
	Panel(target string) (Panel, error)

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
	refusals refusals
}

func New(b Backend, token string) *Server { return &Server{b: b, token: token, assets: assets()} }

// reach says who may call a dashboard route: the browser signed in here, or the main machine's dashboard
// over the tailnet, which the service has already checked.
type reach int

const (
	browser reach = iota
	bridged
)

// localOnly routes never answer over the tailnet: they reveal keys, or need a browser on this machine.
var localOnly = map[string]bool{"keys": true, "own/signin": true}

func (s *Server) apiRoutes(mux *http.ServeMux, from reach) {
	admit := s.needsSession
	if from == bridged {
		admit = jsonOnly
	}
	add := func(method, path string, h http.HandlerFunc) {
		if from == bridged && localOnly[path] {
			return
		}
		if method == http.MethodPost {
			h = s.serialized(h)
		}
		mux.HandleFunc(method+" /api/"+path, admit(h))
	}
	read := func(path string, h http.HandlerFunc) { add(http.MethodGet, path, h) }
	change := func(path string, h http.HandlerFunc) { add(http.MethodPost, path, h) }

	read("overview", s.serveOverview)
	read("machines", s.serveMachines)
	read("doctor", s.serveDoctor)
	read("skills", s.serveSkills)
	read("keys", s.serveKeys)

	change("accounts/handoff", s.serveHandoff)
	change("accounts/takeover", s.serveTakeover)
	change("accounts/restart", s.serveRestart)
	change("machines/main", s.serveMain)

	read("own", s.serveOwnState)
	change("own", s.serveSetOwn)
	change("own/signin", s.serveSignIn)
	read("own/signin", s.serveSignInStatus)
	change("own/signout", s.serveSignOut)

	change("skills/mode", s.serveSkillMode)
	change("skills/remove", s.serveSkillRemove)
	change("skills/restore", s.serveSkillRestore)
	change("skills/adopt", s.serveSkillAdopt)
	change("skills/add", s.serveSkillAdd)
	change("skills/sync", s.serveSkillSync)

	read("t3", s.serveT3)
	change("t3/pair", s.serveT3Pair)
	change("t3/link", s.serveT3Link)
	change("t3/sync", s.serveT3Sync)

	read("apps", s.serveApps)
	read("proxy/logins", s.serveProxyLogins)

	read("update", s.serveUpdateState)
	change("update", s.serveUpdate)
	change("update/auto", s.serveAutoUpdate)

	if from == bridged {
		s.panelRoutes(mux, func(h http.HandlerFunc) http.HandlerFunc { return h })
	}

	unknown := func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "no such dashboard call")
	}
	mux.HandleFunc("GET /api/", unknown)
	mux.HandleFunc("POST /api/", unknown)
}

// panelRoutes serve CLIProxyAPI's Management Center: to the browser here, and to the main machine's
// dashboard over the tailnet, so it shows each machine's own.
func (s *Server) panelRoutes(mux *http.ServeMux, admit func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /cpa/{target}/{$}", admit(s.servePanelPage))
	for _, prefix := range panelAPIPrefixes {
		for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
			mux.HandleFunc(method+" "+prefix, admit(s.servePanelAPI))
		}
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.apiRoutes(mux, browser)
	mux.HandleFunc("GET /api/remote/{machine}/{path...}", s.needsSession(s.serveRemote))
	mux.HandleFunc("POST /api/remote/{machine}/{path...}", s.needsSession(s.serveRemote))

	s.panelRoutes(mux, s.needsSession)
	mux.HandleFunc("GET /", s.serveApp)
	return s.guard(securityHeaders(mux))
}

// BridgePrefix is where the service serves BridgeHandler on the tailnet.
const BridgePrefix = "/spinup/ui"

// BridgeHandler is this machine's dashboard API for the main machine's dashboard, over the tailnet. The
// service checks the caller's key first; keys and sign-ins stay local.
func (s *Server) BridgeHandler() http.Handler {
	mux := http.NewServeMux()
	s.apiRoutes(mux, bridged)
	return http.StripPrefix(BridgePrefix, mux)
}

// jsonOnly takes changes only as JSON, like a browser's, for calls from another machine's dashboard.
func jsonOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			if media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || media != "application/json" {
				writeError(w, http.StatusUnsupportedMediaType, "changes are JSON")
				return
			}
		}
		h(w, r)
	}
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
		h.Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/cpa/") {
			h.Set("X-Frame-Options", "SAMEORIGIN")
			h.Set("Content-Security-Policy", "frame-ancestors 'self'")
		} else {
			h.Set("X-Frame-Options", "DENY")
			h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none'")
		}
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
