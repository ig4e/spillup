//go:build !windows

package mcpvault

func protectDirectory(string) error { return nil }
