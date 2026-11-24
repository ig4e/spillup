package webui

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/darkyeg/spinup/internal/tailnet"
)

// tailnetView keeps the tailnet as Tailscale last reported it, for a few seconds, so each request from a
// device doesn't run the tailscale command.
type tailnetView struct {
	mu   sync.Mutex
	at   time.Time
	last tailnet.Status
	ok   bool
}

const tailnetViewFor = 15 * time.Second

func (v *tailnetView) get(ctx context.Context, read func(context.Context) (tailnet.Status, error)) (tailnet.Status, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if time.Since(v.at) < tailnetViewFor {
		return v.last, v.ok
	}
	st, err := read(ctx)
	v.at, v.last, v.ok = time.Now(), st, err == nil && st.Self.IP != ""
	return v.last, v.ok
}

// yourDevice reports whether ip is this machine, or a device Tailscale says is signed in to its owner's account.
func yourDevice(st tailnet.Status, ip string) bool {
	if ip == st.Self.IP {
		return true
	}
	peer, ok := st.PeerByIP(ip)
	return ok && st.SameOwner(peer)
}

// ownHost reports whether a request's Host names this machine on the tailnet, so a page that points its own
// name at this address can't reach the dashboard.
func ownHost(st tailnet.Status, hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	self := st.Self
	return host == self.IP || host == strings.ToLower(self.Name) || host == strings.TrimSuffix(strings.ToLower(self.DNSName), ".")
}

type accessRequest struct {
	Devices   bool `json:"device_access"`
	Terminals bool `json:"terminals"`
}

func (s *Server) serveAccess(w http.ResponseWriter, r *http.Request) {
	var req accessRequest
	if !decodeBody(w, r, &req) {
		return
	}
	answer(w, s.b.SetAccess(req.Devices, req.Terminals), "Saved.")
}

// DeviceHandler is the dashboard for your other devices, a phone too, on the tailnet. Tailscale vouches for
// who's asking, so it needs no sign-in, but it answers only devices signed in to your own Tailscale account;
// keys, sign-ins and setup stay on this machine.
func (s *Server) DeviceHandler() http.Handler {
	pass := func(h http.HandlerFunc) http.HandlerFunc { return h }
	return s.deviceGuard(securityHeaders(s.appMux(device, pass)))
}

func (s *Server) deviceGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st, ok := s.tailnet.get(r.Context(), s.b.Tailnet)
		if !ok || !ownHost(st, r.Host) {
			http.Error(w, "open this machine's dashboard by its Tailscale name", http.StatusMisdirectedRequest)
			return
		}
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if !yourDevice(st, ip) {
			http.Error(w, "spillup's dashboard answers only devices signed in to your Tailscale account", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOrigin(r) {
			writeError(w, http.StatusForbidden, "this request didn't come from spinup's dashboard")
			return
		}
		next.ServeHTTP(w, r)
	})
}
