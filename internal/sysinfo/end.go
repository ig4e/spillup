package sysinfo

import (
	"errors"
	"os"
)

// ErrProtected is the answer for a process spillup won't end: the system's own, or spillup itself.
var ErrProtected = errors.New("spillup won't end this process: it belongs to the system or to spillup itself")

// End asks a process to quit, as Activity Monitor's Quit does, or with force ends it at once.
func End(pid int, force bool) error {
	if pid <= 4 || pid == os.Getpid() || pid == os.Getppid() {
		return ErrProtected
	}
	return end(pid, force)
}
