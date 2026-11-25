package t3code

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/darkyeg/spinup/internal/tailnet"
)

func TestPhoneSummaryUsesOnlyActiveSessionsAndExactOwnedMobilePeers(t *testing.T) {
	const data = `[
		{"expiresAt":"2026-06-01T00:00:00Z","client":{"deviceType":"mobile","ipAddress":"::ffff:100.64.0.2"}},
		{"expiresAt":"2026-06-01T00:00:00Z","client":{"deviceType":"desktop","ipAddress":"fd7a:115c:a1e0:0:0:0:0:2"},"connected":false},
		{"expiresAt":"2026-06-01T00:00:00Z","client":{"deviceType":"unknown","ipAddress":"100.64.0.3"}},
		{"expiresAt":"2026-06-01T00:00:00Z","client":{"deviceType":"tablet","ipAddress":"100.64.0.3"}},
		{"expiresAt":"2026-06-01T00:00:00Z","client":{"deviceType":"mobile","ipAddress":"100.64.0.4","label":"pixel"}},
		{"expiresAt":"2026-06-01T00:00:00Z","client":{"deviceType":"mobile","ipAddress":"100.64.0.5","os":"Android"}},
		{"expiresAt":"2026-06-01T00:00:00Z","client":{"deviceType":"tablet","ipAddress":"127.0.0.1","label":"slate"}},
		{"expiresAt":"2026-06-01T00:00:00Z","client":{"deviceType":"desktop","ipAddress":"100.64.0.20","label":"pixel"}},
		{"expiresAt":"2026-06-01T00:00:00Z","client":{"deviceType":"bot","ipAddress":"100.64.0.2"}},
		{"expiresAt":"2026-05-01T00:00:00Z","client":{"deviceType":"mobile","ipAddress":"100.64.0.2"}}
	]`
	var sessions []clientSession
	if err := json.Unmarshal([]byte(data), &sessions); err != nil {
		t.Fatal(err)
	}
	st := tailnet.Status{
		Self: tailnet.Node{UserID: 1},
		Peers: []tailnet.Node{
			{Name: "pixel", OS: "android", UserID: 1, IP: "100.64.0.2", IPv6: "fd7a:115c:a1e0::2"},
			{Name: "slate", OS: "iOS", UserID: 1, IP: "100.64.0.3"},
			{Name: "foreign", OS: "iOS", UserID: 2, IP: "100.64.0.4"},
			{Name: "tagged", OS: "iOS", UserID: 1, IP: "100.64.0.5", Tagged: true},
		},
	}
	got := phoneSummary(sessions, st, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))
	want := []Phone{{Name: "pixel", Kind: "phone", Count: 2}, {Name: "slate", Kind: "tablet", Count: 2}, {Kind: "phone", Count: 2}, {Kind: "tablet", Count: 1}}
	if !slices.Equal(got, want) {
		t.Errorf("phone summary = %#v, want %#v", got, want)
	}
}

func TestPhonesUnavailableWhenT3CannotRead(t *testing.T) {
	dir := t.TempDir()
	t3 := T3{Home: dir, Bin: filepath.Join(dir, "missing-t3")}
	if got, err := t3.Phones(context.Background(), nil); err == nil || got != nil || err.Error() != "couldn't read paired devices from T3 Code" {
		t.Fatalf("unexpected session list error or data: %v %v", got, err)
	}
}
