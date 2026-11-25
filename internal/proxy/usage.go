package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/darkyeg/spinup/internal/api"
)

// Older proxies and idle accounts have no passive quota windows. Ask the same fixed usage endpoints
// as CLIProxyAPI's Management Center. Only the holder chooses a credential and destination.
func fillUsage(ctx context.Context, port int, password string, files []quotaFile, s api.QuotaSummary) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	tasks := make(chan int)
	var workers sync.WaitGroup
	for range min(3, len(files)) {
		workers.Go(func() {
			for i := range tasks {
				a := &s.Accounts[i]
				if a.ObservedAt != nil && s.CheckedAt.Sub(*a.ObservedAt) < 5*time.Minute {
					continue
				}
				if a.State == "disabled" || a.State == "sign-in" || files[i].AuthIndex == "" || (a.Provider != "claude" && a.Provider != "codex") {
					continue
				}
				windows, status := usageWindows(ctx, port, password, a.Provider, files[i])
				a.UsageStatus = status
				if len(windows) > 0 {
					now := time.Now().UTC()
					a.ObservedAt, a.Windows = &now, windows
				}
			}
		})
	}
feed:
	for i := range files {
		select {
		case tasks <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(tasks)
	workers.Wait()
}

func usageWindows(ctx context.Context, port int, password, provider string, f quotaFile) ([]api.QuotaWindow, string) {
	headers := map[string]string{"Authorization": "Bearer $TOKEN$", "Content-Type": "application/json"}
	var url string
	switch provider {
	case "claude":
		url = "https://api.anthropic.com/api/oauth/usage"
		headers["anthropic-beta"] = "oauth-2025-04-20"
	case "codex":
		url = "https://chatgpt.com/backend-api/wham/usage"
		var claims struct {
			AccountID string `json:"chatgpt_account_id"`
		}
		if json.Unmarshal(f.IDToken, &claims) == nil && claims.AccountID != "" {
			headers["Chatgpt-Account-Id"] = claims.AccountID
		}
	default:
		return nil, "unavailable"
	}
	payload, err := json.Marshal(struct {
		AuthIndex string            `json:"auth_index"`
		Method    string            `json:"method"`
		URL       string            `json:"url"`
		Header    map[string]string `json:"header"`
	}{f.AuthIndex, http.MethodGet, url, headers})
	if err != nil {
		return nil, "unavailable"
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	var reply struct {
		StatusCode int    `json:"status_code"`
		Body       string `json:"body"`
	}
	if err := readManagement(ctx, port, password, http.MethodPost, "api-call", bytes.NewReader(payload), 1<<20, &reply); err != nil {
		return nil, "unavailable"
	}
	switch reply.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, "refused"
	case http.StatusTooManyRequests:
		return nil, "rate-limited"
	case http.StatusOK:
		var windows []api.QuotaWindow
		if provider == "codex" {
			windows = codexUsage(reply.Body)
		} else {
			windows = claudeUsage(reply.Body)
		}
		if len(windows) > 0 {
			return windows, ""
		}
	}
	return nil, "unavailable"
}

func claudeUsage(body string) []api.QuotaWindow {
	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return nil
	}
	var windows []api.QuotaWindow
	for _, w := range []struct{ key, label string }{{"five_hour", "5h"}, {"seven_day", "7d"}} {
		var item struct {
			Utilization *float64 `json:"utilization"`
			ResetsAt    string   `json:"resets_at"`
		}
		if json.Unmarshal(data[w.key], &item) != nil || !validPercent(item.Utilization) {
			continue
		}
		win := api.QuotaWindow{Label: w.label, UsedPercent: math.Round(*item.Utilization*10) / 10}
		if t, err := time.Parse(time.RFC3339, item.ResetsAt); err == nil {
			win.ResetsAt = &t
		}
		windows = append(windows, win)
	}
	return windows
}

func codexUsage(body string) []api.QuotaWindow {
	type window struct {
		UsedPercent *float64 `json:"used_percent"`
		Seconds     int      `json:"limit_window_seconds"`
		Reset       int64    `json:"reset_at"`
		After       int      `json:"reset_after_seconds"`
	}
	var data struct {
		RateLimit struct {
			Primary   window `json:"primary_window"`
			Secondary window `json:"secondary_window"`
		} `json:"rate_limit"`
	}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return nil
	}
	var windows []api.QuotaWindow
	for i, w := range []window{data.RateLimit.Primary, data.RateLimit.Secondary} {
		if !validPercent(w.UsedPercent) {
			continue
		}
		label := []string{"primary", "secondary"}[i]
		if w.Seconds >= 60 && w.Seconds <= 366*24*60*60 {
			label = durationLabel(w.Seconds / 60)
		}
		win := api.QuotaWindow{Label: label, UsedPercent: math.Round(*w.UsedPercent*10) / 10}
		if w.Reset == 0 && w.After > 0 && w.After < 366*24*60*60 {
			w.Reset = time.Now().Unix() + int64(w.After)
		}
		if w.Reset > 0 && w.Reset <= 253402300799 {
			t := time.Unix(w.Reset, 0).UTC()
			win.ResetsAt = &t
		}
		windows = append(windows, win)
	}
	return windows
}

func validPercent(v *float64) bool { return v != nil && *v >= 0 && *v <= 100 }
