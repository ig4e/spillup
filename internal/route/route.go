// Package route says which model family a request is for, so a machine can serve some families with
// its own logins instead of the shared accounts.
package route

import (
	"encoding/json"
	"strings"
)

// Family is a group of models that one kind of login serves.
type Family string

const (
	Claude Family = "claude"
	GPT    Family = "gpt"
	Gemini Family = "gemini"
)

// Families are every family a machine may serve with its own logins, in the order the dashboard lists them.
var Families = []Family{Claude, GPT, Gemini}

// Known reports whether f is one of Families.
func Known(f Family) bool {
	for _, k := range Families {
		if k == f {
			return true
		}
	}
	return false
}

// FamilyOf names the family of a model, or "" for a model no family claims.
func FamilyOf(model string) Family {
	m := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndex(m, "/"); i >= 0 {
		m = m[i+1:]
	}
	switch {
	case strings.HasPrefix(m, "claude"):
		return Claude
	case strings.HasPrefix(m, "gpt-"), strings.HasPrefix(m, "codex"), strings.HasPrefix(m, "o1"),
		strings.HasPrefix(m, "o3"), strings.HasPrefix(m, "o4"):
		return GPT
	case strings.HasPrefix(m, "gemini"):
		return Gemini
	}
	return ""
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
