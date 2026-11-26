package proxy

import (
	"testing"
)

func TestClaudeUsageReadsEveryLimitCreditAndShare(t *testing.T) {
	u := parseClaudeUsage(`{"five_hour":null,"seven_day":{"utilization":46.0,"resets_at":"2026-11-28T18:00:00.955215+00:00","locked_reason":null},
		"seven_day_opus":{"utilization":100,"resets_at":"2026-11-28T18:00:00Z"},
		"extra_usage":{"is_enabled":false,"monthly_limit":100,"used_credits":177.0,"currency":"USD","decimal_places":2,"disabled_reason":"org_level_disabled_until","spend_limit_reached":true},
		"limits":[{"kind":"weekly_all","group":"weekly","percent":46,"resets_at":"2026-11-28T18:00:00Z"},
			{"kind":"weekly_scoped","group":"weekly","percent":5,"severity":"normal","resets_at":"2026-11-28T17:59:59Z","scope":{"model":{"display_name":"Fable"}}}],
		"spend":{"used":{"amount_minor":177,"currency":"USD","exponent":2},"limit":{"amount_minor":100,"currency":"USD","exponent":2},"enabled":false,"disabled_reason":"org_level_disabled_until"},
		"seven_day_breakdown":{"rows":[{"display_name":"Claude Code","percent":100},{"display_name":"Chats","percent":0}]}}`)
	if len(u.windows) != 3 || u.windows[0].Label != "7-day" || u.windows[0].UsedPercent != 46 || u.windows[0].ResetsAt == nil ||
		u.windows[1].Label != "7-day · Opus" || !u.windows[1].Limited || u.windows[2].Label != "7-day · Fable" || u.windows[2].UsedPercent != 5 {
		t.Fatalf("windows: %+v", u.windows)
	}
	if len(u.credits) != 1 {
		t.Fatalf("credits: %+v", u.credits)
	}
	c := u.credits[0]
	if c.Unit != "USD" || *c.Used != 1.77 || *c.Limit != 1 || c.Note != "Turned off by the organization, monthly limit reached" {
		t.Errorf("credit: %+v used %v", c, *c.Used)
	}
	if len(u.shares) != 1 || u.shares[0].Label != "Claude Code" || u.shares[0].Percent != 100 {
		t.Errorf("shares: %+v", u.shares)
	}
}

func TestCodexUsageReadsPlanCreditsAndResets(t *testing.T) {
	u := parseCodexUsage(`{"email":"private","plan_type":"pro","rate_limit":{"limit_reached":false,"primary_window":{"used_percent":42,"limit_window_seconds":604800,"reset_after_seconds":100,"reset_at":0},"secondary_window":null},
		"code_review_rate_limit":{"primary_window":{"used_percent":3,"limit_window_seconds":18000,"reset_at":1796204202}},
		"credits":{"has_credits":true,"unlimited":false,"balance":"49836.0502425000","approx_local_messages":[12459,64787],"approx_cloud_messages":[1993,12459]},
		"rate_limit_reset_credits":{"available_count":4,"applicable_available_count":0}}`, 1000)
	if u.plan != "Pro" || len(u.windows) != 2 || u.windows[0].Label != "7-day" || u.windows[0].ResetsAt.Unix() != 1100 || u.windows[1].Label != "Code review · 5-hour" {
		t.Fatalf("plan and windows: %q %+v", u.plan, u.windows)
	}
	if len(u.credits) != 2 || *u.credits[0].Balance != 49836.0502425 || u.credits[0].Note != "about 12,459–64,787 local messages, 1,993–12,459 cloud messages" ||
		u.credits[1].Unit != "resets" || *u.credits[1].Balance != 4 || u.credits[1].Note != "0 usable now" {
		t.Errorf("credits: %+v", u.credits)
	}
}

func TestAntigravityModelsShareOneQuotaPerFamily(t *testing.T) {
	models := antigravityModels(`{"models":{
		"claude-opus-high":{"displayName":"Claude Opus 5.5 (High)","quotaInfo":{"remainingFraction":0.4,"resetTime":"2026-11-26T12:53:23Z"}},
		"claude-opus-low":{"displayName":"Claude Opus 5.5 (Low)","quotaInfo":{"remainingFraction":0.4,"resetTime":"2026-11-26T11:00:00Z"}},
		"gemini-flash":{"displayName":"Gemini 3 Flash","quotaInfo":{"remainingFraction":1}},
		"tab_internal":{"quotaInfo":{"remainingFraction":1}},
		"no-quota":{"displayName":"No Quota"}}}`)
	w := modelWindows(models)
	if len(w) != 2 || w[0].Label != "Claude Opus 5.5" || w[0].UsedPercent != 60 || w[0].ResetsAt.Hour() != 11 || w[1].Label != "Gemini 3 Flash" || w[1].UsedPercent != 0 || w[1].ResetsAt != nil {
		t.Fatalf("models: %+v", w)
	}
	if tierName(`{"currentTier":{"name":"Antigravity"},"paidTier":{"name":"Google AI Pro"}}`) != "Google AI Pro" || tierName(`{"currentTier":{"name":"Free"}}`) != "Free" {
		t.Error("tier name")
	}
}

func TestClaudePlanNamesTheSubscriptionWithoutIdentity(t *testing.T) {
	cases := map[string]string{
		`{"account":{"email":"private"},"organization":{"organization_type":"claude_max","rate_limit_tier":"default_claude_max_20x","plan_display_name":"Ultra","subscription_status":"active"}}`: "Ultra",
		`{"organization":{"organization_type":"claude_max","rate_limit_tier":"default_claude_max_5x"}}`:                                                                                           "Max 5x",
		`{"organization":{"organization_type":"claude_pro","subscription_status":"past_due"}}`:                                                                                                    "Pro (past due)",
		`not json`: "",
	}
	for body, want := range cases {
		if got := claudePlan(body); got != want {
			t.Errorf("claudePlan(%s) = %q, want %q", body, got, want)
		}
	}
}
