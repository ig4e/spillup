//go:build !linux && !windows

package pty

// Term is a shell running in a pseudo-terminal.
type Term struct{}

// Start reports ErrUnsupported: this platform has no implementation.
func Start(argv []string, dir string, env []string, cols, rows uint16) (*Term, error) {
	return nil, ErrUnsupported
}

func (t *Term) Read(p []byte) (int, error)     { return 0, ErrUnsupported }
func (t *Term) Write(p []byte) (int, error)    { return 0, ErrUnsupported }
func (t *Term) Resize(cols, rows uint16) error { return ErrUnsupported }
func (t *Term) Close() error                   { return nil }
func (t *Term) Wait() error                    { return ErrUnsupported }
