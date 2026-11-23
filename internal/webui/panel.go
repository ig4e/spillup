package webui

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Panel is a CLIProxyAPI whose own dashboard (the Management Center) the dashboard shows: the shared
// accounts' proxy, or this machine's own.
type Panel struct {
	Port int
	Key  string
}

const (
	panelMachineCookie = "spinup_panel_machine"
	// panelHeader names the proxy when the main machine's dashboard calls this one's over the tailnet.
	panelHeader = "X-Spinup-Panel"
	panelCookie = "spinup_panel"
	panelShared = "shared"
	panelOwn    = "own"
)

// panelAPIPrefixes are where the Management Center calls its proxy: at the root of the page's origin.
var panelAPIPrefixes = []string{"/v0/management/", "/v8/management/"}

func isPanelAPI(path string) bool {
	for _, p := range panelAPIPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// panelSeed signs the Management Center in before it starts, with a placeholder key in its own storage
// format: the real key never reaches the browser, servePanelAPI adds it to every call.
const panelSeed = `<script>(function(){try{var e=new TextEncoder(),d=e.encode(JSON.stringify({state:{apiBase:location.origin,` +
	`managementKey:"spinup",rememberPassword:true,serverVersion:null,serverBuildDate:null},version:0})),` +
	`k=e.encode("cli-proxy-api-webui::secure-storage|"+location.host),b="";for(var i=0;i<d.length;i++)` +
	`b+=String.fromCharCode(d[i]^k[i%k.length]);localStorage.setItem("cli-proxy-auth","enc::v2::"+btoa(b));` +
	`localStorage.setItem("isLoggedIn","true")}catch(x){}})();</script>`

// servePanelPage serves a proxy's Management Center from this dashboard's origin, and remembers which
// proxy its calls go to.
func (s *Server) servePanelPage(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("target")
	machine := r.URL.Query().Get("machine")
	var page string
	var err error
	if machine != "" {
		page, err = s.fetchRemotePanelPage(r, machine, target)
	} else {
		var p Panel
		if p, err = s.b.Panel(target); err == nil {
			page, err = fetchPanelPage(r.Context(), p)
		}
	}
	if err != nil {
		writePanelProblem(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: panelCookie, Value: target, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	machineCookie := &http.Cookie{Name: panelMachineCookie, Value: machine, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode}
	if machine == "" {
		machineCookie.MaxAge = -1
	}
	http.SetCookie(w, machineCookie)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, seeded(page))
}

func seeded(page string) string {
	if i := strings.Index(page, "<script"); i >= 0 {
		return page[:i] + panelSeed + page[i:]
	}
	return panelSeed + page
}

func fetchPanelPage(ctx context.Context, p Panel) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, panelBase(p)+"/management.html", nil)
	resp, err := (&http.Client{Transport: &http.Transport{Proxy: nil}}).Do(req)
	if err != nil {
		return "", fmt.Errorf("CLIProxyAPI isn't answering: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("CLIProxyAPI didn't serve its dashboard (%s)", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	return string(body), err
}

func panelBase(p Panel) string {
	return "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(p.Port))
}

// servePanelAPI passes the Management Center's calls to the proxy it was opened for, with the key added
// here and this dashboard's cookies taken off.
func (s *Server) servePanelAPI(w http.ResponseWriter, r *http.Request) {
	target := panelShared
	if c, err := r.Cookie(panelCookie); err == nil && c.Value == panelOwn || r.Header.Get(panelHeader) == panelOwn {
		target = panelOwn
	}
	if c, err := r.Cookie(panelMachineCookie); err == nil && c.Value != "" {
		s.serveRemotePanelAPI(w, r, c.Value, target)
		return
	}
	if wait := s.refusals.wait(target, time.Now()); wait > 0 {
		writeError(w, http.StatusConflict, fmt.Sprintf("CLIProxyAPI refused spillup's key, so spillup stopped asking for %s to keep it from banning this machine. "+
			"Check that spillup runs this CLIProxyAPI (spinup doctor)", wait.Round(time.Second)))
		return
	}
	p, err := s.b.Panel(target)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	host := net.JoinHostPort("127.0.0.1", strconv.Itoa(p.Port))
	proxy := &httputil.ReverseProxy{
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme, pr.Out.URL.Host, pr.Out.Host = "http", host, ""
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del("Origin")
			pr.Out.Header.Del("Referer")
			pr.Out.Header.Set("Authorization", "Bearer "+p.Key)
			pr.Out.Header.Set("X-Management-Key", p.Key)
		},
		ModifyResponse: func(resp *http.Response) error {
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				s.refusals.refused(target, time.Now())
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeError(w, http.StatusBadGateway, "CLIProxyAPI didn't answer: "+err.Error())
		},
	}
	proxy.ServeHTTP(w, r)
}

// panelBackoff is how long spillup stops calling a CLIProxyAPI that refused its key. CLIProxyAPI bans an
// address after a few refusals, and the Management Center calls many endpoints at once.
const panelBackoff = 2 * time.Minute

// refusals remembers which CLIProxyAPI refused spillup's key, and when.
type refusals struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func (r *refusals) refused(target string, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.last == nil {
		r.last = map[string]time.Time{}
	}
	r.last[target] = now
}

// wait is how long calls to target must still wait, or zero.
func (r *refusals) wait(target string, now time.Time) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if left := r.last[target].Add(panelBackoff).Sub(now); left > 0 {
		return left
	}
	return 0
}

func writePanelProblem(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusConflict)
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><body style="font:15px -apple-system,system-ui,sans-serif;color:#6e6e73;`+
		`display:grid;place-items:center;height:90vh;margin:0;text-align:center"><div><div style="font-size:17px;color:#1d1d1f;`+
		`font-weight:600;margin-bottom:6px">CLIProxyAPI's dashboard isn't available</div>%s</div>`, htmlEscape(err.Error()))
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// fetchRemotePanelPage is another machine's Management Center, which that machine serves already signed in.
func (s *Server) fetchRemotePanelPage(r *http.Request, machine, target string) (string, error) {
	rm, err := s.b.Remote(machine)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, rm.Base.String()+BridgePrefix+"/cpa/"+target+"/", nil)
	for k, v := range rm.Header {
		req.Header[k] = v
	}
	resp, err := (&http.Client{Transport: &http.Transport{Proxy: nil}}).Do(req)
	if err != nil {
		return "", fmt.Errorf("%s didn't answer: %w", machine, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s couldn't show its CLIProxyAPI dashboard (%s)", machine, resp.Status)
	}
	return string(body), err
}

// serveRemotePanelAPI passes the Management Center's calls to the machine it was opened for, whose own
// dashboard adds its key.
func (s *Server) serveRemotePanelAPI(w http.ResponseWriter, r *http.Request, machine, target string) {
	rm, err := s.b.Remote(machine)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	proxy := &httputil.ReverseProxy{
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme, pr.Out.URL.Host, pr.Out.Host = rm.Base.Scheme, rm.Base.Host, ""
			pr.Out.URL.Path, pr.Out.URL.RawPath = BridgePrefix+r.URL.Path, ""
			for _, h := range []string{"Cookie", "Origin", "Referer", "Authorization", "X-Management-Key"} {
				pr.Out.Header.Del(h)
			}
			for k, v := range rm.Header {
				pr.Out.Header[k] = v
			}
			pr.Out.Header.Set(panelHeader, target)
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeError(w, http.StatusBadGateway, machine+" didn't answer: "+err.Error())
		},
	}
	proxy.ServeHTTP(w, r)
}
