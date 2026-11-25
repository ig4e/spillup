**spillup** is [spinup](https://github.com/darkyeg/spinup) with a dashboard: run `spinup ui` to manage your machines, the shared Claude/Codex accounts, the main machine, skills and checks from your browser.

![The spillup dashboard](https://raw.githubusercontent.com/ig4e/spillup/master/docs/screenshots/overview-light.png)

### In v0.4.2

- Phone pages use one stable header and a floating glass tab bar. Navigation respects screen cutouts, light and dark appearance, and reduced transparency.
- Revisiting a page immediately shows the selected machine's last reading while Spillup checks for changes. Views share requests, and common navigation links can preload their data.
- Machine rows, quota readings and resource graphs stay visible during temporary refresh failures. Activity keeps process actions tied to the machine that supplied the process list.
- **T3 Code → Phones & tablets** shows which online host holds each phone's pairing. Spillup reads T3's existing mobile sessions, so already-paired phones appear without scanning another code.

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
