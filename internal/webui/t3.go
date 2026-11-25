package webui

import (
	"net/http"

	"github.com/darkyeg/spinup/internal/t3code"
)

// T3State is T3 Code on a machine, as the main machine needs it to link every machine to every other.
type T3State struct {
	// Installed: the `t3` command is here. App: T3 Code has run here, maybe only as the desktop app.
	Installed     bool   `json:"installed"`
	App           bool   `json:"app"`
	Port          int    `json:"port,omitempty"`
	EnvironmentID string `json:"environment_id,omitempty"`
	// Address is where your devices open this T3 Code on the tailnet.
	Address   string `json:"address,omitempty"`
	LinkingOn bool   `json:"linking_on,omitempty"`
	// NetworkAccess: other machines can reach this T3; the desktop app answers only itself until it's on.
	NetworkAccess bool `json:"network_access"`
	// AwaitsRestart: Network access is on, but T3 Code hasn't restarted since, so it still answers only itself.
	AwaitsRestart bool   `json:"awaits_restart,omitempty"`
	Linked        string `json:"linked,omitempty"`
	Problem       string `json:"problem,omitempty"`
}

type desktopLink struct {
	Link string `json:"link"`
}

func (s *Server) serveT3Desktop(w http.ResponseWriter, r *http.Request) {
	var req mainRequest
	if !decodeBody(w, r, &req) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	link, err := s.b.T3DesktopLink(r.Context(), req.Machine)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, desktopLink{Link: link})
}

type pairRequest struct {
	Label string `json:"label"`
}

type pairAnswer struct {
	Code string `json:"code"`
}

type linkRequest struct {
	URL  string `json:"url"`
	Code string `json:"code"`
}

func (s *Server) serveT3(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.b.T3State(r.Context()))
}

func (s *Server) serveT3Phones(w http.ResponseWriter, r *http.Request) {
	phones, err := s.b.T3Phones(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "Couldn't read paired devices from T3 Code.")
		return
	}
	if phones == nil {
		phones = []t3code.Phone{}
	}
	writeJSON(w, struct {
		Phones []t3code.Phone `json:"phones"`
	}{phones})
}

func (s *Server) serveT3Pair(w http.ResponseWriter, r *http.Request) {
	var req pairRequest
	if !decodeBody(w, r, &req) {
		return
	}
	code, err := s.b.T3Pair(r.Context(), req.Label)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, pairAnswer{Code: code})
}

func (s *Server) serveT3Link(w http.ResponseWriter, r *http.Request) {
	var req linkRequest
	if !decodeBody(w, r, &req) {
		return
	}
	answer(w, s.b.T3Link(r.Context(), req.URL, req.Code), "Linked.")
}

func (s *Server) serveT3Setup(w http.ResponseWriter, r *http.Request) {
	answer(w, s.b.T3Setup(r.Context()), "T3 Code starts at boot now, with linking on. Spillup links your online Tailnet machines and retries when others come online.")
}

func (s *Server) serveT3Linking(w http.ResponseWriter, _ *http.Request) {
	answer(w, s.b.T3EnableLinking(), "Linked environments are on.")
}

func (s *Server) serveT3Sync(w http.ResponseWriter, r *http.Request) {
	summary, err := s.b.SyncT3(r.Context())
	answer(w, err, summary)
}

// Update is this machine's spillup version, the newest release, and whether the service installs it by itself.
type Update struct {
	Current string `json:"current"`
	Latest  string `json:"latest,omitempty"`
	Auto    bool   `json:"auto"`
	// System are the system packages waiting to update, where the system's package manager owns
	// Tailscale, Sunshine and Moonlight (SystemManaged).
	System        []string `json:"system,omitempty"`
	SystemManaged bool     `json:"system_managed,omitempty"`
}

type autoRequest struct {
	Auto bool `json:"auto"`
}

func (s *Server) serveUpdateState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.b.UpdateState(r.Context()))
}

func (s *Server) serveUpdate(w http.ResponseWriter, _ *http.Request) {
	answer(w, s.b.StartUpdate(), "Updating. spillup restarts in a minute; this page reconnects by itself.")
}

func (s *Server) serveAutoUpdate(w http.ResponseWriter, r *http.Request) {
	var req autoRequest
	if !decodeBody(w, r, &req) {
		return
	}
	msg := "spillup now updates itself when a new version is out."
	if !req.Auto {
		msg = "Automatic updates are off. Update from here when you like."
	}
	answer(w, s.b.SetAutoUpdate(req.Auto), msg)
}

func (s *Server) serveRestartService(w http.ResponseWriter, _ *http.Request) {
	answer(w, s.b.RestartService(), "Restarting spillup. This page reconnects in a few seconds.")
}

func (s *Server) serveUpdateSystem(w http.ResponseWriter, _ *http.Request) {
	answer(w, s.b.UpdateSystem(), "Updating the system…")
}

type RolloutRequest struct {
	Version string `json:"version"`
}

func (s *Server) serveRollout(w http.ResponseWriter, r *http.Request) {
	var req RolloutRequest
	if !decodeBody(w, r, &req) {
		return
	}
	answer(w, s.b.StartRollout(r.Context(), req.Version), "Staging the verified Spillup release.")
}

func (s *Server) serveRolloutPlan(w http.ResponseWriter, r *http.Request) {
	plan, err := s.b.RolloutPlan(r.Context())
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, plan)
}
