package sysinfo

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	getSystemTimes       = kernel32.NewProc("GetSystemTimes")
	globalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
	getTickCount64       = kernel32.NewProc("GetTickCount64")
	getProcessMemoryInfo = kernel32.NewProc("K32GetProcessMemoryInfo")
)

type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

type memoryStatusEx struct {
	length, memoryLoad                 uint32
	totalPhys, availPhys               uint64
	totalPageFile, availPageFile       uint64
	totalVirtual, availVirtual, extVir uint64
}

func readSpecs() Specs {
	s := baseSpecs()
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE); err == nil {
		s.CPUModel, _, _ = k.GetStringValue("ProcessorNameString")
		k.Close()
	}
	return s
}

func filetime(ft windows.Filetime) uint64 {
	return uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
}

func read() reading {
	var r reading
	var idle, kernel, user windows.Filetime
	if ok, _, _ := getSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user))); ok != 0 {
		r.cpuTotal = filetime(kernel) + filetime(user)
		r.cpuBusy = r.cpuTotal - filetime(idle)
	}
	m := memoryStatusEx{length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	if ok, _, _ := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m))); ok != 0 {
		r.memTotal, r.memUsed = m.totalPhys, m.totalPhys-m.availPhys
	}
	if ms, _, _ := getTickCount64.Call(); ms != 0 {
		r.uptime = float64(ms) / 1000
	}
	drive := os.Getenv("SystemDrive")
	if drive == "" {
		drive = "C:"
	}
	if p, err := windows.UTF16PtrFromString(drive + `\`); err == nil {
		var free, total, totalFree uint64
		if windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree) == nil {
			r.diskTotal, r.diskUsed = total, total-totalFree
		}
	}
	var table *windows.MibIfTable2
	if windows.GetIfTable2Ex(0, &table) == nil && table != nil {
		rows := unsafe.Slice(&table.Table[0], table.NumEntries)
		for _, row := range rows {
			if row.Type == windows.IF_TYPE_SOFTWARE_LOOPBACK || row.OperStatus != windows.IfOperStatusUp || row.InterfaceAndOperStatusFlags&1 == 0 {
				continue
			}
			r.rx, r.tx = r.rx+row.InOctets, r.tx+row.OutOctets
		}
		windows.FreeMibTable(unsafe.Pointer(table))
	}
	r.procs = processes()
	return r
}

// processes reads every program it may open: its CPU time, in the 100ns units GetSystemTimes counts in,
// and its working set.
func processes() map[int]procTimes {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	out := map[int]procTimes{}
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err := windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		pid := entry.ProcessID
		if pid == 0 {
			continue
		}
		p := procTimes{name: windows.UTF16ToString(entry.ExeFile[:])}
		if h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid); err == nil {
			var created, exited, kernel, user windows.Filetime
			if windows.GetProcessTimes(h, &created, &exited, &kernel, &user) == nil {
				p.cpu = filetime(kernel) + filetime(user)
			}
			mem := processMemoryCounters{cb: uint32(unsafe.Sizeof(processMemoryCounters{}))}
			if ok, _, _ := getProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&mem)), uintptr(mem.cb)); ok != 0 {
				p.mem = uint64(mem.workingSetSize)
			}
			windows.CloseHandle(h)
		}
		out[int(pid)] = p
	}
	return out
}
