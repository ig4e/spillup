package webui

import (
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/logins"
)

// Overview is this machine and its accounts service, for the dashboard's first page.
type Overview struct {
	Installed bool        `json:"installed"`
	Name      string      `json:"name"`
	Hold      config.Hold `json:"hold"`
	OS        string      `json:"os"`
	Version   string      `json:"version"`
	// CanManage: this machine keeps the dashboard password, so it may move the accounts and change roles.
	CanManage      bool        `json:"can_manage"`
	Service        *api.Report `json:"service,omitempty"`
	ServiceProblem string      `json:"service_problem,omitempty"`
	Settings       Settings    `json:"settings"`
	RouterURL      string      `json:"router_url"`
	DashboardURL   string      `json:"dashboard_url,omitempty"`
}

type Settings struct {
	Port                 int    `json:"port"`
	ProxyPort            int    `json:"proxy_port"`
	FailoverAfterSeconds int    `json:"failover_after_seconds"`
	AutoFailback         bool   `json:"auto_failback"`
	Library              string `json:"library,omitempty"`
}

// Machine is one device on the tailnet, with what the accounts service knows of it.
type Machine struct {
	Name      string     `json:"name"`
	DNSName   string     `json:"dns_name,omitempty"`
	IP        string     `json:"ip,omitempty"`
	OS        string     `json:"os,omitempty"`
	Online    bool       `json:"online"`
	LastSeen  *time.Time `json:"last_seen,omitempty"`
	KeyExpiry *time.Time `json:"key_expiry,omitempty"`
	Expired   bool       `json:"expired,omitempty"`
	IsSelf    bool       `json:"is_self,omitempty"`
	// Hold is known only for machines that can hold the accounts; the rest just use them.
	Hold    config.Hold `json:"hold,omitempty"`
	State   string      `json:"state,omitempty"`
	Leading bool        `json:"leading,omitempty"`
}

type Machines struct {
	Machines []Machine `json:"machines"`
	Problem  string    `json:"problem,omitempty"`
}

type Check struct {
	Level string `json:"level"`
	Label string `json:"label"`
	Fix   string `json:"fix,omitempty"`
}

type Section struct {
	Title  string  `json:"title"`
	Checks []Check `json:"checks"`
}

type Doctor struct {
	Sections []Section `json:"sections"`
	Failures int       `json:"failures"`
}

// Skills is the three tabs of `spinup skills`: shared, only on this machine, and removed.
type Skills struct {
	Library       string        `json:"library"`
	TokenEstimate int           `json:"token_estimate"`
	Shared        []SharedSkill `json:"shared"`
	OnlyHere      []LocalSkill  `json:"only_here"`
	Removed       []Removed     `json:"removed"`
	Clashes       []Clash       `json:"clashes,omitempty"`
}

type SharedSkill struct {
	Name             string   `json:"name"`
	Source           string   `json:"source"`
	Mode             string   `json:"mode"`
	Installed        bool     `json:"installed"`
	DescriptionChars int      `json:"description_chars"`
	Agents           []string `json:"agents"`
}

// LocalSkill is on this machine only: installed by spinup but not in the library, or an agent's own folder.
type LocalSkill struct {
	Name     string   `json:"name"`
	AgentOwn bool     `json:"agent_own"`
	Agents   []string `json:"agents"`
	Path     string   `json:"path,omitempty"`
}

type Removed struct {
	Name    string    `json:"name"`
	Was     string    `json:"was"`
	Removed time.Time `json:"removed"`
}

type Clash struct {
	Name    string   `json:"name"`
	Folders []string `json:"folders"`
}

type Keys struct {
	APIKey             string `json:"api_key"`
	ManagementPassword string `json:"management_password,omitempty"`
}

// Done answers a change: what happened, in a sentence the UI shows as is.
type Done struct {
	Message string `json:"message"`
}

type handoffRequest struct {
	To string `json:"to"`
}

type mainRequest struct {
	Machine string `json:"machine"`
}

type skillsRequest struct {
	Names []string `json:"names"`
}

type modeRequest struct {
	Names  []string `json:"names"`
	Manual bool     `json:"manual"`
}

type addRequest struct {
	Repo   string   `json:"repo"`
	Names  []string `json:"names"`
	Manual bool     `json:"manual"`
}

type nameRequest struct {
	Name string `json:"name"`
}

// OwnState is the providers this machine signs in to by itself, those sign-ins, and every provider it could.
type OwnState struct {
	Providers []string         `json:"providers"`
	Available []ProviderInfo   `json:"available"`
	Running   bool             `json:"running"`
	Logins    []logins.Summary `json:"logins"`
}

// ProviderInfo is a provider the dashboard offers; SignIn says CLIProxyAPI can sign in to it by itself.
type ProviderInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	SignIn bool   `json:"sign_in"`
}

type ownRequest struct {
	Providers []string `json:"providers"`
}

type signInRequest struct {
	Provider string `json:"provider"`
}
