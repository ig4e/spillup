// Package sysinfo reads this machine's specs and load, and keeps a short history of them for the
// dashboard's charts.
package sysinfo

import (
	"os"
	"runtime"
	"sort"
	"sync"
	"time"
)

// Specs are what a machine is: they don't change while it runs.
type Specs struct {
	Hostname  string `json:"hostname"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	CPUModel  string `json:"cpu_model,omitempty"`
	Cores     int    `json:"cores"`
	MemTotal  uint64 `json:"mem_total"`
	DiskTotal uint64 `json:"disk_total,omitempty"`
}

// Sample is the machine's load at one moment; rates are per second since the sample before.
type Sample struct {
	At       time.Time `json:"at"`
	CPU      float64   `json:"cpu"`
	MemUsed  uint64    `json:"mem_used"`
	DiskUsed uint64    `json:"disk_used,omitempty"`
	Load1    float64   `json:"load1,omitempty"`
	RxRate   float64   `json:"rx_rate"`
	TxRate   float64   `json:"tx_rate"`
	RxTotal  uint64    `json:"rx_total"`
	TxTotal  uint64    `json:"tx_total"`
	Uptime   float64   `json:"uptime"`
}

// Process is one program's share of the machine, as Activity Monitor shows it: CPU is a percentage of
// one core, so a busy program on several cores goes over 100.
type Process struct {
	PID  int     `json:"pid"`
	Name string  `json:"name"`
	CPU  float64 `json:"cpu"`
	Mem  uint64  `json:"mem"`
}

// Stats are a machine's specs, its load now, its recent history, oldest first, and its busiest programs.
type Stats struct {
	Specs     Specs     `json:"specs"`
	Now       Sample    `json:"now"`
	History   []Sample  `json:"history"`
	Processes []Process `json:"processes"`
	Count     int       `json:"process_count"`
}

// procTimes are one program's raw readings: its CPU time, in the same unit as the machine's, and memory.
type procTimes struct {
	name string
	cpu  uint64
	mem  uint64
}

// counters are the raw cumulative readings two samples are compared from.
type counters struct {
	cpuBusy, cpuTotal uint64
	rx, tx            uint64
}

// reading is one look at the machine: what can be read directly, plus the counters for rates.
type reading struct {
	counters
	procs               map[int]procTimes
	memUsed, memTotal   uint64
	diskUsed, diskTotal uint64
	load1, uptime       float64
}

const (
	every   = 2 * time.Second
	history = 90
	// topProcesses is how many programs the dashboard lists.
	topProcesses = 40
	// idleAfter stops sampling when nobody has asked for a while, so an unwatched dashboard costs nothing.
	idleAfter = 2 * time.Minute
)

// Sampler samples while someone watches.
type Sampler struct {
	mu      sync.Mutex
	specs   Specs
	samples []Sample
	last    reading
	lastAt  time.Time
	asked   time.Time
	running bool
	procs   []Process
	count   int
}

func NewSampler() *Sampler { return &Sampler{} }

// Stats starts sampling if needed and returns what it has so far.
func (s *Sampler) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asked = time.Now()
	if !s.running {
		s.running = true
		s.specs = readSpecs()
		s.sampleLocked()
		go s.loop()
	}
	out := Stats{Specs: s.specs, History: append([]Sample(nil), s.samples...), Processes: append([]Process{}, s.procs...), Count: s.count}
	if n := len(s.samples); n > 0 {
		out.Now = s.samples[n-1]
	}
	return out
}

func (s *Sampler) loop() {
	t := time.NewTicker(every)
	defer t.Stop()
	for range t.C {
		s.mu.Lock()
		if time.Since(s.asked) > idleAfter {
			s.running, s.samples = false, nil
			s.mu.Unlock()
			return
		}
		s.sampleLocked()
		s.mu.Unlock()
	}
}

func (s *Sampler) sampleLocked() {
	r, now := read(), time.Now()
	sample := Sample{At: now, MemUsed: r.memUsed, DiskUsed: r.diskUsed, Load1: r.load1, RxTotal: r.rx, TxTotal: r.tx, Uptime: r.uptime}
	if r.memTotal > 0 {
		s.specs.MemTotal = r.memTotal
	}
	if r.diskTotal > 0 {
		s.specs.DiskTotal = r.diskTotal
	}
	if !s.lastAt.IsZero() {
		secs := now.Sub(s.lastAt).Seconds()
		sample.CPU = percent(r.cpuBusy-s.last.cpuBusy, r.cpuTotal-s.last.cpuTotal)
		if secs > 0 && r.rx >= s.last.rx && r.tx >= s.last.tx {
			sample.RxRate = float64(r.rx-s.last.rx) / secs
			sample.TxRate = float64(r.tx-s.last.tx) / secs
		}
		s.procs, s.count = busiest(r.procs, s.last.procs, r.cpuTotal-s.last.cpuTotal, s.specs.Cores), len(r.procs)
		s.samples = append(s.samples, sample)
		if len(s.samples) > history {
			s.samples = s.samples[len(s.samples)-history:]
		}
	}
	s.last, s.lastAt = r, now
}

func percent(busy, total uint64) float64 {
	if total == 0 || busy > total {
		return 0
	}
	return float64(busy) / float64(total) * 100
}

func baseSpecs() Specs {
	host, _ := os.Hostname()
	return Specs{Hostname: host, OS: runtime.GOOS, Arch: runtime.GOARCH, Cores: runtime.NumCPU()}
}

// busiest ranks programs by CPU, then memory, from two readings a sample apart.
func busiest(now, before map[int]procTimes, totalDelta uint64, cores int) []Process {
	if cores < 1 {
		cores = 1
	}
	out := make([]Process, 0, len(now))
	for pid, p := range now {
		proc := Process{PID: pid, Name: p.name, Mem: p.mem}
		if b, ok := before[pid]; ok && b.name == p.name && totalDelta > 0 && p.cpu >= b.cpu {
			proc.CPU = float64(p.cpu-b.cpu) / (float64(totalDelta) / float64(cores)) * 100
		}
		out = append(out, proc)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CPU != out[j].CPU {
			return out[i].CPU > out[j].CPU
		}
		return out[i].Mem > out[j].Mem
	})
	if len(out) > topProcesses {
		out = out[:topProcesses]
	}
	return out
}
