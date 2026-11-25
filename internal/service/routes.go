package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/logins"
)

// audience is who can reach a listener: this machine only, or the whole tailnet.
type audience int

const (
	onLocalhost audience = iota
	onTailnet
)

const (
	loginsLimit  = 64 << 20
	commandLimit = 4 << 10
	// handoffTimeout covers draining running requests, the leader's proxy going quiet and the target starting its own.
	handoffTimeout = drainWithin + 2*time.Minute
)

func (m *Machine) routes(a audience) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+api.PathLeader, m.serveLeader)
	mux.HandleFunc("GET "+api.PathLogins, m.keyed(m.holding(m.serveLogins)))
	mux.HandleFunc("POST "+api.PathLogins, m.keyed(m.holding(m.acceptLogins)))
	mux.HandleFunc("GET "+api.PathSecrets, m.keyed(m.serveSecrets))
	mux.HandleFunc("POST "+api.PathReceive, m.keyed(m.holding(m.acceptAccounts)))
	mux.HandleFunc("POST "+api.PathHandoff, m.keyed(m.serveHandoff))
	mux.HandleFunc("POST "+api.PathRole, m.keyed(m.holding(m.serveRole)))
	mux.HandleFunc("POST "+api.PathMode, m.keyed(m.holding(m.serveMode)))
	if m.library != nil {
		libraryAccess := m.withAPIKey
		if a == onTailnet {
			libraryAccess = m.withLibraryPeer
		}
		mux.HandleFunc("GET "+api.PathLibraryState, libraryAccess(m.serveLibraryState))
		mux.HandleFunc("GET "+api.PathLibrary, libraryAccess(m.serveLibrary))
		mux.HandleFunc("POST "+api.PathLibrary, libraryAccess(m.acceptLibrary))
		mux.HandleFunc("GET "+api.PathLibraryArtifact, libraryAccess(m.serveArtifact))
		mux.HandleFunc("POST "+api.PathLibraryArtifact, libraryAccess(m.acceptArtifact))
		if a == onLocalhost {
			mux.HandleFunc("POST "+api.PathLibrarySync, m.withAPIKey(m.serveLibrarySync))
		}
	}
	if m.o.Dashboard != nil && a == onTailnet {
		mux.Handle(dashboardPrefix, m.bridged(m.o.Dashboard))
	}
	switch a {
	case onLocalhost:
		mux.HandleFunc("GET "+api.PathState, m.serveState)
		// Only the user at this machine may force it to lead or stop it.
		mux.HandleFunc("POST "+api.PathTakeover, m.keyed(m.holding(m.serveTakeover)))
		mux.HandleFunc("POST "+api.PathStop, m.keyed(m.holding(m.serveStop)))
		mux.HandleFunc("POST "+api.PathRestart, m.keyed(m.holding(m.serveRestart)))
		mux.HandleFunc("POST "+api.PathMain, m.keyed(m.holding(m.serveMain)))
		mux.HandleFunc("POST "+api.PathOwn, m.withAPIKey(m.serveOwn))
	case onTailnet:
		mux.HandleFunc("GET "+api.PathState, m.keyed(m.serveState))
		mux.HandleFunc("POST "+api.PathJoin, m.holding(m.serveJoin))
	}
	mux.HandleFunc("/spinup/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "no such spinup call here")
	})
	if a == onLocalhost {
		mux.HandleFunc("/", m.forwardLocal)
	} else {
		mux.HandleFunc("/", m.forward)
	}
	return mux
}

const dashboardPrefix = "/spinup/ui/"

// tailnetHandler is what other machines reach on this one: everything a holder serves, or only the
// dashboard on a machine that just uses the accounts, which routes nobody's requests.
func (m *Machine) tailnetHandler() http.Handler {
	if m.hold().CanHold() {
		return m.routes(onTailnet)
	}
	mux := http.NewServeMux()
	mux.Handle(dashboardPrefix, m.bridged(m.o.Dashboard))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "this machine only uses the accounts")
	})
	return mux
}

