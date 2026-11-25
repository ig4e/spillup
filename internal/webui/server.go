// Package webui is spinup's dashboard: the built web app and the JSON API it calls, on localhost only.
package webui

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/doctor"
	"github.com/darkyeg/spinup/internal/logins"
	"github.com/darkyeg/spinup/internal/proxy"
	"github.com/darkyeg/spinup/internal/remoteweb"
	"github.com/darkyeg/spinup/internal/skills"
	"github.com/darkyeg/spinup/internal/sysinfo"
	"github.com/darkyeg/spinup/internal/tailnet"
)

// Backend is what the dashboard reads and changes; the command line implements it with the units its own commands use.
type Backend interface {
	SharedQuotas(context.Context) (api.QuotaSummary, error)
	Facts() Facts
	Report(ctx context.Context) (api.Report, error)
	Tailnet(ctx context.Context) (tailnet.Status, error)
	Doctor(ctx context.Context) []doctor.Section
	Keys() (config.Secrets, error)

	HandOff(to string) error
	TakeOver() error
	MakeMain(machine string) error
	RestartProxy() error
	// SetHubOnly keeps the logins on the main machine alone, or lets standbys keep copies and take over.
	SetHubOnly(on bool) error
	// ScanDevices finds which devices on the tailnet run spillup.
	ScanDevices(ctx context.Context) ([]Device, error)

	// OwnLogins are this machine's own logins; SetOwn picks the model families they serve, installing
	// CLIProxyAPI when needed; OwnManagement signs in and out on their proxy.
	OwnLogins() ([]logins.Summary, error)
	SetOwn(ctx context.Context, families []string) error
	OwnManagement() (proxy.Management, error)
	// Remote is how to reach another machine's dashboard over the tailnet: holders only.
	Remote(machine string) (Remote, error)
	// T3State, T3Pair and T3Link drive T3 Code here; SyncT3 links every machine's T3 to every other's.
	T3State(ctx context.Context) T3State
	T3DesktopLink(ctx context.Context, machine string) (string, error)
	T3Pair(ctx context.Context, label string) (string, error)
	T3Link(ctx context.Context, url, code string) error
	SyncT3(ctx context.Context) (string, error)
	T3Setup(ctx context.Context) error
	T3EnableLinking() error

	// Apps are the AI apps installed here, for hints that fit this machine.
	Apps() []App

	// RunFix runs a doctor fix that is a spinup command; StartSetup and SetupProgress set this machine up.
	RunFix(ctx context.Context, fix string) (string, error)
	PreviewSetup(ctx context.Context, name, role string) (SetupPreview, error)
	StartSetup(name, role string) error
	SetupProgress() SetupProgress
	AnswerSetup(password string, cancel bool) error

	// UpdateState, StartUpdate and SetAutoUpdate keep spillup itself up to date.
	UpdateState(ctx context.Context) Update
	StartUpdate() error
	StartRollout(context.Context, string) error
	RolloutPlan(context.Context) (FleetPreview, error)
	SetAutoUpdate(auto bool) error
	// SetAccess lets your devices open this dashboard over Tailscale, and the dashboard open terminals here.
	SetAccess(devices, terminals bool) error
	UpdateSystem() error
	// RestartService restarts the accounts service and the dashboard it hosts.
	RestartService() error

	// Remote desktop: this machine's Sunshine, Moonlight and moonlight-web, PINs for Sunshine, pairing the
	// other machines, setting it up, and where moonlight-web answers.
	RemoteClipboard(context.Context, uint32) (ClipboardText, error)
	HostClipboard(context.Context, string) (ClipboardText, error)
	RemoteState(ctx context.Context) RemoteState
	SunshinePIN(ctx context.Context, pin, name, fromIP string) error
	RemoteSync(ctx context.Context) (string, error)
	RemoteSetup() error
	RemoteWebPort() int

	// SSHState, SSHEnable and SSHSync make `ssh <machine>` work between your machines.
	SSHState(ctx context.Context) SSHState
	SSHEnable(on bool) error
	SSHSync(ctx context.Context) (string, error)

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
	changing *sync.Mutex
	refusals refusals
	stats    *sysinfo.Sampler
	tailnet  tailnetView
	terms    terminals
}

var dashboardChanges sync.Mutex

func New(b Backend, token string) *Server {
	return &Server{b: b, token: token, assets: assets(), stats: sysinfo.NewSampler(), changing: &dashboardChanges}
}

// reach says who may call a dashboard route: the browser signed in here, or the main machine's dashboard
// over the tailnet, which the service has already checked.
type reach int

const (
	browser reach = iota
	bridged
	// device is the dashboard on one of your devices, such as a phone, over the tailnet.
	device
)

