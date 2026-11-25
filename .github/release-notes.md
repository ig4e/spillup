**spillup** is [spinup](https://github.com/darkyeg/spinup) with a dashboard: run `spinup ui` to manage your machines, the shared Claude/Codex accounts, the main machine, skills and checks from your browser.

![The spillup dashboard](https://raw.githubusercontent.com/ig4e/spillup/master/docs/screenshots/overview-light.png)

### In v0.4.7

- **The accounts stay up on a busy machine.** When the hub was under heavy load, `tailscale status` sometimes failed and the hub stopped and restarted its proxy every few minutes. That cut running requests and briefly showed "CLIProxyAPI runs on <this machine>, not here". Spillup now runs one shared Tailscale status check per moment instead of one per caller, retries a quick failure once, and logs the CLI's own error.
- A starting CLIProxyAPI gets 40 seconds to answer instead of 30.

### Install

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/ig4e/spillup/master/scripts/install.ps1 | iex
```

Linux (Arch and others) and macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/ig4e/spillup/master/scripts/install.sh | sh
```

Or download the binary for your system below (`spinup_windows_amd64.exe`, `spinup_linux_amd64`, …) and check it against `checksums.txt`.
