//go:build !linux && !darwin && !windows

package main

import (
	"errors"

	"github.com/darkyeg/spinup/internal/config"
)

func restartServiceNow(config.Config) error {
	return errors.New("restart the accounts service with your system's service manager")
}
