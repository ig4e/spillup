package webui

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/api"
)

// Remote is another machine's dashboard: its service's address on the tailnet, and the keys to send.
type Remote struct {
	Base   *url.URL
	Header http.Header
}

var dashboardTransport = &http.Transport{
	DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	ResponseHeaderTimeout: 75 * time.Second,
	IdleConnTimeout:       60 * time.Second,
	MaxIdleConnsPerHost:   8,
}

var dashboardClient = &http.Client{
	Transport:     dashboardTransport,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func stripDashboardHeaders(h http.Header) {
	for _, name := range []string{"Cookie", "Origin", "Referer", "Authorization", "X-Management-Key", api.KeyHeader, api.APIKeyHeader, panelHeader} {
		h.Del(name)
	}
}

// serveRemote passes a call to another machine's dashboard, with this machine's keys and none of the
// browser's cookies, so the main machine's dashboard can show any machine.
func (s *Server) serveRemote(w http.ResponseWriter, r *http.Request) {
	rm, err := s.b.Remote(r.PathValue("machine"))
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	route := r.PathValue("path")
	if route == "" || path.Clean(route) != route || route == ".." || strings.HasPrefix(route, "../") || strings.Contains(route, "\\") || strings.HasPrefix(route, "/") {
		writeError(w, http.StatusBadRequest, "invalid dashboard route")
		return
	}
	if !strings.HasPrefix(route, "term/") || !strings.HasSuffix(route, "/out") {
		ctx, cancel := context.WithTimeout(r.Context(), 80*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
	}
	dest := BridgePrefix + "/api/" + route
	proxy := &httputil.ReverseProxy{
		Transport: dashboardTransport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme, pr.Out.URL.Host, pr.Out.Host = rm.Base.Scheme, rm.Base.Host, ""
			pr.Out.URL.Path, pr.Out.URL.RawPath = dest, ""
			stripDashboardHeaders(pr.Out.Header)
			for k, v := range rm.Header {
				pr.Out.Header[k] = v
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("Set-Cookie")
			resp.Header.Del("Clear-Site-Data")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeError(w, http.StatusBadGateway, r.PathValue("machine")+" didn't answer: "+err.Error())
		},
	}
	proxy.ServeHTTP(w, r)
}
