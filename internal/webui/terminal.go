package webui

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/darkyeg/spinup/internal/pty"
)

// terminalIdle is how long a terminal with no window showing it keeps running before it closes.
const terminalIdle = 2 * time.Minute

// scrollback is how much recent output a terminal keeps, to show again when a window reattaches.
const scrollback = 256 << 10

type terminal struct {
	id   string
	term *pty.Term

	mu       sync.Mutex
	recent   []byte
	readers  map[chan []byte]bool
	exited   bool
	detached time.Time
}

type terminals struct {
	mu   sync.Mutex
	open map[string]*terminal
}

func (ts *terminals) get(id string) *terminal {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.open[id]
}

func (ts *terminals) start(cols, rows uint16) (*terminal, error) {
	home, _ := os.UserHomeDir()
	term, err := pty.Start(pty.Shell(), home, nil, cols, rows)
	if err != nil {
		return nil, err
	}
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	t := &terminal{id: hex.EncodeToString(b), term: term, readers: map[chan []byte]bool{}, detached: time.Now()}
	ts.mu.Lock()
	if ts.open == nil {
		ts.open = map[string]*terminal{}
	}
	ts.open[t.id] = t
	ts.mu.Unlock()
	go t.pump(func() { ts.drop(t.id) })
	go ts.closeWhenIdle(t)
	return t, nil
}

func (ts *terminals) drop(id string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	delete(ts.open, id)
}

func (ts *terminals) closeWhenIdle(t *terminal) {
	for {
		time.Sleep(terminalIdle / 4)
		t.mu.Lock()
		idle := !t.exited && len(t.readers) == 0 && time.Since(t.detached) > terminalIdle
		done := t.exited
		t.mu.Unlock()
		if done {
			return
		}
		if idle {
			_ = t.term.Close()
			return
		}
	}
}

// pump copies the shell's output to every window showing it, keeping the recent part for the next one.
func (t *terminal) pump(gone func()) {
	buf := make([]byte, 32<<10)
	for {
		n, err := t.term.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			t.mu.Lock()
			t.recent = append(t.recent, chunk...)
			if over := len(t.recent) - scrollback; over > 0 {
				t.recent = t.recent[over:]
			}
			for r := range t.readers {
				select {
				case r <- chunk:
				default:
				}
			}
			t.mu.Unlock()
		}
		if err != nil {
			break
		}
	}
	_ = t.term.Close()
	_ = t.term.Wait()
	t.mu.Lock()
	t.exited = true
	for r := range t.readers {
		close(r)
	}
	t.readers = map[chan []byte]bool{}
	t.mu.Unlock()
	gone()
}

func (t *terminal) attach() (chan []byte, []byte, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.exited {
		return nil, nil, false
	}
	r := make(chan []byte, 256)
	t.readers[r] = true
	return r, append([]byte(nil), t.recent...), true
}

func (t *terminal) detach(r chan []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.readers[r] {
		delete(t.readers, r)
		close(r)
	}
	if len(t.readers) == 0 {
		t.detached = time.Now()
	}
}

type terminalSize struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

type terminalStarted struct {
	ID string `json:"id"`
}

type terminalInput struct {
	Data string `json:"data"`
}

func (s *Server) terminalsAllowed(w http.ResponseWriter) bool {
	if !s.b.Facts().Config.Terminals {
		writeError(w, http.StatusForbidden, "terminals are off on this machine: turn them on in its Settings")
		return false
	}
	return true
}

func (s *Server) serveTerminalStart(w http.ResponseWriter, r *http.Request) {
	var size terminalSize
	if !s.terminalsAllowed(w) || !decodeBody(w, r, &size) {
		return
	}
	t, err := s.terms.start(max(size.Cols, 20), max(size.Rows, 5))
	if err != nil {
		writeError(w, http.StatusConflict, "couldn't open a terminal: "+err.Error())
		return
	}
	writeJSON(w, terminalStarted{ID: t.id})
}

func (s *Server) terminalFor(w http.ResponseWriter, r *http.Request) *terminal {
	if !s.terminalsAllowed(w) {
		return nil
	}
	t := s.terms.get(r.PathValue("id"))
	if t == nil {
		writeError(w, http.StatusNotFound, "this terminal has closed")
	}
	return t
}

// serveTerminalOutput streams the terminal's output, starting with what it showed recently, until the shell
// exits or the window goes away.
func (s *Server) serveTerminalOutput(w http.ResponseWriter, r *http.Request) {
	t := s.terminalFor(w, r)
	if t == nil {
		return
	}
	out, recent, ok := t.attach()
	if !ok {
		writeError(w, http.StatusNotFound, "this terminal has closed")
		return
	}
	defer t.detach(out)
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	write := func(b []byte) bool {
		if _, err := w.Write(b); err != nil {
			return false
		}
		if flusher != nil {
			flusher.Flush()
		}
		return true
	}
	if !write(recent) {
		return
	}
	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case chunk, open := <-out:
			if !open || !write(chunk) {
				return
			}
		case <-keepAlive.C:
			if !write(nil) {
				return
			}
		}
	}
}

func (s *Server) serveTerminalInput(w http.ResponseWriter, r *http.Request) {
	t := s.terminalFor(w, r)
	var in terminalInput
	if t == nil || !decodeBody(w, r, &in) {
		return
	}
	if _, err := t.term.Write([]byte(in.Data)); err != nil {
		writeError(w, http.StatusGone, "this terminal has closed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) serveTerminalResize(w http.ResponseWriter, r *http.Request) {
	t := s.terminalFor(w, r)
	var size terminalSize
	if t == nil || !decodeBody(w, r, &size) {
		return
	}
	if err := t.term.Resize(max(size.Cols, 20), max(size.Rows, 5)); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) serveTerminalClose(w http.ResponseWriter, r *http.Request) {
	if t := s.terminalFor(w, r); t != nil {
		_ = t.term.Close()
		w.WriteHeader(http.StatusNoContent)
	}
}
