//go:build !windows

package main

import (
	"context"
	"errors"
)

func desktopClipboard(ctx context.Context) (string, error) { return readClipboard(ctx) }

type clipboardSessionCmd struct{}

func (clipboardSessionCmd) Run() error {
	return errors.New("this helper is used by the Windows desktop")
}
