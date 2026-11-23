package route

import (
	"reflect"
	"testing"
)

func TestProvidersOf(t *testing.T) {
	cases := []struct {
		model string
		want  []Provider
	}{
		{"claude-sonnet-4-5-20250929", []Provider{"claude"}},
		{"anthropic/claude-3-haiku", []Provider{"claude"}},
		{"gpt-5-codex", []Provider{"codex"}},
		{"o4-mini", []Provider{"codex"}},
		{"gemini-2.5-pro", []Provider{"antigravity", "gemini"}},
		{"grok-4", []Provider{"xai"}},
		{"kimi-k2", []Provider{"kimi", "kimi-ai"}},
		{"mystery-model", nil},
		{"", nil},
	}
	for _, c := range cases {
		if got := ProvidersOf(c.model); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ProvidersOf(%q) = %v, want %v", c.model, got, c.want)
		}
	}
}

func TestCanonicalReadsEarlierFamilyNames(t *testing.T) {
	for in, want := range map[string]Provider{"claude": "claude", "gpt": "codex", "gemini": "antigravity", "kimi": "kimi"} {
		if got := Canonical(in); got != want {
			t.Errorf("Canonical(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidAcceptsAnyLoginType(t *testing.T) {
	for _, ok := range []string{"claude", "kimi-ai", "qwen", "iflow2"} {
		if !Valid(ok) {
			t.Errorf("%q should be a valid provider", ok)
		}
	}
	for _, bad := range []string{"", "Claude", "../x", "a b", "-x"} {
		if Valid(bad) {
			t.Errorf("%q should not be a valid provider", bad)
		}
	}
}

func TestModelOf(t *testing.T) {
	cases := []struct {
		name, path, body, want string
	}{
		{"anthropic messages", "/v1/messages", `{"model":"claude-sonnet-4-5","stream":true}`, "claude-sonnet-4-5"},
		{"openai responses", "/v1/responses", `{"input":"hi","model":"gpt-5"}`, "gpt-5"},
		{"gemini generate", "/v1beta/models/gemini-2.5-pro:streamGenerateContent", ``, "gemini-2.5-pro"},
		{"model list", "/v1/models", ``, ""},
		{"not json", "/v1/chat/completions", `model=x`, ""},
	}
	for _, c := range cases {
		if got := ModelOf(c.path, []byte(c.body)); got != c.want {
			t.Errorf("%s: ModelOf = %q, want %q", c.name, got, c.want)
		}
	}
}
