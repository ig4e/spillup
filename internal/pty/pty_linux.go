package pty

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// Term is a shell running in a pseudo-terminal.
type Term struct {
	ptmx      *os.File
	cmd       *exec.Cmd
	closeOnce sync.Once
	closeErr  error
}

// Start runs argv in dir on a new pseudo-terminal of cols x rows. A nil env inherits spinup's
// environment; TERM defaults to xterm-256color.
func Start(argv []string, dir string, env []string, cols, rows uint16) (*Term, error) {
	if len(argv) == 0 {
		return nil, errors.New("pty: no command")
	}
	ptmx, tty, err := open()
	if err != nil {
		return nil, err
	}
	defer tty.Close()
	if err := setSize(ptmx, cols, rows); err != nil {
		ptmx.Close()
		return nil, err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = withTerm(environment(env))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		ptmx.Close()
		return nil, err
	}
	return &Term{ptmx: ptmx, cmd: cmd}, nil
}

func open() (ptmx, tty *os.File, err error) {
	ptmx, err = os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	var n uint32
	err = control(ptmx, func(fd int) error {
		if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
			return err
		}
		n, err = unix.IoctlGetUint32(fd, unix.TIOCGPTN)
		return err
	})
	if err == nil {
		tty, err = os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	}
	if err != nil {
		ptmx.Close()
		return nil, nil, err
	}
	return ptmx, tty, nil
}

func control(f *os.File, op func(fd int) error) error {
	conn, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var opErr error
	if err := conn.Control(func(fd uintptr) { opErr = op(int(fd)) }); err != nil {
		return err
	}
	return opErr
}

func setSize(f *os.File, cols, rows uint16) error {
	return control(f, func(fd int) error {
		return unix.IoctlSetWinsize(fd, unix.TIOCSWINSZ, &unix.Winsize{Row: rows, Col: cols})
	})
}

func withTerm(env []string) []string {
	for _, kv := range env {
		if strings.HasPrefix(kv, "TERM=") {
			return env
		}
	}
	return append(env[:len(env):len(env)], "TERM=xterm-256color")
}

// Read returns the terminal's output, and io.EOF once the shell and everything holding the
// terminal have exited.
func (t *Term) Read(p []byte) (int, error) {
	n, err := t.ptmx.Read(p)
	if errors.Is(err, syscall.EIO) {
		err = io.EOF
	}
	return n, err
}

// Write types p into the terminal.
func (t *Term) Write(p []byte) (int, error) { return t.ptmx.Write(p) }

// Resize changes the terminal size; the shell gets SIGWINCH.
func (t *Term) Resize(cols, rows uint16) error { return setSize(t.ptmx, cols, rows) }

// Close kills the shell's process group and frees the terminal.
func (t *Term) Close() error {
	t.closeOnce.Do(func() {
		_ = syscall.Kill(-t.cmd.Process.Pid, syscall.SIGKILL)
		_ = t.cmd.Process.Kill()
		t.closeErr = t.ptmx.Close()
	})
	return t.closeErr
}

// Wait waits for the shell to exit.
func (t *Term) Wait() error { return t.cmd.Wait() }
