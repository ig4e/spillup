package webui

import (
	"context"
	"crypto/rand"
	"log"
	"net/http"
	"time"

	"github.com/darkyeg/spinup/internal/history"
)

func (s *Server) serveActivity(w http.ResponseWriter, _ *http.Request) {
	items, err := history.Local().Actions()
	historyResponse(w, items, err)
}

func (s *Server) serveJobs(w http.ResponseWriter, _ *http.Request) {
	items, err := history.Local().Jobs()
	historyResponse(w, items, err)
}

func (s *Server) serveHealthTimeline(w http.ResponseWriter, _ *http.Request) {
	items, err := history.Local().Health()
	historyResponse(w, items, err)
}

func historyResponse(w http.ResponseWriter, items any, err error) {
	if err != nil {
		log.Printf("dashboard history: %v", err)
		writeError(w, http.StatusInternalServerError, "Couldn't read this machine's saved history.")
		return
	}
	writeJSON(w, items)
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

type actionKey struct{}

func actionID(r *http.Request) string {
	id, _ := r.Context().Value(actionKey{}).(string)
	return id
}

func trackAction(from reach, route string, next http.HandlerFunc) http.HandlerFunc {
	origin := [...]string{"local dashboard", "another machine", "Tailnet dashboard"}[from]
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		out := &activityResponse{ResponseWriter: w}
		id := rand.Text()
		next(out, r.WithContext(context.WithValue(r.Context(), actionKey{}, id)))
		status := out.status
		if status == 0 {
			status = http.StatusOK
		}
		entry := history.Action{ID: id, At: start, Action: route, Origin: origin, Status: status, ElapsedMS: time.Since(start).Milliseconds()}
		if err := history.Local().RecordAction(entry); err != nil {
			log.Printf("dashboard action history: %v", err)
		}
	}
}
