package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/api"
)

type codexWindow struct {
	UsedPercent *float64 `json:"used_percent"`
	Seconds     int      `json:"limit_window_seconds"`
	Reset       int64    `json:"reset_at"`
	After       int64    `json:"reset_after_seconds"`
}

type codexLimit struct {
	Reached   bool         `json:"limit_reached"`
	Primary   *codexWindow `json:"primary_window"`
	Secondary *codexWindow `json:"secondary_window"`
}

type codexReply struct {
	Plan       string      `json:"plan_type"`
	RateLimit  *codexLimit `json:"rate_limit"`
	CodeReview *codexLimit `json:"code_review_rate_limit"`
	Additional []struct {
		Name      string      `json:"limit_name"`
		Feature   string      `json:"metered_feature"`
		RateLimit *codexLimit `json:"rate_limit"`
	} `json:"additional_rate_limits"`
	Credits *struct {
		HasCredits bool    `json:"has_credits"`
		Unlimited  bool    `json:"unlimited"`
		Balance    string  `json:"balance"`
		Local      []int64 `json:"approx_local_messages"`
		Cloud      []int64 `json:"approx_cloud_messages"`
	} `json:"credits"`
	SpendControl *struct {
		Reached bool `json:"reached"`
	} `json:"spend_control"`
	Resets *struct {
		Available  int `json:"available_count"`
		Applicable int `json:"applicable_available_count"`
	} `json:"rate_limit_reset_credits"`
}

func codexUsage(ctx context.Context, c caller, f quotaFile) (usage, string) {
	headers := map[string]string{}
	var claims struct {
		AccountID string `json:"chatgpt_account_id"`
	}
	if json.Unmarshal(f.IDToken, &claims) == nil && claims.AccountID != "" {
		headers["Chatgpt-Account-Id"] = claims.AccountID
	}
	body, status := c.get(ctx, f, usageRequest{method: http.MethodGet, url: "https://chatgpt.com/backend-api/wham/usage", headers: headers})
	if status != "" {
		return usage{}, status
	}
	u := parseCodexUsage(body, time.Now().Unix())
	if u.empty() {
		return u, "unavailable"
	}
	return u, ""
}

func parseCodexUsage(body string, now int64) usage {
	var r codexReply
	if json.Unmarshal([]byte(body), &r) != nil {
		return usage{}
	}
	u := usage{plan: planName(r.Plan)}
	u.windows = codexWindows(u.windows, "", r.RateLimit, now)
	u.windows = codexWindows(u.windows, "Code review", r.CodeReview, now)
	for _, a := range r.Additional {
		name := a.Name
		if name == "" {
			name = a.Feature
		}
		u.windows = codexWindows(u.windows, name, a.RateLimit, now)
	}
	if r.Credits != nil && (r.Credits.HasCredits || r.Credits.Unlimited || r.Credits.Balance != "") {
		credit := api.QuotaCredit{Label: "Credits", Unit: "credits"}
		if b, err := strconv.ParseFloat(r.Credits.Balance, 64); err == nil && b >= 0 {
			credit.Balance = number(b)
		}
		var notes []string
		if r.Credits.Unlimited {
			notes = append(notes, "Unlimited")
		}
		if m := messageRange(r.Credits.Local); m != "" {
			notes = append(notes, "about "+m+" local messages")
		}
		if m := messageRange(r.Credits.Cloud); m != "" {
			notes = append(notes, m+" cloud messages")
		}
		if r.SpendControl != nil && r.SpendControl.Reached {
			notes = append(notes, "spend limit reached")
		}
		credit.Note = strings.Join(notes, ", ")
		u.credits = append(u.credits, credit)
	}
	if r.Resets != nil && r.Resets.Available > 0 {
		u.credits = append(u.credits, api.QuotaCredit{
			Label: "Limit resets", Unit: "resets", Balance: number(float64(r.Resets.Available)),
			Note: fmt.Sprintf("%d usable now", r.Resets.Applicable),
		})
	}
	return u
}

func codexWindows(windows []api.QuotaWindow, name string, l *codexLimit, now int64) []api.QuotaWindow {
	if l == nil {
		return windows
	}
	for i, w := range []*codexWindow{l.Primary, l.Secondary} {
		if w == nil || !validPercent(w.UsedPercent) {
			continue
		}
		label := []string{"Primary", "Secondary"}[i]
		if w.Seconds >= 60 && w.Seconds <= 366*24*60*60 {
			label = durationLabel(w.Seconds / 60)
		}
		if name != "" {
			label = name + " · " + label
		}
		reset := w.Reset
		if reset == 0 && w.After > 0 && w.After < 366*24*60*60 {
			reset = now + w.After
		}
		windows = append(windows, quotaWindow(label, *w.UsedPercent, unixTime(reset), l.Reached && *w.UsedPercent >= 100))
	}
	return windows
}

func messageRange(r []int64) string {
	switch {
	case len(r) == 2 && r[0] > 0 && r[1] > r[0]:
		return thousands(r[0]) + "–" + thousands(r[1])
	case len(r) >= 1 && r[0] > 0:
		return thousands(r[0])
	}
	return ""
}

func thousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func planName(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	return strings.ToUpper(p[:1]) + p[1:]
}
