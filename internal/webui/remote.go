package webui

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

// Remote is another machine's dashboard: its service's address on the tailnet, and the keys to send.
type Remote struct {
	Base   *url.URL
	Header http.Header
}

// serveRemote passes a call to another machine's dashboard, with this machine's keys and none of the
// browser's cookies, so the main machine's dashboard can show any machine.
func (s *Server) serveRemote(w http.ResponseWriter, r *http.Request) {
	rm, err := s.b.Remote(r.PathValue("machine"))
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	path := BridgePrefix + "/api/" + r.PathValue("path")
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme, pr.Out.URL.Host, pr.Out.Host = rm.Base.Scheme, rm.Base.Host, ""
			pr.Out.URL.Path, pr.Out.URL.RawPath = path, ""
			for _, h := range []string{"Cookie", "Origin", "Referer"} {
				pr.Out.Header.Del(h)
			}
			for k, v := range rm.Header {
				pr.Out.Header[k] = v
			}
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeError(w, http.StatusBadGateway, r.PathValue("machine")+" didn't answer: "+err.Error())
		},
	}
	proxy.ServeHTTP(w, r)
}
