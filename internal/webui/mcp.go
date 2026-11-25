package webui

import (
	"net/http"

	"github.com/darkyeg/spinup/internal/mcp"
)

func (s *Server) mcpRoutes(read, change func(string, http.HandlerFunc)) {
	read("mcp", func(w http.ResponseWriter, r *http.Request) {
		data, err := s.b.MCP().List()
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, data)
	})
	change("mcp/save", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			mcp.Definition
			Sharing string `json:"sharing"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		if in.Sharing == "shared" {
			s.b.CatchUpLibrary()
		}
		answer(w, s.b.MCP().Save(r.Context(), in.Definition, in.Sharing), "MCP definition saved.")
	})
	change("mcp/remove", func(w http.ResponseWriter, r *http.Request) {
		var in nameRequest
		if !decodeBody(w, r, &in) {
			return
		}
		s.b.CatchUpLibrary()
		answer(w, s.b.MCP().Remove(r.Context(), in.Name), "MCP definition removed.")
	})
	change("mcp/sharing", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Name, Action, Revision string }
		if !decodeBody(w, r, &in) {
			return
		}
		s.b.CatchUpLibrary()
		answer(w, s.b.MCP().Sharing(r.Context(), in.Name, in.Action, in.Revision), "Sharing updated.")
	})
	change("mcp/target", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name, Target, Revision string
			Enabled                *bool
		}
		if !decodeBody(w, r, &in) {
			return
		}
		if in.Enabled == nil {
			writeError(w, http.StatusBadRequest, "choose enable or disable")
			return
		}
		answer(w, s.b.MCP().Target(r.Context(), in.Name, in.Target, in.Revision, *in.Enabled), "Agent configuration updated.")
	})
}
