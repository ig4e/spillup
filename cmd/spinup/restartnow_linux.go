package main

import "github.com/darkyeg/spinup/internal/config"

// restartServiceNow restarts the accounts service as this user: systemd stops it gracefully, which
// hands the accounts on, and starts the binary installed now.
func restartServiceNow(config.Config) error { return run("systemctl", "--user", "restart", unitName) }
