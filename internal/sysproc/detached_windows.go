package sysproc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// The scheduled service runs inside a job that ends with it. WMI starts the helper outside that job.
func StartDetached(logPath string, exe string, args ...string) error {
	request := struct {
		Exe      string
		Args     []string
		Log      string
		StateDir string
	}{exe, args, logPath, os.Getenv("SPINUP_HOME")}
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	child := `$j = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + base64.StdEncoding.EncodeToString(data) + `')) | ConvertFrom-Json
if ($j.StateDir) { $env:SPINUP_HOME = $j.StateDir }
$arguments = @($j.Args)
& $j.Exe @arguments 2>&1 | Out-File -LiteralPath $j.Log -Append -Encoding utf8
exit $LASTEXITCODE`
	ps := `$ErrorActionPreference = 'Stop'
$encoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes('` + strings.ReplaceAll(child, "'", "''") + `'))
$line = '"' + (Join-Path $PSHOME 'powershell.exe') + '" -NoProfile -NonInteractive -WindowStyle Hidden -EncodedCommand ' + $encoded
$r = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{CommandLine=$line} -OperationTimeoutSec 12
if ($null -eq $r -or $r.ReturnValue -ne 0 -or -not $r.ProcessId) { throw "WMI process creation failed (code $($r.ReturnValue))" }`
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.WaitDelay = 3 * time.Second
	Hide(cmd)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("start detached process through WMI: %w", ctx.Err())
	}
	if err != nil {
		return fmt.Errorf("start detached process through WMI: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
