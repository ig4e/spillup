package tailnet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const fakeModeEnv, fakeCountEnv = "SPINUP_FAKE_TAILSCALE", "SPINUP_FAKE_TAILSCALE_COUNT"

const fakeComplaint = "failed to connect to local tailscaled; it doesn't appear to be running"

func init() {
	mode := os.Getenv(fakeModeEnv)
	if mode == "" {
		return
	}
	countFile := os.Getenv(fakeCountEnv)
	data, _ := os.ReadFile(countFile)
	calls, _ := strconv.Atoi(string(data))
	calls++
	_ = os.WriteFile(countFile, []byte(strconv.Itoa(calls)), 0o600)
	if mode == "slow" {
		time.Sleep(500 * time.Millisecond)
	}
	if mode == "fail-always" || (mode == "fail-once" && calls == 1) {
		fmt.Fprintln(os.Stderr, fakeComplaint)
		os.Exit(1)
	}
	fmt.Print(`{"BackendState":"Running","Self":{"HostName":"office-pc","DNSName":"office-pc.tail1234.ts.net.","TailscaleIPs":["100.64.0.1"],"Online":true}}`)
	os.Exit(0)
}

func fakeTailscale(t *testing.T, mode string) (CLI, func() int) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	countFile := filepath.Join(t.TempDir(), "calls")
	t.Setenv(fakeModeEnv, mode)
	t.Setenv(fakeCountEnv, countFile)
	return CLI{Bin: exe}, func() int {
		data, _ := os.ReadFile(countFile)
		n, _ := strconv.Atoi(string(data))
		return n
	}
}

func TestStatusRetriesAQuickFailureOnce(t *testing.T) {
	cli, calls := fakeTailscale(t, "fail-once")
	st, err := cli.Status(context.Background())
	if err != nil {
		t.Fatalf("a single quick failure should be retried: %v", err)
	}
	if !st.Running || !st.Self.Online || calls() != 2 {
		t.Errorf("status = %+v after %d calls, want online after 2", st, calls())
	}
}

func TestCallersAskingAtTheSameTimeShareOneRun(t *testing.T) {
	cli, calls := fakeTailscale(t, "slow")
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for range 6 {
		wg.Go(func() {
			if st, err := cli.Status(context.Background()); err != nil || !st.Self.Online {
				errs <- fmt.Errorf("status = %+v, err = %v", st, err)
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if calls() != 1 {
		t.Errorf("ran the CLI %d times for 6 simultaneous callers, want 1", calls())
	}
}

func TestACancelledCallerDoesNotFailTheSharedRun(t *testing.T) {
	cli, _ := fakeTailscale(t, "slow")
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	go func() {
		close(started)
		_, _ = cli.Status(ctx)
	}()
	<-started
	time.Sleep(50 * time.Millisecond)
	cancel()
	if st, err := cli.Status(context.Background()); err != nil || !st.Self.Online {
		t.Errorf("status = %+v, err = %v; another caller's cancel must not fail it", st, err)
	}
}

func TestStatusNamesTheCLIComplaintWhenItKeepsFailing(t *testing.T) {
	cli, calls := fakeTailscale(t, "fail-always")
	_, err := cli.Status(context.Background())
	if err == nil || !strings.Contains(err.Error(), fakeComplaint) {
		t.Fatalf("error = %v, want it to carry the CLI's own message", err)
	}
	if calls() != 2 {
		t.Errorf("ran the CLI %d times, want 2 (one retry)", calls())
	}
}
