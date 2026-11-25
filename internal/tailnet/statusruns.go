package tailnet

import (
	"sync"
	"time"
)

// statusRun is one `tailscale status` run that every caller asking meanwhile waits for.
type statusRun struct {
	done chan struct{}
	st   Status
	err  error
}

// reuseWithin is how long a good reading answers every caller, so a busy machine runs the CLI once per moment
// instead of once per caller; it is small next to the leader's fence budget.
const reuseWithin = 2 * time.Second

type statusRunner struct {
	mu   sync.Mutex
	runs map[string]*statusRun
	good map[string]goodReading
}

type goodReading struct {
	at time.Time
	st Status
}

var statusRuns = &statusRunner{runs: map[string]*statusRun{}, good: map[string]goodReading{}}

// recent is the last good reading for bin, if it is younger than reuseWithin.
func (r *statusRunner) recent(bin string) (Status, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.good[bin]
	return g.st, ok && time.Since(g.at) < reuseWithin
}

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
	if run.err == nil {
		r.good[bin] = goodReading{time.Now(), run.st}
	}
	r.mu.Unlock()
	close(run.done)
}

func (r *statusRunner) forget() {
	r.mu.Lock()
	defer r.mu.Unlock()
	clear(r.good)
}
