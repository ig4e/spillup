//go:build !linux && !windows

package sysinfo

import (
	"os/exec"
	"strconv"
	"strings"
)

func readSpecs() Specs {
	s := baseSpecs()
	if out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
		s.CPUModel = strings.TrimSpace(string(out))
	}
	return s
}

func read() reading {
	var r reading
	if out, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
		r.memTotal, _ = strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	}
	if out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output(); err == nil {
		if f := strings.Fields(strings.Trim(strings.TrimSpace(string(out)), "{}")); len(f) > 0 {
			r.load1, _ = strconv.ParseFloat(f[0], 64)
		}
	}
	return r
}
