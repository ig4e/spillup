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

// usage is one account's reading from its provider's usage endpoint.
type usage struct {
	plan    string
	windows []api.QuotaWindow
	models  []api.QuotaWindow
	credits []api.QuotaCredit
	shares  []api.QuotaShare
}

func (u usage) empty() bool {
	return len(u.windows) == 0 && len(u.models) == 0 && len(u.credits) == 0
}

// usageReaders ask the same fixed usage endpoints as CLIProxyAPI's Management Center.
var usageReaders = map[string]func(context.Context, caller, quotaFile) (usage, string){
	"claude":      claudeUsage,
	"codex":       codexUsage,
	"antigravity": antigravityUsage,
	"gemini":      geminiUsage,
}

// fillUsage replaces passive observations with a full reading where the provider has a usage endpoint.
// Only the holder chooses a credential and destination.
func fillUsage(ctx context.Context, port int, password string, files []quotaFile, s api.QuotaSummary) {
	ctx, cancel := context.WithTimeout(ctx, 14*time.Second)
	defer cancel()
	call := caller{port: port, password: password}
	tasks := make(chan int)
	var workers sync.WaitGroup
	for range min(3, len(files)) {
		workers.Go(func() {
			for i := range tasks {
				a := &s.Accounts[i]
				read, ok := usageReaders[a.Provider]
				if !ok || a.State == "disabled" || a.State == "sign-in" || files[i].AuthIndex == "" {
					continue
				}
				u, status := read(ctx, call, files[i])
				a.UsageStatus = status
				if u.empty() {
					continue
				}
				now := time.Now().UTC()
				a.ObservedAt, a.Plan, a.Credits, a.Shares, a.Models = &now, u.plan, u.credits, u.shares, u.models
				if len(u.windows) > 0 {
					a.Windows = u.windows
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

// caller sends one request with an account's token through CLIProxyAPI's api-call.
type caller struct {
	port     int
	password string
}

type usageRequest struct {
	method  string
	url     string
	headers map[string]string
	body    string
}

// get returns the provider's body for a 200, or a usage status naming why there is none.
func (c caller) get(ctx context.Context, f quotaFile, r usageRequest) (string, string) {
	headers := map[string]string{"Authorization": "Bearer $TOKEN$", "Content-Type": "application/json"}
	for k, v := range r.headers {
		headers[k] = v
	}
	payload, err := json.Marshal(struct {
		AuthIndex string            `json:"auth_index"`
		Method    string            `json:"method"`
		URL       string            `json:"url"`
		Header    map[string]string `json:"header"`
		Data      string            `json:"data,omitempty"`
	}{f.AuthIndex, r.method, r.url, headers, r.body})
	if err != nil {
		return "", "unavailable"
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	var reply struct {
		StatusCode int    `json:"status_code"`
		Body       string `json:"body"`
	}
	if err := readManagement(ctx, c.port, c.password, http.MethodPost, "api-call", bytes.NewReader(payload), 4<<20, &reply); err != nil {
		return "", "unavailable"
	}
	switch reply.StatusCode {
	case http.StatusOK:
		return reply.Body, ""
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", "refused"
	case http.StatusTooManyRequests:
		return "", "rate-limited"
	}
	return "", "unavailable"
}

func validPercent(v *float64) bool { return v != nil && *v >= 0 && *v <= 100 }

func roundPercent(v float64) float64 { return math.Round(v*10) / 10 }

func unixTime(sec int64) *time.Time {
	if sec <= 0 || sec > 253402300799 {
		return nil
	}
	t := time.Unix(sec, 0).UTC()
	return &t
}

func isoTime(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	t = t.UTC()
	return &t
}

func number(v float64) *float64 { return &v }

func quotaWindow(label string, used float64, resets *time.Time, limited bool) api.QuotaWindow {
	return api.QuotaWindow{Label: label, UsedPercent: roundPercent(used), ResetsAt: resets, Limited: limited || used >= 100}
}
