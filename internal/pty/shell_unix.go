//go:build !windows

package pty

import "os"

// Shell is the user's interactive shell as a login shell: $SHELL, else bash, else sh.
func Shell() []string {
	for _, sh := range []string{os.Getenv("SHELL"), "/bin/bash"} {
		if sh == "" {
			continue
		}
		if _, err := os.Stat(sh); err == nil {
			return []string{sh, "-l"}
		}
	}
	return []string{"/bin/sh", "-l"}
}
