package webui

import "net/http"

// SetupProgress is a `spinup setup` the dashboard started: whether it still runs, and what it printed.
type SetupProgress struct {
	Running bool     `json:"running"`
	Done    bool     `json:"done"`
	Failed  string   `json:"failed,omitempty"`
	Log     []string `json:"log"`
}

type fixRequest struct {
	Fix string `json:"fix"`
}

type setupRequest struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

func (s *Server) serveFix(w http.ResponseWriter, r *http.Request) {
	var req fixRequest
	if !decodeBody(w, r, &req) {
		return
	}
	done, err := s.b.RunFix(r.Context(), req.Fix)
	answer(w, err, done)
}

func (s *Server) serveSetup(w http.ResponseWriter, r *http.Request) {
	var req setupRequest
	if !decodeBody(w, r, &req) {
		return
	}
	answer(w, s.b.StartSetup(req.Name, req.Role), "Setting up this machine…")
}

func (s *Server) serveSetupProgress(w http.ResponseWriter, _ *http.Request) {
	p := s.b.SetupProgress()
	if p.Log == nil {
		p.Log = []string{}
	}
	writeJSON(w, p)
}
