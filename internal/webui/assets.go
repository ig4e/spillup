package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// dist is the built dashboard (web/, `npm run build`); a binary built without it serves notBuilt.
//
//go:embed all:dist
var dist embed.FS

const notBuilt = `<!doctype html><meta charset="utf-8"><title>spinup</title>
<body style="font-family:system-ui;max-width:40rem;margin:4rem auto;line-height:1.5">
<h1>The dashboard isn't in this build</h1>
<p>This spinup was built without its web dashboard. Build it with <code>npm --prefix web ci && npm --prefix web run build</code>,
then <code>go build ./cmd/spinup</code>, or install a release build.</p>
<p>Everything still works from the command line: <code>spinup --help</code>.</p>`

func assets() fs.FS {
	sub, _ := fs.Sub(dist, "dist")
	return sub
}

// serveApp serves a built file, and the app's page for every other path so its own routes work on reload.
func (s *Server) serveApp(w http.ResponseWriter, r *http.Request) {
	if s.signIn(w, r) {
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name != "" && name != "index.html" {
		if f, err := fs.Stat(s.assets, name); err == nil && !f.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.ServeFileFS(w, r, s.assets, name)
			return
		}
	}
	page, err := fs.ReadFile(s.assets, "index.html")
	if err != nil {
		page = []byte(notBuilt)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(page)
}
