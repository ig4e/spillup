package t3code

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/tailnet"
)

// Phone is the part of an active T3 client session the dashboard can share.
type Phone struct {
	Name  string `json:"name,omitempty"`
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

type clientSession struct {
	ExpiresAt time.Time `json:"expiresAt"`
	Client    struct {
		Type string `json:"deviceType"`
		IP   string `json:"ipAddress"`
	} `json:"client"`
}

// Phones reads T3's active sessions separately from its linked agent environments.
func (t T3) Phones(ctx context.Context, network tailnet.Source) ([]Phone, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out, err := t.run(ctx, "auth", "session", "list", "--json")
	var sessions []clientSession
	if err != nil || json.Unmarshal(out, &sessions) != nil || sessions == nil {
		return nil, errors.New("couldn't read paired devices from T3 Code")
	}
	var st tailnet.Status
	if len(sessions) > 0 {
		st, _ = network.Status(ctx)
	}
	return phoneSummary(sessions, st, time.Now()), nil
}

func phoneSummary(sessions []clientSession, st tailnet.Status, now time.Time) []Phone {
	peers := make(map[netip.Addr]string)
	for _, peer := range st.Peers {
		if !st.SameOwner(peer) || !tailnet.ValidName(peer.Name) || !peer.Mobile() {
			continue
		}
		for _, raw := range []string{peer.IP, peer.IPv6} {
			if ip, err := netip.ParseAddr(raw); err == nil {
				peers[ip.Unmap()] = peer.Name
			}
		}
	}
	rows := make(map[string]Phone)
	for _, session := range sessions {
		if !session.ExpiresAt.After(now) {
			continue
		}
		ip, _ := netip.ParseAddr(session.Client.IP)
		name := peers[ip.Unmap()]
		kind := "phone"
		switch session.Client.Type {
		case "tablet":
			kind = "tablet"
		case "mobile":
		case "", "unknown", "desktop":
			if name == "" {
				continue
			}
		default:
			continue
		}
		key := name
		if key == "" {
			key = ":" + kind
		}
		row := rows[key]
		if row.Kind != "tablet" {
			row.Kind = kind
		}
		row.Name = name
		row.Count++
		rows[key] = row
	}
	phones := make([]Phone, 0, len(rows))
	for _, row := range rows {
		phones = append(phones, row)
	}
	slices.SortFunc(phones, func(a, b Phone) int {
		if a.Name == b.Name {
			return strings.Compare(a.Kind, b.Kind)
		}
		if a.Name == "" {
			return 1
		}
		if b.Name == "" {
			return -1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return phones
}