// bridged admits another machine's dashboard. A machine that keeps the dashboard password answers to it,
// and to the API key from a device Tailscale says is on its own account, so your workers can open the main
// machine's dashboard and nobody else's device can. An API key alone can read a worker's overview.
func (m *Machine) bridged(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workerOverview := m.o.Secrets.ManagementPassword == "" &&
			r.Method == http.MethodGet && r.URL.Path == dashboardPrefix+"api/overview"
		if m.bridgeAdmitsOwner(r) || workerOverview && m.matchesAPIKey(r) {
			h.ServeHTTP(w, r)
			return
		}
		writeError(w, http.StatusUnauthorized, "missing or wrong key for this machine's dashboard")
	}
}

// bridgeAdmitsOwner admits the dashboard password, or the API key from one of your own devices, even where
// the API key alone opens the rest of the dashboard.
func (m *Machine) bridgeAdmitsOwner(r *http.Request) bool {
	if want := m.o.Secrets.ManagementPassword; want != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get(api.KeyHeader)), []byte(want)) == 1 {
		return true
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	_, yours := m.tail.owned(ip)
	return yours && m.matchesAPIKey(r)
}

// keyed admits callers with the management password, and remembers callers that can hold.
func (m *Machine) keyed(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		want := m.o.Secrets.ManagementPassword
		got := r.Header.Get(api.KeyHeader)
		if want == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			writeError(w, http.StatusUnauthorized, "missing or wrong "+api.KeyHeader)
			return
		}
		m.learn(r.Header.Get(api.ForwardedHeader), config.Hold(r.Header.Get(api.HoldHeader)))
		h(w, r)
	}
}

// holding refuses requests that only a machine able to hold the accounts can serve.
func (m *Machine) holding(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !m.hold().CanHold() {
			writeError(w, http.StatusConflict, "this machine never holds the accounts (install it with --standby)")
			return
		}
		h(w, r)
	}
}

func (m *Machine) serveLeader(w http.ResponseWriter, r *http.Request) {
	st := m.ledger.view(time.Now())
	l := api.Leader{
		Name: m.tail.selfName(), Hold: m.hold(), Leading: st.Leading, Starting: st.Starting,
		Leader: st.Leader, Epoch: st.Epoch,
	}
	writeJSON(w, l.Prove(m.o.Secrets, r.URL.Query().Get(api.NonceParam)))
}

func (m *Machine) serveState(w http.ResponseWriter, _ *http.Request) { writeJSON(w, m.Report()) }

func (m *Machine) serveLogins(w http.ResponseWriter, r *http.Request) {
	own, err := m.ownLogins(logins.Everything)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag := own.ETag(); tag != "" {
		w.Header().Set("ETag", tag)
		w.Header().Set("Cache-Control", "no-store")
		if r.Header.Get("If-None-Match") == tag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	writeJSON(w, own)
}

func (m *Machine) acceptLogins(w http.ResponseWriter, r *http.Request) {
	if !m.keepsCopies() {
		writeError(w, http.StatusConflict, "this standby keeps no login copies: the fleet keeps them on the main machine only")
		return
	}
	var incoming api.Logins
	if !decode(w, r, loginsLimit, &incoming) {
		return
	}
	if _, err := m.mergeLogins(r.Context(), incoming); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, okBody)
}

func (m *Machine) serveSecrets(w http.ResponseWriter, _ *http.Request) { writeJSON(w, m.o.Secrets) }

func (m *Machine) acceptAccounts(w http.ResponseWriter, r *http.Request) {
	var accounts api.Logins
	if !decode(w, r, loginsLimit, &accounts) {
		return
	}
	if err := m.receive(accounts); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, okBody)
}

