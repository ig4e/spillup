// Package pty runs an interactive shell in a pseudo-terminal.
package pty

import (
	"errors"
	"os"
)

// ErrUnsupported is returned by Start on platforms without a pseudo-terminal implementation.
var ErrUnsupported = errors.New("pty: not supported on this platform")

func environment(env []string) []string {
	if env == nil {
		return os.Environ()
	}
	return env
}
