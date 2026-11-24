package sysinfo

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func readSpecs() Specs {
	s := baseSpecs()
	if f, err := os.Open("/proc/cpuinfo"); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if k, v, ok := strings.Cut(sc.Text(), ":"); ok && strings.TrimSpace(k) == "model name" {
				s.CPUModel = strings.TrimSpace(v)
				break
			}
		}
		f.Close()
	}
	return s
}

func read() reading {
	var r reading
	if data, err := os.ReadFile("/proc/stat"); err == nil {
		line, _, _ := strings.Cut(string(data), "\n")
		fields := strings.Fields(line)
		for i, f := range fields[1:] {
			v, _ := strconv.ParseUint(f, 10, 64)
			r.cpuTotal += v
			if i != 3 && i != 4 {
				r.cpuBusy += v
			}
		}
	}
	mem := map[string]uint64{}
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if k, v, ok := strings.Cut(line, ":"); ok {
				n, _ := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(v), " kB"), 10, 64)
				mem[k] = n * 1024
			}
		}
	}
	r.memTotal = mem["MemTotal"]
	if avail := mem["MemAvailable"]; avail <= r.memTotal {
		r.memUsed = r.memTotal - avail
	}
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		r.load1, _ = strconv.ParseFloat(strings.Fields(string(data))[0], 64)
	}
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		r.uptime, _ = strconv.ParseFloat(strings.Fields(string(data))[0], 64)
	}
	var fs syscall.Statfs_t
	if syscall.Statfs("/", &fs) == nil {
		r.diskTotal = fs.Blocks * uint64(fs.Bsize)
		r.diskUsed = r.diskTotal - fs.Bavail*uint64(fs.Bsize)
	}
	if data, err := os.ReadFile("/proc/net/dev"); err == nil {
		for _, line := range strings.Split(string(data), "\n")[2:] {
			name, rest, ok := strings.Cut(line, ":")
			if !ok || strings.TrimSpace(name) == "lo" {
				continue
			}
			f := strings.Fields(rest)
			if len(f) >= 9 {
				rx, _ := strconv.ParseUint(f[0], 10, 64)
				tx, _ := strconv.ParseUint(f[8], 10, 64)
				r.rx, r.tx = r.rx+rx, r.tx+tx
			}
		}
	}
	r.procs = processes()
	return r
}

var pageSize = uint64(os.Getpagesize())

// processes reads every program's CPU ticks and resident memory from /proc.
func processes() map[int]procTimes {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	out := make(map[int]procTimes, len(entries))
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		text := string(data)
		open, close := strings.IndexByte(text, '('), strings.LastIndexByte(text, ')')
		if open < 0 || close < open {
			continue
		}
		f := strings.Fields(text[close+1:])
		if len(f) < 22 {
			continue
		}
		utime, _ := strconv.ParseUint(f[11], 10, 64)
		stime, _ := strconv.ParseUint(f[12], 10, 64)
		rss, _ := strconv.ParseUint(f[21], 10, 64)
		out[pid] = procTimes{name: text[open+1 : close], cpu: utime + stime, mem: rss * pageSize}
	}
	return out
}
