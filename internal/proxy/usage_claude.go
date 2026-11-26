package proxy

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"slices"
	"strings"

	"github.com/darkyeg/spinup/internal/api"
)

var claudeWindows = []struct{ key, label string }{
	{"five_hour", "5-hour"},
	{"seven_day", "7-day"},
	{"seven_day_opus", "7-day · Opus"},
	{"seven_day_sonnet", "7-day · Sonnet"},
	{"seven_day_oauth_apps", "7-day · OAuth apps"},
	{"seven_day_cowork", "7-day · Cowork"},
}

type claudeWindow struct {
	Utilization  *float64 `json:"utilization"`
	ResetsAt     string   `json:"resets_at"`
	LockedReason *string  `json:"locked_reason"`
}

type claudeLimit struct {
	Group    string   `json:"group"`
	Percent  *float64 `json:"percent"`
	Severity string   `json:"severity"`
	ResetsAt string   `json:"resets_at"`
	Scope    *struct {
		Model *struct {
			DisplayName string `json:"display_name"`
		} `json:"model"`
	} `json:"scope"`
}

type claudeMoney struct {
	AmountMinor *float64 `json:"amount_minor"`
	Currency    string   `json:"currency"`
	Exponent    int      `json:"exponent"`
}

func (m *claudeMoney) value() *float64 {
	if m == nil || m.AmountMinor == nil || m.Exponent < 0 || m.Exponent > 6 {
		return nil
	}
	return number(*m.AmountMinor / math.Pow10(m.Exponent))
}

type claudeReply struct {
	ExtraUsage *struct {
		Enabled           bool     `json:"is_enabled"`
		MonthlyLimit      *float64 `json:"monthly_limit"`
		UsedCredits       *float64 `json:"used_credits"`
		Currency          string   `json:"currency"`
		DecimalPlaces     int      `json:"decimal_places"`
		DisabledReason    string   `json:"disabled_reason"`
		UserDisabled      bool     `json:"user_disabled"`
		SpendLimitReached bool     `json:"spend_limit_reached"`
	} `json:"extra_usage"`
	Spend *struct {
		Used           *claudeMoney `json:"used"`
		Limit          *claudeMoney `json:"limit"`
		Balance        *claudeMoney `json:"balance"`
		Enabled        bool         `json:"enabled"`
		DisabledReason string       `json:"disabled_reason"`
	} `json:"spend"`
	Limits    []claudeLimit `json:"limits"`
	Breakdown *struct {
		Rows []struct {
			Name    string   `json:"display_name"`
			Percent *float64 `json:"percent"`
		} `json:"rows"`
	} `json:"seven_day_breakdown"`
}

func claudeUsage(ctx context.Context, c caller, f quotaFile) (usage, string) {
	body, status := c.get(ctx, f, usageRequest{
		method:  http.MethodGet,
		url:     "https://api.anthropic.com/api/oauth/usage",
		headers: map[string]string{"anthropic-beta": "oauth-2025-04-20"},
	})
	if status != "" {
		return usage{}, status
	}
	u := parseClaudeUsage(body)
	if u.empty() {
		return u, "unavailable"
	}
	if profile, s := c.get(ctx, f, usageRequest{
		method:  http.MethodGet,
		url:     "https://api.anthropic.com/api/oauth/profile",
		headers: map[string]string{"anthropic-beta": "oauth-2025-04-20"},
	}); s == "" {
		u.plan = claudePlan(profile)
	}
	return u, ""
}

var claudeTiers = map[string]string{"claude_max": "Max", "claude_pro": "Pro", "claude_team": "Team", "claude_enterprise": "Enterprise"}

// claudePlan names the subscription as Claude does, e.g. "Max 20x"; it reads no identity.
func claudePlan(body string) string {
	var reply struct {
		Organization struct {
			Type        string `json:"organization_type"`
			Tier        string `json:"rate_limit_tier"`
			DisplayName string `json:"plan_display_name"`
			Status      string `json:"subscription_status"`
		} `json:"organization"`
	}
	if json.Unmarshal([]byte(body), &reply) != nil {
		return ""
	}
	o := reply.Organization
	plan := o.DisplayName
	if plan == "" {
		plan = claudeTiers[o.Type]
		for _, x := range []string{"20x", "5x"} {
			if plan != "" && strings.HasSuffix(o.Tier, "_"+x) {
				plan += " " + x
			}
		}
	}
	if plan != "" && o.Status != "" && o.Status != "active" {
		plan += " (" + strings.ReplaceAll(o.Status, "_", " ") + ")"
	}
	return plan
}