// serveHandoff hands off if this machine leads, and otherwise passes the request to the leader once.
func (m *Machine) serveHandoff(w http.ResponseWriter, r *http.Request) {
	var req api.Handoff
	if !decode(w, r, commandLimit, &req) {
		return
	}
	st := m.ledger.view(time.Now())
	ctx, cancel := context.WithTimeout(r.Context(), handoffTimeout)
	defer cancel()
	var err error
	switch {
	case st.Leading:
		err = m.handOff(ctx, req.To, drainWithin)
	case st.Leader != "" && r.Header.Get(api.ForwardedHeader) == "":
		err = m.callPeer(ctx, st.Leader, api.PathHandoff, req, nil)
	default:
		err = errors.New("no machine holds the accounts right now")
	}
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, okBody)
}

func (m *Machine) serveTakeover(w http.ResponseWriter, _ *http.Request) {
	if err := m.ledger.takeover(time.Now()); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	m.log.Printf("the user asked this machine to take the accounts")
	writeJSON(w, okBody)
}

func (m *Machine) serveStop(w http.ResponseWriter, _ *http.Request) {
	m.log.Printf("the user asked the service to stop")
	writeJSON(w, okBody)
	_ = http.NewResponseController(w).Flush()
	m.stopRun()
}

func (m *Machine) serveRestart(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), handoffTimeout)
	defer cancel()
	if err := m.restartProxy(ctx); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, okBody)
}

func (m *Machine) serveRole(w http.ResponseWriter, r *http.Request) {
	var req api.Role
	if !decode(w, r, commandLimit, &req) {
		return
	}
	if err := m.setHold(req.Hold); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, okBody)
}

func (m *Machine) serveMain(w http.ResponseWriter, r *http.Request) {
	var req api.Main
	if !decode(w, r, commandLimit, &req) {
		return
	}
	if err := m.makeMain(r.Context(), req.Machine); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, okBody)
}

func (m *Machine) serveOwn(w http.ResponseWriter, r *http.Request) {
	var req api.Own
	if !decode(w, r, commandLimit, &req) {
		return
	}
	if err := m.setOwn(r.Context(), req.Providers); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, okBody)
}

// serveJoin gives a new machine its keys without anyone typing them, but only when Tailscale vouches that
// the caller is one of your own devices: signed in with this machine's account, untagged. Tailscale
// guarantees a tailnet address can't be forged.
func (m *Machine) serveJoin(w http.ResponseWriter, r *http.Request) {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	peer, ok := m.tail.owned(ip)
	if !ok {
		writeError(w, http.StatusForbidden, "only devices signed in with this machine's own Tailscale account join by themselves; use `spinup keys` instead")
		return
	}
	var req api.Join
	if !decode(w, r, commandLimit, &req) {
		return
	}
	keys := config.Secrets{APIKey: m.o.Secrets.APIKey}
	if req.Hold.CanHold() {
		keys.ManagementPassword = m.o.Secrets.ManagementPassword
	}
	m.log.Printf("%s joined as %s", peer.Name, req.Hold)
	writeJSON(w, keys)
}

func (m *Machine) serveMode(w http.ResponseWriter, r *http.Request) {
	var req api.Mode
	if !decode(w, r, commandLimit, &req) {
		return
	}
	if err := m.setHubOnly(r.Context(), req.HubOnly, r.Header.Get(api.ForwardedHeader) != ""); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, okBody)
}

var okBody = map[string]bool{"ok": true}

func decode(w http.ResponseWriter, r *http.Request, limit int64, v any) bool {
	clearDeadline := readBodyDeadline(w, time.Minute)
	defer clearDeadline()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	err := decoder.Decode(v)
	if err == nil {
		err = decoder.Decode(new(json.RawMessage))
		switch {
		case errors.Is(err, io.EOF):
			err = nil
		case err == nil:
			err = errors.New("request must contain one JSON value")
		}
	}
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "request too large")
	case err != nil:
		writeError(w, http.StatusBadRequest, "bad request: "+err.Error())
	}
	return err == nil
}

func writeJSON(w http.ResponseWriter, v any) { writeJSONStatus(w, http.StatusOK, v) }

func writeError(w http.ResponseWriter, status int, message string) {
	var e api.Error
	e.Error.Message = message
	writeJSONStatus(w, status, e)
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
