package webui

import "net/http"

// SetupProgress is a `spinup setup` the dashboard started: whether it still runs, and what it printed.
type SetupProgress struct {
	Task           string   `json:"task,omitempty"`
	Phase          string   `json:"phase,omitempty"`
	Step           int      `json:"step,omitempty"`
	Total          int      `json:"total,omitempty"`
	ElapsedSeconds int      `json:"elapsed_seconds,omitempty"`
	QuietSeconds   int      `json:"quiet_seconds,omitempty"`
	Running        bool     `json:"running"`
	Done           bool     `json:"done"`
	Failed         string   `json:"failed,omitempty"`
	Log            []string `json:"log"`
	// Prompt is what setup asks while it waits for your password, such as sudo's question.
	Prompt string `json:"prompt,omitempty"`
}

type answerRequest struct {
	Password string `json:"password"`
	Cancel   bool   `json:"cancel"`
}

func (s *Server) serveSetupAnswer(w http.ResponseWriter, r *http.Request) {
	var req answerRequest
	if !decodeBody(w, r, &req) {
		return
	}
	answer(w, s.b.AnswerSetup(req.Password, req.Cancel), "Thanks.")
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
	answer(w, s.b.StartSetup(req.Name, req.Role, actionID(r)), "Setting up this machine…")
}

func (s *Server) serveSetupProgress(w http.ResponseWriter, _ *http.Request) {
	p := s.b.SetupProgress()
	if p.Log == nil {
		p.Log = []string{}
	}
	writeJSON(w, p)
}

func (s *Server) serveSetupPreview(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	preview, err := s.b.PreviewSetup(r.Context(), query.Get("name"), query.Get("role"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, preview)
}