// localOnly routes never answer over the tailnet: they reveal keys, or need a browser on this machine.
var localOnly = map[string]bool{"t3/desktop": true, "keys": true, "own/signin": true, "setup": true, "setup/preview": true, "setup/answer": true, "remote/setup": true, "update/fleet": true, "update/system": true, "ssh/enable": true, "settings/access": true}

func (s *Server) apiRoutes(mux *http.ServeMux, from reach) {
	admit := s.needsSession
	switch from {
	case bridged:
		admit = jsonOnly
	case device:
		admit = func(h http.HandlerFunc) http.HandlerFunc { return h }
	}
	route := func(method, path string, h http.HandlerFunc) {
		if from != browser && localOnly[path] {
			return
		}
		mux.HandleFunc(method+" /api/"+path, admit(h))
	}
	add := func(method, path string, h http.HandlerFunc) {
		if method == http.MethodPost {
			h = s.serialized(h)
			h = trackAction(from, path, h)
		}
		route(method, path, h)
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
	read("machines/scan", s.serveScan)
	change("accounts/mode", s.serveAccountsMode)

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
	change("t3/desktop", s.serveT3Desktop)
	change("t3/link", s.serveT3Link)
	change("t3/sync", s.serveT3Sync)
	change("t3/setup", s.serveT3Setup)
	change("t3/linking", s.serveT3Linking)

	change("fix", s.serveFix)
	change("setup", s.serveSetup)
	read("setup", s.serveSetupProgress)
	read("setup/preview", s.serveSetupPreview)
	route(http.MethodPost, "setup/answer", s.serveSetupAnswer)

	if from == bridged {
		route(http.MethodPost, "host/clipboard", s.serveHostClipboard)
	} else {
		route(http.MethodPost, "remote/clipboard", s.serveRemoteClipboard)
	}
	read("remote", s.serveRemoteState)
	change("sunshine/pin", s.serveSunshinePin)
	change("remote/sync", s.serveRemoteSync)
	change("remote/setup", s.serveRemoteSetup)
	change("processes/end", s.serveEndProcess)
	read("activity", s.serveActivity)
	read("activity/jobs", s.serveJobs)
	read("machines/history", s.serveHealthTimeline)
	read("stats", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, s.stats.Stats()) })
	read("apps", s.serveApps)
	read("proxy/logins", s.serveProxyLogins)
	read("accounts/quotas", s.serveSharedQuotas)

	route(http.MethodPost, "term", trackAction(from, "term", s.serveTerminalStart))
	route(http.MethodGet, "term/{id}/out", s.serveTerminalOutput)
	route(http.MethodPost, "term/{id}/in", s.serveTerminalInput)
	route(http.MethodPost, "term/{id}/size", s.serveTerminalResize)
	route(http.MethodPost, "term/{id}/close", s.serveTerminalClose)

	read("ssh", s.serveSSH)
	change("ssh/enable", s.serveSSHEnable)
	change("ssh/sync", s.serveSSHSync)

	read("update", s.serveUpdateState)
	read("update/fleet", s.serveRolloutPlan)
	change("update", s.serveUpdate)
	change("update/rollout", s.serveRollout)
	change("update/auto", s.serveAutoUpdate)
	change("settings/access", s.serveAccess)
	change("update/system", s.serveUpdateSystem)
	change("restart", s.serveRestartService)

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
	return s.guard(securityHeaders(s.appMux(browser, s.needsSession)))
}

// appMux is the whole dashboard, the app and its API, for a browser that admit lets in.
func (s *Server) appMux(from reach, admit func(http.HandlerFunc) http.HandlerFunc) *http.ServeMux {
	mux := http.NewServeMux()
	s.apiRoutes(mux, from)
	mux.HandleFunc("GET /api/remote/{machine}/{path...}", admit(s.serveRemote))
	mux.HandleFunc("POST /api/remote/{machine}/{path...}", admit(s.serveRemote))

	s.panelRoutes(mux, admit)
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		mux.HandleFunc(method+" "+remoteweb.Prefix+"/", admit(s.serveRemoteWeb))
	}
	mux.HandleFunc("GET /", s.serveApp)
	return mux
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
		if !s.changing.TryLock() {
			writeError(w, http.StatusConflict, "another dashboard change is running. Try again when it finishes")
			return
		}
		defer s.changing.Unlock()
		h(w, r)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/cpa/") || isRemoteWeb(r.URL.Path) {
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
	controller := http.NewResponseController(w)
	if controller.SetReadDeadline(time.Now().Add(10*time.Second)) == nil {
		defer controller.SetReadDeadline(time.Time{})
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	if err := decoder.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "bad request: "+err.Error())
		return false
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		writeError(w, http.StatusBadRequest, "request must contain one JSON value")
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
