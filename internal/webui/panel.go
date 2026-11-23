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
	"time"
)

// Panel is a CLIProxyAPI whose own dashboard (the Management Center) the dashboard shows: the shared
// accounts' proxy, or this machine's own.
type Panel struct {
	Port int
	Key  string
}

const (
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
	p, err := s.b.Panel(target)
	if err != nil {
		writePanelProblem(w, err)
		return
	}
	page, err := fetchPanelPage(r.Context(), p)
	if err != nil {
		writePanelProblem(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: panelCookie, Value: target, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
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
	if c, err := r.Cookie(panelCookie); err == nil && c.Value == panelOwn {
		target = panelOwn
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
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeError(w, http.StatusBadGateway, "CLIProxyAPI didn't answer: "+err.Error())
		},
	}
	proxy.ServeHTTP(w, r)
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
