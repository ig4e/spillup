package webui

import (
	"net/http"

	"github.com/darkyeg/spinup/internal/config"
)

// Device is a device on the tailnet and whether it runs spillup, as a scan found it.
type Device struct {
	Name    string      `json:"name"`
	IP      string      `json:"ip"`
	OS      string      `json:"os,omitempty"`
	Online  bool        `json:"online"`
	Yours   bool        `json:"yours"`
	Spillup bool        `json:"spillup"`
	Hold    config.Hold `json:"hold,omitempty"`
}

type modeRequestBody struct {
	HubOnly bool `json:"hub_only"`
}

func (s *Server) serveScan(w http.ResponseWriter, r *http.Request) {
	devices, err := s.b.ScanDevices(r.Context())
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if devices == nil {
		devices = []Device{}
	}
	writeJSON(w, devices)
}

func (s *Server) serveAccountsMode(w http.ResponseWriter, r *http.Request) {
	var req modeRequestBody
	if !decodeBody(w, r, &req) {
		return
	}
	msg := "Standbys keep copies of the logins again, and take over while the main machine is off."
	if req.HubOnly {
		msg = "Only the main machine keeps the logins now. Standbys drop their copies and won't take over."
	}
	answer(w, s.b.SetHubOnly(req.HubOnly), msg)
}
