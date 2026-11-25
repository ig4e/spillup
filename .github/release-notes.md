**spillup** is [spinup](https://github.com/darkyeg/spinup) with a dashboard: run `spinup ui` to manage your machines, the shared Claude/Codex accounts, the main machine, skills and checks from your browser.

![The spillup dashboard](https://raw.githubusercontent.com/ig4e/spillup/master/docs/screenshots/overview-light.png)

### In v0.4.1

- Remote has a native Spillup streaming client with touch, keyboard, audio, fullscreen and text clipboard controls.
- Doctor and Updates now share one screen. Dashboard pages fit phone widths, with shorter labels and simpler machine and proxy controls.
- T3 repairs Usage credentials for the verified active local CLIProxyAPI port and the Spillup gateway. To work on a linked machine from T3 Desktop, choose it under **T3 Code → Desktop connections** or run `spinup t3 connect <machine>`. Confirm once in T3, then select the machine in **Add Project**.
- Workers can read quota windows and account health. Machines shows a health timeline, and Activity preserves actions and job outcomes across restarts.
- Setup previews local configuration drift. Doctor & Updates previews the fleet, and `spinup rollout --apply` updates followers one at a time from the hub. A sole active holder waits for a maintenance update.

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
