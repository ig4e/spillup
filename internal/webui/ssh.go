package webui

import (
	"net/http"

	"github.com/darkyeg/spinup/internal/sshd"
)

// SSHState is SSH on a machine: the server it runs, whether it's on, and who to sign in as with which key.
type SSHState struct {
	Server  sshd.Server `json:"server"`
	On      bool        `json:"on"`
	User    string      `json:"user"`
	Address string      `json:"address,omitempty"`
	Key     string      `json:"key,omitempty"`
	// Hosts are the machines `ssh <name>` reaches from here.
	Hosts   []string `json:"hosts"`
	Problem string   `json:"problem,omitempty"`
}

type sshRequest struct {
	On bool `json:"on"`
}

func (s *Server) serveSSH(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.b.SSHState(r.Context()))
}

func (s *Server) serveSSHEnable(w http.ResponseWriter, r *http.Request) {
	var req sshRequest
	if !decodeBody(w, r, &req) {
		return
	}
	answer(w, s.b.SSHEnable(req.On, actionID(r)), "Turning SSH "+map[bool]string{true: "on", false: "off"}[req.On]+"…")
}

func (s *Server) serveSSHSync(w http.ResponseWriter, r *http.Request) {
	summary, err := s.b.SSHSync(r.Context())
	answer(w, err, summary)
}
