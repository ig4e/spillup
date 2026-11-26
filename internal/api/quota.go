package api

import "time"

const PathQuotas = "/spinup/quotas"

// QuotaSummary contains only the account holder's read-only, anonymous observations.
type QuotaSummary struct {
	CheckedAt time.Time      `json:"checked_at"`
	Accounts  []QuotaAccount `json:"accounts"`
}

type QuotaAccount struct {
	Provider    string        `json:"provider"`
	UsageStatus string        `json:"usage_status,omitempty"`
	State       string        `json:"state"`
	ObservedAt  *time.Time    `json:"observed_at,omitempty"`
	Plan        string        `json:"plan,omitempty"`
	Windows     []QuotaWindow `json:"windows,omitempty"`
	Models      []QuotaWindow `json:"models,omitempty"`
	Credits     []QuotaCredit `json:"credits,omitempty"`
	Shares      []QuotaShare  `json:"shares,omitempty"`
}

type QuotaWindow struct {
	Label       string     `json:"label"`
	UsedPercent float64    `json:"used_percent"`
	ResetsAt    *time.Time `json:"resets_at,omitempty"`
	Limited     bool       `json:"limited,omitempty"`
}

// QuotaCredit is money or credits beyond the plan's windows. Unit is an ISO currency, "credits" or "resets".
type QuotaCredit struct {
	Label   string   `json:"label"`
	Unit    string   `json:"unit"`
	Used    *float64 `json:"used,omitempty"`
	Limit   *float64 `json:"limit,omitempty"`
	Balance *float64 `json:"balance,omitempty"`
	Note    string   `json:"note,omitempty"`
}

// QuotaShare is how much of one window a product used.
type QuotaShare struct {
	Label   string  `json:"label"`
	Percent float64 `json:"percent"`
}
