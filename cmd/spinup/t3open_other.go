//go:build !windows

package main

func openT3Desktop(link string) bool { return openBrowser(link) }
