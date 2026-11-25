package history

import (
	"time"

	"github.com/darkyeg/spinup/internal/tailnet"
)

type Signal struct {
	Machine string `json:"machine"`
	Check   string `json:"check"`
	State   string `json:"state"`
}

type Observation struct {
	Signal
	Since time.Time `json:"since"`
	Last  time.Time `json:"last"`
}

func (s Store) Observe(now time.Time, signals []Signal) error {
	if len(signals) == 0 {
		return nil
	}
	return edit(s.path("health"), 400, func(list []Observation) ([]Observation, bool) {
		type key struct{ machine, check string }
		latest := make(map[key]int, len(list))
		for i, obs := range list {
			latest[key{obs.Machine, obs.Check}] = i
		}
		changed := false
		for _, sig := range signals {
			if !tailnet.ValidName(sig.Machine) {
				continue
			}
			k := key{sig.Machine, sig.Check}
			if i, ok := latest[k]; ok && list[i].State == sig.State {
				if now.Sub(list[i].Last) >= 5*time.Minute {
					list[i].Last, changed = now, true
				}
				continue
			}
			latest[k] = len(list)
			list = append(list, Observation{Signal: sig, Since: now, Last: now})
			changed = true
		}
		return list, changed
	})
}

func (s Store) Health() ([]Observation, error) { return recent[Observation](s.path("health")) }
