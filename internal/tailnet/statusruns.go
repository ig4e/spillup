package tailnet

import "sync"

// statusRun is one `tailscale status` run that every caller asking meanwhile waits for.
type statusRun struct {
	done chan struct{}
	st   Status
	err  error
}

type statusRunner struct {
	mu   sync.Mutex
	runs map[string]*statusRun
}

var statusRuns = &statusRunner{runs: map[string]*statusRun{}}

// join returns the run in flight for bin, or a new one that the caller (first) must perform and finish.
func (r *statusRunner) join(bin string) (run *statusRun, first bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run, ok := r.runs[bin]; ok {
		return run, false
	}
	run = &statusRun{done: make(chan struct{})}
	r.runs[bin] = run
	return run, true
}

func (r *statusRunner) finish(bin string, run *statusRun) {
	r.mu.Lock()
	delete(r.runs, bin)
	r.mu.Unlock()
	close(run.done)
}
