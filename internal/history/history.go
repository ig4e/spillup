// Package history keeps bounded operation and health records in the machine's private state directory.
package history

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/darkyeg/spinup/internal/atomicfile"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/filelock"
)

type Store struct{ Dir string }

func Local() Store { return Store{Dir: config.StateDir()} }

func (s Store) path(name string) string { return filepath.Join(s.Dir, "history", name+".json") }

func read[T any](path string) ([]T, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return []T{}, nil
	}
	if err != nil {
		return nil, err
	}
	const limit = 512 << 10
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("invalid history file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if len(buf) > limit {
		return nil, fmt.Errorf("history file too large")
	}
	var list []T
	if err := json.Unmarshal(buf, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func recent[T any](path string) ([]T, error) {
	list, err := read[T](path)
	slices.Reverse(list)
	return list, err
}

func edit[T any](path string, limit int, change func([]T) ([]T, bool)) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	lock, err := filelock.Acquire(ctx, path+".lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	list, err := read[T](path)
	if err != nil {
		return err
	}
	list, changed := change(list)
	if !changed {
		return nil
	}
	if len(list) > limit {
		list = list[len(list)-limit:]
	}
	data, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return atomicfile.Write(path, data, 0o600)
}

type Action struct {
	At        time.Time `json:"at"`
	Action    string    `json:"action"`
	Origin    string    `json:"origin"`
	Status    int       `json:"status"`
	ElapsedMS int64     `json:"elapsed_ms"`
}

func (s Store) RecordAction(a Action) error {
	return edit(s.path("actions"), 100, func(list []Action) ([]Action, bool) { return append(list, a), true })
}

func (s Store) Actions() ([]Action, error) { return recent[Action](s.path("actions")) }
