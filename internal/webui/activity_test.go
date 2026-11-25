package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkyeg/spinup/internal/history"
)

type setupHistoryBackend struct{ fakeBackend }

func (*setupHistoryBackend) StartSetup(_, _, id string) error {
	store := history.Local()
	job, err := store.BeginJob("setup", id)
	if err != nil {
		return err
	}
	job.State = "failed"
	return store.UpdateJob(job)
}

func TestActionLinksToItsJobThroughDashboard(t *testing.T) {
	t.Setenv("SPINUP_HOME", t.TempDir())
	h := New(&setupHistoryBackend{}, testToken).Handler()
	for range 2 {
		r := httptest.NewRequest(http.MethodPost, "/api/setup", strings.NewReader(`{"name":"desk","role":"use"}`))
		r.Host = "localhost:8320"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Request-ID", "untrusted")
		signedIn(r)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	actions, err := history.Local().Actions()
	if err != nil || len(actions) != 2 {
		t.Fatal(actions, err)
	}
	jobs, err := history.Local().Jobs()
	if err != nil || len(jobs) != 2 {
		t.Fatal(jobs, err)
	}
	if actions[0].ID == actions[1].ID {
		t.Fatal("distinct operations share an ID")
	}
	for i, a := range actions {
		if a.ID == "" || a.ID == "untrusted" || jobs[i].ActionID != a.ID || jobs[i].State != "failed" || a.Status != 200 {
			t.Fatalf("incorrect action/job association: %+v, %+v", a, jobs[i])
		}
	}
}
