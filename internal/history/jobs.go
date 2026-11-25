package history

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"time"

	"github.com/darkyeg/spinup/internal/filelock"
)

type Job struct {
	ActionID string    `json:"action_id,omitempty"`
	Started  time.Time `json:"started"`
	Updated  time.Time `json:"updated"`
	Task     string    `json:"task"`
	State    string    `json:"state"`
	Step     int       `json:"step,omitempty"`
	Total    int       `json:"total,omitempty"`
}

func (s Store) BeginJob(task, actionID string) (Job, error) {
	now := time.Now().UTC()
	j := Job{ActionID: actionID, Started: now, Updated: now, Task: task, State: "running"}
	err := edit(s.path("jobs"), 50, func(jobs []Job) ([]Job, bool) {
		interrupt(jobs)
		return append(jobs, j), true
	})
	return j, err
}

func (s Store) UpdateJob(j Job) error {
	return edit(s.path("jobs"), 50, func(jobs []Job) ([]Job, bool) {
		for i := len(jobs) - 1; i >= 0; i-- {
			if jobs[i].Started.Equal(j.Started) && jobs[i].State == "running" {
				j.Updated = time.Now().UTC()
				jobs[i] = j
				return jobs, true
			}
		}
		return jobs, false
	})
}

func interrupt(jobs []Job) bool {
	changed := false
	for i := range jobs {
		if jobs[i].State == "running" {
			jobs[i].State = "interrupted"
			changed = true
		}
	}
	return changed
}

// A running job keeps task.lock through its final write. Its last recorded step survives a lost process.
func (s Store) Jobs() ([]Job, error) {
	path := s.path("jobs")
	jobs, err := recent[Job](path)
	if err != nil || !slices.ContainsFunc(jobs, func(j Job) bool { return j.State == "running" }) {
		return jobs, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	lock, err := filelock.Acquire(ctx, filepath.Join(s.Dir, "task.lock"))
	if errors.Is(err, context.DeadlineExceeded) {
		return jobs, nil
	}
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if err := edit(path, 50, func(list []Job) ([]Job, bool) { return list, interrupt(list) }); err != nil {
		return nil, err
	}
	return recent[Job](path)
}
