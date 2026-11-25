package library

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"
)

// A single unified hunk keeps memory linear even for entirely rewritten files.
func documentDiff(current, prior []byte) string {
	if bytes.Equal(current, prior) {
		return ""
	}
	old, newer := strings.SplitAfter(string(current), "\n"), strings.SplitAfter(string(prior), "\n")
	if old[len(old)-1] == "" {
		old = old[:len(old)-1]
	}
	if newer[len(newer)-1] == "" {
		newer = newer[:len(newer)-1]
	}
	start := 0
	for start < len(old) && start < len(newer) && old[start] == newer[start] {
		start++
	}
	end := 0
	for end < len(old)-start && end < len(newer)-start && old[len(old)-end-1] == newer[len(newer)-end-1] {
		end++
	}
	var out strings.Builder
	from, to := max(0, start-3), max(0, end-3)
	line := func(prefix, content string) {
		out.WriteString(prefix)
		for i, r := range content {
			switch {
			case r == '\r' && (i+1 >= len(content) || content[i+1] != '\n'):
				out.WriteString("\\r")
			case unicode.In(r, unicode.Cf):
				fmt.Fprintf(&out, "\\u{%04X}", r)
			default:
				out.WriteRune(r)
			}
		}
		if !strings.HasSuffix(content, "\n") {
			out.WriteString("\n\\ No newline at end of file\n")
		}
	}
	fmt.Fprintf(&out, "--- current\n+++ selected revision\n@@ -%d,%d +%d,%d @@\n", min(from+1, len(old)), len(old)-from-to, min(from+1, len(newer)), len(newer)-from-to)
	for _, s := range old[from:start] {
		line(" ", s)
	}
	for _, s := range old[start : len(old)-end] {
		line("-", s)
	}
	for _, s := range newer[start : len(newer)-end] {
		line("+", s)
	}
	for _, s := range old[len(old)-end : len(old)-to] {
		line(" ", s)
	}
	return out.String()
}
