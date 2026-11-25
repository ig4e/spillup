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
	Windows     []QuotaWindow `json:"windows,omitempty"`
}

type QuotaWindow struct {
	Label       string     `json:"label"`
	UsedPercent float64    `json:"used_percent"`
	ResetsAt    *time.Time `json:"resets_at,omitempty"`
}