func parseClaudeUsage(body string) usage {
	var raw map[string]json.RawMessage
	var reply claudeReply
	if json.Unmarshal([]byte(body), &raw) != nil || json.Unmarshal([]byte(body), &reply) != nil {
		return usage{}
	}
	var u usage
	for _, w := range claudeWindows {
		var item claudeWindow
		if json.Unmarshal(raw[w.key], &item) != nil || !validPercent(item.Utilization) {
			continue
		}
		u.windows = append(u.windows, quotaWindow(w.label, *item.Utilization, isoTime(item.ResetsAt), item.LockedReason != nil))
	}
	for _, l := range reply.Limits {
		label := claudeLimitLabel(l)
		if label == "" || !validPercent(l.Percent) || slices.ContainsFunc(u.windows, func(w api.QuotaWindow) bool { return w.Label == label }) {
			continue
		}
		u.windows = append(u.windows, quotaWindow(label, *l.Percent, isoTime(l.ResetsAt), l.Severity == "critical"))
	}
	if credit, ok := claudeCredit(reply); ok {
		u.credits = append(u.credits, credit)
	}
	if reply.Breakdown != nil {
		for _, r := range reply.Breakdown.Rows {
			if r.Name != "" && validPercent(r.Percent) && *r.Percent > 0 {
				u.shares = append(u.shares, api.QuotaShare{Label: r.Name, Percent: roundPercent(*r.Percent)})
			}
		}
	}
	return u
}

func claudeLimitLabel(l claudeLimit) string {
	var label string
	switch l.Group {
	case "weekly":
		label = "7-day"
	case "session", "five_hour":
		label = "5-hour"
	default:
		return ""
	}
	if l.Scope != nil && l.Scope.Model != nil && l.Scope.Model.DisplayName != "" {
		label += " · " + l.Scope.Model.DisplayName
	}
	return label
}

// claudeCredit is the extra usage that pays past the plan's limits, in money.
func claudeCredit(r claudeReply) (api.QuotaCredit, bool) {
	credit := api.QuotaCredit{Label: "Extra usage this month"}
	var enabled, userOff, reached bool
	var reason string
	switch {
	case r.Spend != nil && r.Spend.Used != nil:
		credit.Unit, credit.Used, credit.Limit, credit.Balance = r.Spend.Used.Currency, r.Spend.Used.value(), r.Spend.Limit.value(), r.Spend.Balance.value()
		enabled, reason = r.Spend.Enabled, r.Spend.DisabledReason
	case r.ExtraUsage != nil && r.ExtraUsage.UsedCredits != nil:
		e := r.ExtraUsage
		scale := math.Pow10(min(max(e.DecimalPlaces, 0), 6))
		credit.Unit, credit.Used = e.Currency, number(*e.UsedCredits/scale)
		if e.MonthlyLimit != nil {
			credit.Limit = number(*e.MonthlyLimit / scale)
		}
		enabled, reason, userOff = e.Enabled, e.DisabledReason, e.UserDisabled
	default:
		return credit, false
	}
	if r.ExtraUsage != nil {
		reached, userOff = r.ExtraUsage.SpendLimitReached, r.ExtraUsage.UserDisabled
	}
	if credit.Unit == "" {
		credit.Unit = "USD"
	}
	var notes []string
	switch {
	case enabled:
	case strings.HasPrefix(reason, "org_level"):
		notes = append(notes, "Turned off by the organization")
	case userOff:
		notes = append(notes, "Turned off")
	default:
		notes = append(notes, "Off")
	}
	if reached {
		notes = append(notes, "monthly limit reached")
	}
	credit.Note = strings.Join(notes, ", ")
	return credit, true
}
