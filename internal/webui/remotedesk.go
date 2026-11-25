package webui

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"

	"github.com/darkyeg/spinup/internal/remoteweb"
)

// RemoteState is remote desktop on a machine: Sunshine to be reached, Moonlight and moonlight-web to reach
// the others, and the hosts moonlight-web knows.
type RemoteState struct {
	SunshineInstalled  bool         `json:"sunshine_installed"`
	SunshineRunning    bool         `json:"sunshine_running"`
	MoonlightInstalled bool         `json:"moonlight_installed"`
	WebInstalled       bool         `json:"web_installed"`
	WebRunning         bool         `json:"web_running"`
	Problem            string       `json:"problem,omitempty"`
	Hosts              []RemoteHost `json:"hosts"`
}

// RemoteHost is a machine moonlight-web can stream.
type RemoteHost struct {
	ID      uint32 `json:"id"`
	Name    string `json:"name"`
	Machine string `json:"machine,omitempty"`
	Paired  bool   `json:"paired"`
	State   string `json:"state,omitempty"`
}

type pinRequest struct {
	Pin  string `json:"pin"`
	Name string `json:"name"`
}

func (s *Server) serveRemoteState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.b.RemoteState(r.Context()))
}

// serveSunshinePin confirms the PIN of the client waiting to pair from whoever asks: another machine's
// spillup over the tailnet, or Moonlight on this one.
func (s *Server) serveSunshinePin(w http.ResponseWriter, r *http.Request) {
	var req pinRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.Pin) != 4 || strings.Trim(req.Pin, "0123456789") != "" {
		writeError(w, http.StatusBadRequest, "the PIN is the 4 digits Moonlight shows")
		return
	}
	if req.Name == "" {
		req.Name = "Moonlight"
	}
	from, _, _ := net.SplitHostPort(r.RemoteAddr)
	answer(w, s.b.SunshinePIN(r.Context(), req.Pin, req.Name, from), "Paired.")
}

func (s *Server) serveRemoteSync(w http.ResponseWriter, r *http.Request) {
	summary, err := s.b.RemoteSync(r.Context())
	answer(w, err, summary)
}

func (s *Server) serveRemoteSetup(w http.ResponseWriter, _ *http.Request) {
	answer(w, s.b.RemoteSetup(), "Setting up remote desktop…")
}

// serveRemoteWeb gives the native player same-origin access to the local stream transport.
func (s *Server) serveRemoteWeb(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, remoteweb.Prefix)
	if r.Method == http.MethodGet && (path == "/" || path == "/index.html" || path == "/stream.html") {
		dest := "/desktop"
		if id, err := strconv.ParseUint(r.URL.Query().Get("hostId"), 10, 32); err == nil {
			dest = fmt.Sprintf("/desktop?hostId=%d", id)
		}
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}
	if !streamRoute(r.Method, path) {
		http.NotFound(w, r)
		return
	}
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") && r.Header.Get("Origin") != "http://"+r.Host {
		writeError(w, http.StatusForbidden, "the stream must open from this dashboard")
		return
	}
	target := fmt.Sprintf("127.0.0.1:%d", s.b.RemoteWebPort())
	proxy := &httputil.ReverseProxy{
		Transport:     dashboardTransport,
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme, pr.Out.URL.Host, pr.Out.Host = "http", target, ""
			stripDashboardHeaders(pr.Out.Header)
			pr.Out.Header.Set(remoteweb.UserHeader, remoteweb.User)
		},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("Set-Cookie")
			resp.Header.Del("Clear-Site-Data")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeError(w, http.StatusBadGateway, "moonlight-web isn't running: set remote desktop up first ("+err.Error()+")")
		},
	}
	proxy.ServeHTTP(w, r)
}

func isRemoteWeb(path string) bool {
	return path == remoteweb.Prefix || strings.HasPrefix(path, remoteweb.Prefix+"/")
}

func streamRoute(method, path string) bool {
	const webrtc = "/api/host/stream/webrtc"
	switch path {
	case "/api/apps", "/api/hosts", "/api/host/stream/web_socket":
		return method == http.MethodGet
	case webrtc:
		return method == http.MethodOptions || method == http.MethodPost
	}
	if id, ok := strings.CutPrefix(path, webrtc+"/"); ok && id != "" {
		_, err := strconv.ParseUint(id, 10, 64)
		return err == nil && (method == http.MethodPatch || method == http.MethodDelete)
	}
	return false
}
