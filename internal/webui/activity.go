package webui

import (
	"net/http"
	"slices"
	"sync"
	"time"
)

type activityEntry struct {
	At        time.Time `json:"at"`
	Action    string    `json:"action"`
	Origin    string    `json:"origin"`
	Status    int       `json:"status"`
	ElapsedMS int64     `json:"elapsed_ms"`
}

var dashboardActivity struct {
	sync.Mutex
	entries []activityEntry
}

func (s *Server) serveActivity(w http.ResponseWriter, _ *http.Request) {
	dashboardActivity.Lock()
	entries := slices.Clone(dashboardActivity.entries)
	dashboardActivity.Unlock()
	slices.Reverse(entries)
	if entries == nil {
		entries = []activityEntry{}
	}
	writeJSON(w, entries)
}

type activityResponse struct {
	http.ResponseWriter
	status int
}

func (w *activityResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *activityResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *activityResponse) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func trackAction(from reach, route string, next http.HandlerFunc) http.HandlerFunc {
	origin := [...]string{"local dashboard", "another machine", "Tailnet dashboard"}[from]
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		out := &activityResponse{ResponseWriter: w}
		next(out, r)
		entry := activityEntry{At: start, Action: route, Origin: origin, Status: out.status, ElapsedMS: time.Since(start).Milliseconds()}
		dashboardActivity.Lock()
		defer dashboardActivity.Unlock()
		const limit = 100
		if len(dashboardActivity.entries) == limit {
			copy(dashboardActivity.entries, dashboardActivity.entries[1:])
			dashboardActivity.entries = dashboardActivity.entries[:limit-1]
		}
		dashboardActivity.entries = append(dashboardActivity.entries, entry)
	}
}
