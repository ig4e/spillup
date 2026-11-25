package history

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/darkyeg/spinup/internal/filelock"
)

func TestActionsPersistPrivatelyAcrossWritersAndStayBounded(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	for i := range 99 {
		if err := s.RecordAction(Action{Status: i}); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for i := range 5 {
		wg.Go(func() {
			if err := (Store{Dir: s.Dir}).RecordAction(Action{Status: 200 + i}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	actions, err := (Store{Dir: s.Dir}).Actions()
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 100 || actions[99].Status != 4 {
		t.Fatalf("wrong retained range: %d records, last %+v", len(actions), actions[len(actions)-1])
	}
	seen := map[int]bool{}
	for _, a := range actions[:5] {
		seen[a.Status] = true
	}
	for n := range 5 {
		if !seen[200+n] {
			t.Fatalf("lost concurrent action %d", 200+n)
		}
	}
	fi, err := os.Stat(s.path("actions"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", fi.Mode())
	}
}

func TestJobCheckpointSurvivesLossOfTaskLock(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	lock, err := filelock.Acquire(context.Background(), filepath.Join(s.Dir, "task.lock"))
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.BeginJob("setup")
	if err != nil {
		t.Fatal(err)
	}
	j.Step, j.Total = 2, 5
	if err := s.UpdateJob(j); err != nil {
		t.Fatal(err)
	}
	jobs, err := s.Jobs()
	if err != nil || len(jobs) != 1 || jobs[0].State != "running" {
		t.Fatalf("live job %+v, %v", jobs, err)
	}
	lock.Close()
	jobs, err = (Store{Dir: s.Dir}).Jobs()
	if err != nil || len(jobs) != 1 || jobs[0].State != "interrupted" || jobs[0].Step != 2 {
		t.Fatalf("lost job %+v, %v", jobs, err)
	}
	if err := s.UpdateJob(j); err != nil {
		t.Fatal(err)
	}
	jobs, _ = s.Jobs()
	if jobs[0].State != "interrupted" {
		t.Fatal("a stale writer resurrected the job")
	}
}

func TestHealthRetainsTransitionsAndLastObservation(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	now := time.Now().UTC()
	sig := Signal{Machine: "test-device", Check: "Spillup", State: "Responding"}
	observe := func(at time.Time, state string) {
		sig.State = state
		if err := s.Observe(at, []Signal{sig}); err != nil {
			t.Fatal(err)
		}
	}
	observe(now, "Responding")
	observe(now.Add(5*time.Minute), "Responding")
	observe(now.Add(6*time.Minute), "No response")
	more, err := (Store{Dir: s.Dir}).Health()
	if err != nil || len(more) != 2 || more[0].State != "No response" || !more[1].Last.Equal(now.Add(5*time.Minute)) || !more[1].Since.Equal(now) {
		t.Fatalf("transitions: %+v, %v", more, err)
	}
}
