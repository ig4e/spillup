package shell

import (
	"os"
	"path/filepath"
	"strings"
)

// userBins are where per-user installers put programs: rustup, bun, go install, deno, pipx and others.
// Their installers add them to your shell's profile, which the service and a fresh process don't read.
func userBins() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var dirs []string
	for _, d := range []string{".cargo/bin", ".bun/bin", ".local/bin", "go/bin", ".deno/bin", ".npm-global/bin", ".local/share/mise/shims"} {
		if fi, err := os.Stat(filepath.Join(home, d)); err == nil && fi.IsDir() {
			dirs = append(dirs, filepath.Join(home, d))
		}
	}
	return dirs
}

// AddUserBins puts the per-user program folders on this process's PATH, so its checks and the programs
// it runs find tools installed there without a new terminal.
func AddUserBins() {
	path := os.Getenv("PATH")
	have := map[string]bool{}
	for _, p := range filepath.SplitList(path) {
		have[filepath.Clean(p)] = true
	}
	var add []string
	for _, d := range userBins() {
		if !have[filepath.Clean(d)] {
			add = append(add, d)
		}
	}
	if len(add) > 0 {
		os.Setenv("PATH", strings.Join(append(filepath.SplitList(path), add...), string(os.PathListSeparator)))
	}
}
