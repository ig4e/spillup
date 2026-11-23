// Package route says which provider a request is for, so a machine can serve some providers with its own
// sign-ins instead of the shared accounts.
package route

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Provider is a kind of login CLIProxyAPI holds, named as in its login files: claude, codex, antigravity…
type Provider string

// SignIn is a provider CLIProxyAPI can sign in to by itself, and the management endpoint that starts it.
type SignIn struct {
	Provider Provider
	Name     string
	Path     string
}

// SignIns are the OAuth sign-ins CLIProxyAPI's management API offers. Providers found in your logins
// join them on the dashboard, so a new kind of login shows up without a change here.
var SignIns = []SignIn{
	{"claude", "Claude", "anthropic-auth-url"},
	{"codex", "ChatGPT & Codex", "codex-auth-url"},
	{"antigravity", "Google Antigravity", "antigravity-auth-url"},
	{"kimi", "Kimi", "kimi-auth-url"},
	{"kimi-ai", "Kimi (Moonshot AI)", "kimi-ai-auth-url"},
	{"xai", "Grok (xAI)", "xai-auth-url"},
	{"devin", "Devin", "devin-auth-url"},
	{"meta", "Meta AI", "meta-auth-url"},
}

// SignInFor is the sign-in for a provider, if CLIProxyAPI has one.
func SignInFor(p Provider) (SignIn, bool) {
	for _, s := range SignIns {
		if s.Provider == p {
			return s, true
		}
	}
	return SignIn{}, false
}

var providerName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// Valid reports whether name can be a provider: any login type, not only the ones CLIProxyAPI signs in to.
func Valid(name string) bool { return providerName.MatchString(name) }

// legacyFamilies are the model families earlier versions stored, and the provider each means now.
var legacyFamilies = map[string]Provider{"gpt": "codex", "gemini": "antigravity"}

// Canonical turns a stored name into a provider, reading an earlier version's family names.
func Canonical(name string) Provider {
	if p, ok := legacyFamilies[name]; ok {
		return p
	}
	return Provider(name)
}

// ProvidersOf names the providers that may serve a model, from its name alone: the fallback while a
// machine's own CLIProxyAPI hasn't said which models its sign-ins serve.
func ProvidersOf(model string) []Provider {
	m := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndex(m, "/"); i >= 0 {
		m = m[i+1:]
	}
	switch {
	case strings.HasPrefix(m, "claude"):
		return []Provider{"claude"}
	case strings.HasPrefix(m, "gpt-"), strings.HasPrefix(m, "codex"), strings.HasPrefix(m, "o1"),
		strings.HasPrefix(m, "o3"), strings.HasPrefix(m, "o4"):
		return []Provider{"codex"}
	case strings.HasPrefix(m, "gemini"):
		return []Provider{"antigravity", "gemini"}
	case strings.HasPrefix(m, "grok"):
		return []Provider{"xai"}
	case strings.HasPrefix(m, "kimi"):
		return []Provider{"kimi", "kimi-ai"}
	}
	return nil
}

// ModelOf finds the model a request asks for: Gemini names it in the path, the others in the JSON body.
func ModelOf(path string, body []byte) string {
	if i := strings.Index(path, "/models/"); i >= 0 {
		rest := path[i+len("/models/"):]
		if j := strings.IndexAny(rest, ":/"); j >= 0 {
			rest = rest[:j]
		}
		if rest != "" {
			return rest
		}
	}
	var req struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &req) != nil {
		return ""
	}
	return req.Model
}
