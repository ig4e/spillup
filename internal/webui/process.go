package webui

import (
	"net/http"

	"github.com/darkyeg/spinup/internal/sysinfo"
)

type endRequest struct {
	PID   int  `json:"pid"`
	Force bool `json:"force"`
}

func (s *Server) serveEndProcess(w http.ResponseWriter, r *http.Request) {
	var req endRequest
	if !decodeBody(w, r, &req) {
		return
	}
	done := "Asked it to quit."
	if req.Force {
		done = "Ended it."
	}
	answer(w, sysinfo.End(req.PID, req.Force), done)
}
