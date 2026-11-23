package sysproc

import (
	"os/exec"
	"strings"
)

// StartDetached starts a program that must outlive spinup's service, such as an update that restarts it.
// The boot task runs spinup in a job object that ends with it, so the program is created through WMI,
// outside that job. Its output goes to logPath.
func StartDetached(logPath string, exe string, args ...string) error {
	line := quote(exe)
	for _, a := range args {
		line += " " + quote(a)
	}
	command := `cmd.exe /d /c "` + line + ` >> ` + quote(logPath) + ` 2>&1"`
	ps := "Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{CommandLine=" + psString(command) + "} | Out-Null"
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", ps)
	Hide(cmd)
	return cmd.Run()
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, ``) + `"` }

func psString(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
