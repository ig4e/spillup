package main

import "github.com/darkyeg/spinup/internal/config"

func restartServiceNow(config.Config) error {
	return runServiceManager("systemctl", "--user", "restart", unitName)
}
