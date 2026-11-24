package sysinfo

import (
	"testing"
	"time"
)

func TestTheSamplerReadsThisMachine(t *testing.T) {
	s := NewSampler()
	first := s.Stats()
	if first.Specs.Cores == 0 || first.Specs.OS == "" {
		t.Fatalf("specs: %+v", first.Specs)
	}
	time.Sleep(every + 300*time.Millisecond)
	st := s.Stats()
	if len(st.History) == 0 {
		t.Fatal("no samples after one interval")
	}
	if st.Now.CPU < 0 || st.Now.CPU > 100 {
		t.Errorf("cpu %v outside 0..100", st.Now.CPU)
	}
	if st.Specs.MemTotal != 0 && st.Now.MemUsed > st.Specs.MemTotal {
		t.Errorf("memory used %d over total %d", st.Now.MemUsed, st.Specs.MemTotal)
	}
}

func TestPercentHandlesNoTimeAndWraparound(t *testing.T) {
	if percent(5, 0) != 0 || percent(10, 5) != 0 || percent(1, 4) != 25 {
		t.Error("percent")
	}
}

func TestBusiestRanksByCPUThenMemory(t *testing.T) {
	before := map[int]procTimes{1: {name: "a", cpu: 100}, 2: {name: "b", cpu: 100}, 3: {name: "c", cpu: 0}}
	now := map[int]procTimes{1: {name: "a", cpu: 150, mem: 10}, 2: {name: "b", cpu: 300, mem: 5}, 3: {name: "c", cpu: 0, mem: 99}, 4: {name: "new", mem: 1}}
	got := busiest(now, before, 400, 2)
	if got[0].Name != "b" || got[0].CPU != 100 || got[1].Name != "a" || got[1].CPU != 25 || got[2].Name != "c" {
		t.Errorf("ranking: %+v", got)
	}
}
