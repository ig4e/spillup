package webui

import "net/http"

type ClipboardText struct {
	Text string `json:"text"`
}

func (s *Server) serveRemoteClipboard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		HostID uint32 `json:"host_id"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	value, err := s.b.RemoteClipboard(r.Context(), req.HostID)
	writeClipboard(w, value, err)
}

func (s *Server) serveHostClipboard(w http.ResponseWriter, r *http.Request) {
	value, err := s.b.HostClipboard(r.Context(), r.RemoteAddr)
	writeClipboard(w, value, err)
}

func writeClipboard(w http.ResponseWriter, value ClipboardText, err error) {
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, value)
}
