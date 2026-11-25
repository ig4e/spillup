package main

import (
	"fmt"
	"os"

	"github.com/darkyeg/spinup/internal/config"
)

// restartServiceNow restarts the accounts service's LaunchAgent, which stops it gracefully first.
func restartServiceNow(config.Config) error {
	return runServiceManager("launchctl", "kickstart", "-k", fmt.Sprintf("gui/%d/%s", os.Getuid(), agentLabel))
}
