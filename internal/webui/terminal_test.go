//go:build linux

package webui

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestATerminalRunsWhatYouTypeAndStreamsItsOutput(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	srv := httptest.NewServer(New(&fakeBackend{}, testToken).Handler())
	defer srv.Close()
	post := func(path, body string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: testToken})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	resp := post("/api/term", `{"cols":80,"rows":24}`)
	var started terminalStarted
	_ = json.NewDecoder(resp.Body).Decode(&started)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || started.ID == "" {
		t.Fatalf("start: %d %+v", resp.StatusCode, started)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/term/"+started.ID+"/out", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: testToken})
	out, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Body.Close()

	post("/api/term/"+started.ID+"/in", `{"data":"echo spill$((40+2))\n"}`).Body.Close()
	seen := make(chan bool, 1)
	go func() {
		s := bufio.NewScanner(out.Body)
		for s.Scan() {
			if strings.Contains(s.Text(), "spill42") {
				seen <- true
				return
			}
		}
		seen <- false
	}()
	select {
	case ok := <-seen:
		if !ok {
			t.Fatal("the output ended without the command's answer")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no answer from the terminal")
	}

	if resp := post("/api/term/"+started.ID+"/close", `{}`); resp.StatusCode != http.StatusNoContent {
		t.Errorf("close: %d", resp.StatusCode)
	}
}
