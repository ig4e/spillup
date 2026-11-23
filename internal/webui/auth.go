package webui

import (
	"crypto/subtle"
	"mime"
	"net"
	"net/http"
	"strings"
)

const (
	sessionCookie = "spinup_session"
	tokenParam    = "token"
)

// guard admits only requests addressed to this machine's loopback by name, so a web page that rebinds
// its own name to 127.0.0.1 can't reach the API, and only state-changing calls a page can't forge.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host) {
			http.Error(w, "spinup's dashboard answers only on localhost", http.StatusMisdirectedRequest)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOrigin(r) {
			writeError(w, http.StatusForbidden, "this request didn't come from spinup's dashboard")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// signIn trades the token in the link `spinup ui` opened for a cookie, then drops it from the address bar.
func (s *Server) signIn(w http.ResponseWriter, r *http.Request) bool {
	token := r.URL.Query().Get(tokenParam)
	if token == "" {
		return false
	}
	if !s.validToken(token) {
		http.Error(w, "this link is out of date: run `spinup ui` again", http.StatusUnauthorized)
		return true
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		MaxAge: 30 * 24 * 60 * 60,
	})
	clean := *r.URL
	q := clean.Query()
	q.Del(tokenParam)
	clean.RawQuery = q.Encode()
	http.Redirect(w, r, clean.RequestURI(), http.StatusSeeOther)
	return true
}

func (s *Server) signedIn(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	return err == nil && s.validToken(c.Value)
}

func (s *Server) validToken(got string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}

func (s *Server) needsSession(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.signedIn(r) {
			writeError(w, http.StatusUnauthorized, "open the dashboard with `spinup ui` to sign in")
			return
		}
		h(w, r)
	}
}

func loopbackHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// sameOrigin needs a JSON body, which a cross-site form can't send without asking first, and an Origin,
// when the browser sends one, that is this dashboard.
func sameOrigin(r *http.Request) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return false
	}
	origin := r.Header.Get("Origin")
	return origin == "" || origin == "http://"+r.Host
}
