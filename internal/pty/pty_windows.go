package pty

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Term is a shell running in a pseudo-console (ConPTY).
type Term struct {
	console   windows.Handle
	in        *os.File
	out       *os.File
	mu        sync.Mutex
	process   windows.Handle
	exited    chan struct{}
	exitErr   error
	closeOnce sync.Once
}

// Start runs argv in dir on a new pseudo-console of cols x rows. A nil env inherits spinup's
// environment.
func Start(argv []string, dir string, env []string, cols, rows uint16) (*Term, error) {
	if len(argv) == 0 {
		return nil, errors.New("pty: no command")
	}
	exe, err := exec.LookPath(argv[0])
	if err != nil {
		return nil, err
	}
	var ptyIn, inWrite, outRead, ptyOut windows.Handle
	if err := windows.CreatePipe(&ptyIn, &inWrite, nil, 0); err != nil {
		return nil, err
	}
	if err := windows.CreatePipe(&outRead, &ptyOut, nil, 0); err != nil {
		closeHandles(ptyIn, inWrite)
		return nil, err
	}
	defer closeHandles(ptyIn, ptyOut)
	var console windows.Handle
	if err := windows.CreatePseudoConsole(coord(cols, rows), ptyIn, ptyOut, 0, &console); err != nil {
		closeHandles(inWrite, outRead)
		return nil, err
	}
	process, err := spawn(console, exe, argv, dir, environment(env))
	if err != nil {
		windows.ClosePseudoConsole(console)
		closeHandles(inWrite, outRead)
		return nil, err
	}
	t := &Term{
		console: console,
		in:      os.NewFile(uintptr(inWrite), "conpty-in"),
		out:     os.NewFile(uintptr(outRead), "conpty-out"),
		process: process,
		exited:  make(chan struct{}),
	}
	go t.reap()
	return t, nil
}

func spawn(console windows.Handle, exe string, argv []string, dir string, env []string) (windows.Handle, error) {
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return 0, err
	}
	defer attrs.Delete()
	hpc := *(*unsafe.Pointer)(unsafe.Pointer(&console))
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, hpc, unsafe.Sizeof(console)); err != nil {
		return 0, err
	}
	si := windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = windows.STARTF_USESTDHANDLES
	app, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return 0, err
	}
	line, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(argv))
	if err != nil {
		return 0, err
	}
	var cwd *uint16
	if dir != "" {
		if cwd, err = windows.UTF16PtrFromString(dir); err != nil {
			return 0, err
		}
	}
	block, err := envBlock(env)
	if err != nil {
		return 0, err
	}
	var pi windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT)
	if err := windows.CreateProcess(app, line, nil, nil, false, flags, &block[0], cwd, &si.StartupInfo, &pi); err != nil {
		return 0, err
	}
	windows.CloseHandle(pi.Thread)
	return pi.Process, nil
}

func envBlock(env []string) ([]uint16, error) {
	var block []uint16
	for _, kv := range env {
		for _, r := range kv {
			if r == 0 {
				return nil, fmt.Errorf("pty: NUL in environment entry %q", kv)
			}
		}
		block = append(block, utf16.Encode([]rune(kv))...)
		block = append(block, 0)
	}
	if len(block) == 0 {
		block = append(block, 0)
	}
	return append(block, 0), nil
}

func coord(cols, rows uint16) windows.Coord {
	return windows.Coord{X: int16(cols), Y: int16(rows)}
}

func closeHandles(hs ...windows.Handle) {
	for _, h := range hs {
		windows.CloseHandle(h)
	}
}

func (t *Term) reap() {
	_, err := windows.WaitForSingleObject(t.process, windows.INFINITE)
	t.mu.Lock()
	if err == nil {
		var code uint32
		if err = windows.GetExitCodeProcess(t.process, &code); err == nil && code != 0 {
			err = fmt.Errorf("exit status %d", code)
		}
	}
	t.exitErr = err
	windows.CloseHandle(t.process)
	t.process = 0
	t.mu.Unlock()
	close(t.exited)
}

// Read returns the console's output, and io.EOF once the pseudo-console is closed.
func (t *Term) Read(p []byte) (int, error) { return t.out.Read(p) }

// Write types p into the console.
func (t *Term) Write(p []byte) (int, error) { return t.in.Write(p) }

// Resize changes the console size.
func (t *Term) Resize(cols, rows uint16) error {
	return windows.ResizePseudoConsole(t.console, coord(cols, rows))
}

// Close ends the shell and frees the pseudo-console.
func (t *Term) Close() error {
	t.closeOnce.Do(func() {
		t.mu.Lock()
		if t.process != 0 {
			_ = windows.TerminateProcess(t.process, 1)
		}
		t.mu.Unlock()
		t.in.Close()
		windows.ClosePseudoConsole(t.console)
		t.out.Close()
	})
	return nil
}

// Wait waits for the shell to exit.
func (t *Term) Wait() error {
	<-t.exited
	return t.exitErr
}
