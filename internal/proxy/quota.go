package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/route"
)

type quotaFile struct {
	LoginState
	Provider  string          `json:"provider"`
	Type      string          `json:"type"`
	AuthIndex string          `json:"auth_index"`
	IDToken   json.RawMessage `json:"id_token"`
	Quota     struct {
		ObservedAt time.Time         `json:"observed_at"`
		Signals    map[string]string `json:"signals"`
	} `json:"quota"`
}

// ReadQuotas projects CLIProxyAPI's passive response observations into the worker-visible fields.
func ReadQuotas(ctx context.Context, port int, password string) (api.QuotaSummary, error) {
	var listing struct {
		Files []quotaFile `json:"files"`
	}
	if err := readAuthFiles(ctx, port, password, &listing); err != nil {
		return api.QuotaSummary{}, err
	}
	if listing.Files == nil {
		return api.QuotaSummary{}, errors.New("CLIProxyAPI did not return its account list")
	}
	s := api.QuotaSummary{CheckedAt: time.Now().UTC(), Accounts: make([]api.QuotaAccount, 0, len(listing.Files))}
	for _, f := range listing.Files {
		s.Accounts = append(s.Accounts, f.summary())
	}
	fillUsage(ctx, port, password, listing.Files, s)
	return s, nil
}

func (f quotaFile) summary() api.QuotaAccount {
	p := route.Provider(strings.ToLower(strings.TrimSpace(f.Provider)))
	if p == "" {
		p = route.Provider(strings.ToLower(strings.TrimSpace(f.Type)))
	}
	if _, known := route.SignInFor(p); !known && p != "gemini" && p != "copilot" {
		p = "other"
	}
	a := api.QuotaAccount{Provider: string(p), State: accountState(f.LoginState)}
	if f.Quota.ObservedAt.IsZero() {
		return a
	}
	h := http.Header{}
	for k, v := range f.Quota.Signals {
		h.Set(k, v)
	}
	switch p {
	case "codex":
		for _, window := range []string{"primary", "secondary"} {
			prefix := "X-Codex-" + window
			label := window
			if mins, err := strconv.Atoi(h.Get(prefix + "-window-minutes")); err == nil && mins > 0 && mins <= 60*24*366 {
				label = durationLabel(mins)
			}
			reset := h.Get(prefix + "-reset-at")
			if reset == "" {
				if after, err := strconv.ParseInt(h.Get(prefix+"-reset-after-seconds"), 10, 32); err == nil && after > 0 {
					reset = strconv.FormatInt(f.Quota.ObservedAt.Unix()+after, 10)
				}
			}
			a.Windows = addWindow(a.Windows, label, h.Get(prefix+"-used-percent"), reset, 1)
		}
	case "claude":
		for _, window := range []string{"5h", "7d"} {
			prefix := "Anthropic-Ratelimit-Unified-" + window
			a.Windows = addWindow(a.Windows, window, h.Get(prefix+"-utilization"), h.Get(prefix+"-reset"), 100)
		}
	}
	if len(a.Windows) > 0 {
		a.ObservedAt = &f.Quota.ObservedAt
	}
	return a
}

func accountState(s LoginState) string {
	msg := strings.ToLower(s.StatusMessage)
	switch {
	case s.Disabled || strings.EqualFold(s.Status, "disabled"):
		return "disabled"
	case s.Refused():
		return "sign-in"
	case strings.Contains(msg, "429") || strings.Contains(msg, "quota"):
		return "limited"
	case s.Unavailable || strings.EqualFold(s.Status, "error"):
		return "unavailable"
	case strings.EqualFold(s.Status, "active"):
		return "ready"
	default:
		return "unknown"
	}
}

func durationLabel(mins int) string {
	if mins%(60*24) == 0 {
		return strconv.Itoa(mins/(60*24)) + "d"
	}
	if mins%60 == 0 {
		return strconv.Itoa(mins/60) + "h"
	}
	return strconv.Itoa(mins) + "m"
}

func addWindow(windows []api.QuotaWindow, label, used, reset string, scale float64) []api.QuotaWindow {
	n, err := strconv.ParseFloat(used, 64)
	pct := n * scale
	if err != nil || math.IsNaN(pct) || pct < 0 || pct > 100 {
		return windows
	}
	w := api.QuotaWindow{Label: label, UsedPercent: math.Round(pct*10) / 10}
	if sec, err := strconv.ParseInt(reset, 10, 64); err == nil && sec > 0 && sec <= 253402300799 {
		at := time.Unix(sec, 0).UTC()
		w.ResetsAt = &at
	}
	return append(windows, w)
}
