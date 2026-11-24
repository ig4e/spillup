package webui

import "net/http"

// T3State is T3 Code on a machine, as the main machine needs it to link every machine to every other.
type T3State struct {
	// Installed: the `t3` command is here. App: T3 Code has run here, maybe only as the desktop app.
	Installed bool   `json:"installed"`
	App       bool   `json:"app"`
	Port      int    `json:"port,omitempty"`
	LinkingOn bool   `json:"linking_on,omitempty"`
	Linked    string `json:"linked,omitempty"`
	Problem   string `json:"problem,omitempty"`
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
	answer(w, s.b.T3Setup(r.Context()), "T3 Code starts at boot now, with linking on. The main machine links it to your other machines within a few minutes.")
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
