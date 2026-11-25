package webui

import (
	"context"
	"net/http"
	"strings"
)

type MCPSecretRequest struct {
	Server  string `json:"server"`
	Key     string `json:"key"`
	Machine string `json:"machine,omitempty"`
	Value   string `json:"value,omitempty"`
}

type MCPShareRef struct {
	Server string `json:"server"`
	Key    string `json:"key"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type MCPTransfer struct {
	MCPShareRef
	Permit string `json:"permit"`
	Value  []byte `json:"value"`
}

type mcpSecretBackend interface {
	MCPSecret(context.Context, string, MCPSecretRequest) error
	MCPPermit(context.Context, string, MCPShareRef) (string, error)
	MCPReceive(context.Context, string, MCPTransfer) error
}

func privateMCPRoute(path string) bool {
	return path == "mcp" || strings.HasPrefix(path, "mcp/") || strings.HasPrefix(path, "host/mcp/")
}

func (s *Server) mcpSecretRoutes(from reach, route func(string, string, http.HandlerFunc)) {
	switch from {
	case browser:
		for _, op := range []string{"secret", "forget", "allow", "share"} {
			route(http.MethodPost, "mcp/"+op, func(w http.ResponseWriter, r *http.Request) {
				var req MCPSecretRequest
				if !decodeBody(w, r, &req) {
					return
				}
				err := s.b.MCPSecret(r.Context(), op, req)
				messages := map[string]string{"secret": "Secret saved on this machine.", "forget": "Secret removed and approvals cancelled.", "allow": "Approved for ten minutes.", "share": "Secret received by the selected machine."}
				answer(w, err, messages[op])
			})
		}
	case bridged:
		route(http.MethodPost, "host/mcp/permit", func(w http.ResponseWriter, r *http.Request) {
			var req MCPShareRef
			if !decodeBody(w, r, &req) {
				return
			}
			permit, err := s.b.MCPPermit(r.Context(), r.RemoteAddr, req)
			if err != nil {
				writeError(w, http.StatusForbidden, "No matching transfer approval.")
				return
			}
			writeJSON(w, struct {
				Permit string `json:"permit"`
			}{permit})
		})
		route(http.MethodPost, "host/mcp/receive", func(w http.ResponseWriter, r *http.Request) {
			var req MCPTransfer
			if !decodeBody(w, r, &req) {
				return
			}
			if err := s.b.MCPReceive(r.Context(), r.RemoteAddr, req); err != nil {
				writeError(w, http.StatusForbidden, "The transfer was declined.")
				return
			}
			writeJSON(w, Done{Message: "Received."})
		})
	}
}
