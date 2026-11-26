package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/api"
)

const antigravityAgent = "antigravity/1.11.5 windows/amd64"

var antigravityHosts = []string{"https://daily-cloudcode-pa.googleapis.com", "https://cloudcode-pa.googleapis.com"}

// effort strips "(High)" style suffixes: a model's effort levels share one quota.
var effort = regexp.MustCompile(`\s*\([^)]*\)\s*$`)

type modelQuota struct {
	name      string
	remaining float64
	resets    *time.Time
}

func antigravityUsage(ctx context.Context, c caller, f quotaFile) (usage, string) {
	project, _ := json.Marshal(struct {
		Project string `json:"project,omitempty"`
	}{f.ProjectID})
	var body, status string
	for _, host := range antigravityHosts {
		body, status = c.get(ctx, f, usageRequest{
			method: http.MethodPost, url: host + "/v1internal:fetchAvailableModels",
			headers: map[string]string{"User-Agent": antigravityAgent}, body: string(project),
		})
		if status == "" || status == "refused" {
			break
		}
	}
	if status != "" {
		return usage{}, status
	}
	u := usage{models: modelWindows(antigravityModels(body))}
	if len(u.models) == 0 {
		return u, "unavailable"
	}
	if tier, s := c.get(ctx, f, usageRequest{
		method: http.MethodPost, url: antigravityHosts[0] + "/v1internal:loadCodeAssist",
		headers: map[string]string{"User-Agent": antigravityAgent}, body: `{"metadata":{"ideType":"ANTIGRAVITY"}}`,
	}); s == "" {
		u.plan = tierName(tier)
	}
	return u, ""
}

func antigravityModels(body string) []modelQuota {
	var reply struct {
		Models map[string]struct {
			DisplayName string `json:"displayName"`
			Quota       *struct {
				Remaining *float64 `json:"remainingFraction"`
				ResetTime string   `json:"resetTime"`
			} `json:"quotaInfo"`
		} `json:"models"`
	}
	if json.Unmarshal([]byte(body), &reply) != nil {
		return nil
	}
	var models []modelQuota
	for _, m := range reply.Models {
		if m.DisplayName == "" || m.Quota == nil || m.Quota.Remaining == nil {
			continue
		}
		models = append(models, modelQuota{effort.ReplaceAllString(m.DisplayName, ""), *m.Quota.Remaining, isoTime(m.Quota.ResetTime)})
	}
	return models
}

func geminiUsage(ctx context.Context, c caller, f quotaFile) (usage, string) {
	project, _ := json.Marshal(struct {
		Project string `json:"project,omitempty"`
	}{f.ProjectID})
	body, status := c.get(ctx, f, usageRequest{
		method: http.MethodPost, url: "https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuota", body: string(project),
	})
	if status != "" {
		return usage{}, status
	}
	var reply struct {
		Buckets []struct {
			Model     string   `json:"modelId"`
			Remaining *float64 `json:"remainingFraction"`
			ResetTime string   `json:"resetTime"`
		} `json:"buckets"`
	}
	if json.Unmarshal([]byte(body), &reply) != nil {
		return usage{}, "unavailable"
	}
	var models []modelQuota
	for _, b := range reply.Buckets {
		if b.Model != "" && b.Remaining != nil {
			models = append(models, modelQuota{strings.TrimSuffix(b.Model, "_vertex"), *b.Remaining, isoTime(b.ResetTime)})
		}
	}
	u := usage{models: modelWindows(models)}
	if len(u.models) == 0 {
		return u, "unavailable"
	}
	return u, ""
}

// modelWindows keeps the lowest remaining quota per model name, and its soonest reset.
func modelWindows(models []modelQuota) []api.QuotaWindow {
	byName := map[string]modelQuota{}
	for _, m := range models {
		if m.remaining < 0 || m.remaining > 1 {
			continue
		}
		seen, ok := byName[m.name]
		if !ok || m.remaining < seen.remaining {
			byName[m.name] = m
		} else if m.remaining == seen.remaining && m.resets != nil && (seen.resets == nil || m.resets.Before(*seen.resets)) {
			seen.resets = m.resets
			byName[m.name] = seen
		}
	}
	windows := make([]api.QuotaWindow, 0, len(byName))
	for _, m := range byName {
		windows = append(windows, quotaWindow(m.name, (1-m.remaining)*100, m.resets, false))
	}
	slices.SortFunc(windows, func(a, b api.QuotaWindow) int { return strings.Compare(a.Label, b.Label) })
	return windows
}

func tierName(body string) string {
	type tier struct {
		Name string `json:"name"`
	}
	var reply struct {
		Current *tier `json:"currentTier"`
		Paid    *tier `json:"paidTier"`
	}
	if json.Unmarshal([]byte(body), &reply) != nil {
		return ""
	}
	if reply.Paid != nil && reply.Paid.Name != "" {
		return reply.Paid.Name
	}
	if reply.Current != nil {
		return reply.Current.Name
	}
	return ""
}
