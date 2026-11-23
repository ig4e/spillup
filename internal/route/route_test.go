package route

import "testing"

func TestFamilyOf(t *testing.T) {
	cases := []struct {
		model string
		want  Family
	}{
		{"claude-sonnet-4-5-20250929", Claude},
		{"Claude-Opus-4", Claude},
		{"anthropic/claude-3-haiku", Claude},
		{"gpt-5-codex", GPT},
		{"codex-mini-latest", GPT},
		{"o3", GPT},
		{"o4-mini", GPT},
		{"gemini-2.5-pro", Gemini},
		{"models/gemini-3-pro-preview", Gemini},
		{"kimi-k2", ""},
		{"", ""},
		{"opus", ""},
	}
	for _, c := range cases {
		if got := FamilyOf(c.model); got != c.want {
			t.Errorf("FamilyOf(%q) = %q, want %q", c.model, got, c.want)
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
