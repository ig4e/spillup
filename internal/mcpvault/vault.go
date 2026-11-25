// Package mcpvault keeps launcher secrets and single-use transfer approvals in the local user state directory.
package mcpvault

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/darkyeg/spinup/internal/atomicfile"
	"github.com/darkyeg/spinup/internal/filelock"
)

type Store struct{ Dir string }

type entry struct {
	Value string `json:"value,omitempty"`
	Grant *grant `json:"grant,omitempty"`
}

type grant struct {
	SourceID string    `json:"source_id"`
	Token    string    `json:"token"`
	Until    time.Time `json:"until"`
}

type entries map[string]map[string]entry

var serverName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
var variableName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,63}$`)
var ErrConsent = errors.New("this machine has no current approval for that sender, server and key")

const maxStore = 2 << 20

func Validate(server, key string) error {
	if !serverName.MatchString(server) || !variableName.MatchString(key) {
		return errors.New("invalid MCP server or environment variable name")
	}
	return nil
}

func validateValue(value string) error {
	if !utf8.ValidString(value) || value == "" || len(value) > 16<<10 || strings.IndexByte(value, 0) >= 0 {
		return errors.New("the secret must be nonempty text of at most 16 KiB without a NUL character")
	}
	return nil
}

func privatePath(path string, dir bool) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 || (dir && !st.IsDir()) || (!dir && !st.Mode().IsRegular()) || (runtime.GOOS != "windows" && st.Mode().Perm()&0o077 != 0) {
		return errors.New("MCP vault paths must be private regular files in a private directory")
	}
	return nil
}

func (s Store) dir(create bool) (string, error) {
	if !filepath.IsAbs(s.Dir) {
		return "", errors.New("MCP vault needs an absolute state directory")
	}
	dir := filepath.Join(s.Dir, ".mcp-vault")
	if create {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", err
		}
	}
	if err := privatePath(dir, true); err != nil {
		return "", err
	}
	if create {
		if err := protectDirectory(dir); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func load(dir string) (entries, error) {
	p := filepath.Join(dir, "secrets.json")
	if err := privatePath(p, false); err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxStore+1))
	var data entries
	if err != nil || len(raw) > maxStore || json.Unmarshal(raw, &data) != nil || data == nil {
		return nil, errors.New("couldn't read the MCP vault")
	}
	return data, nil
}

func (s Store) lookup(server, key string) (entry, error) {
	if err := Validate(server, key); err != nil {
		return entry{}, err
	}
	dir, err := s.dir(false)
	if err != nil {
		return entry{}, err
	}
	all, err := load(dir)
	if err != nil {
		return entry{}, err
	}
	item, ok := all[server][key]
	if !ok {
		return entry{}, os.ErrNotExist
	}
	return item, nil
}

// Get returns the local value for a launcher. Get and Has never create a vault.
func (s Store) Get(server, key string) (string, error) {
	item, err := s.lookup(server, key)
	if err != nil {
		return "", err
	}
	if item.Value == "" {
		return "", os.ErrNotExist
	}
	return item.Value, nil
}

func (s Store) Has(server, key string) (bool, error) {
	_, err := s.Get(server, key)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// Keys returns the variable names that currently have values, grouped by server.
func (s Store) Keys() (map[string][]string, error) {
	keys := make(map[string][]string)
	dir, err := s.dir(false)
	if errors.Is(err, os.ErrNotExist) {
		return keys, nil
	}
	if err != nil {
		return nil, err
	}
	all, err := load(dir)
	if errors.Is(err, os.ErrNotExist) {
		return keys, nil
	}
	if err != nil {
		return nil, err
	}
	for server, items := range all {
		for key, e := range items {
			if e.Value != "" && Validate(server, key) == nil {
				keys[server] = append(keys[server], key)
			}
		}
		slices.Sort(keys[server])
	}
	return keys, nil
}

func (s Store) edit(ctx context.Context, server, key string, change func(*entry) error) error {
	if err := Validate(server, key); err != nil {
		return err
	}
	dir, err := s.dir(true)
	if err != nil {
		return err
	}
	lp := filepath.Join(dir, "write.lock")
	if err := privatePath(lp, false); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	lock, err := filelock.Acquire(ctx, lp)
	if err != nil {
		return err
	}
	defer lock.Close()
	all, err := load(dir)
	if errors.Is(err, os.ErrNotExist) {
		all = make(entries)
	} else if err != nil {
		return err
	}
	item := all[server][key]
	if err := change(&item); err != nil {
		return err
	}
	if item.Value == "" && item.Grant == nil {
		delete(all[server], key)
		if len(all[server]) == 0 {
			delete(all, server)
		}
	} else {
		if all[server] == nil {
			all[server] = make(map[string]entry)
		}
		all[server][key] = item
	}
	raw, err := json.Marshal(all)
	if err != nil || len(raw) > maxStore {
		return errors.New("MCP vault size limit exceeded")
	}
	return atomicfile.Write(filepath.Join(dir, "secrets.json"), raw, 0o600)
}

func (s Store) Set(ctx context.Context, server, key, value string) error {
	if err := validateValue(value); err != nil {
		return err
	}
	return s.edit(ctx, server, key, func(item *entry) error { *item = entry{Value: value}; return nil })
}

func (s Store) Forget(ctx context.Context, server, key string) error {
	return s.edit(ctx, server, key, func(item *entry) error { *item = entry{}; return nil })
}

func (s Store) Allow(ctx context.Context, server, key, sourceID string) error {
	if sourceID == "" {
		return ErrConsent
	}
	return s.edit(ctx, server, key, func(item *entry) error {
		item.Grant = &grant{SourceID: sourceID, Token: rand.Text(), Until: time.Now().Add(10 * time.Minute)}
		return nil
	})
}

func (item entry) approved(sourceID string) bool {
	return sourceID != "" && item.Grant != nil && item.Grant.SourceID == sourceID && time.Now().Before(item.Grant.Until)
}

// Permit returns only the one-time challenge to the approved Tailnet sender.
func (s Store) Permit(server, key, sourceID string) (string, error) {
	item, err := s.lookup(server, key)
	if err != nil || !item.approved(sourceID) {
		return "", ErrConsent
	}
	return item.Grant.Token, nil
}

func (s Store) Receive(ctx context.Context, server, key, sourceID, permit, value string) error {
	if err := validateValue(value); err != nil {
		return err
	}
	return s.edit(ctx, server, key, func(item *entry) error {
		if !item.approved(sourceID) || subtle.ConstantTimeCompare([]byte(permit), []byte(item.Grant.Token)) != 1 {
			return ErrConsent
		}
		*item = entry{Value: value}
		return nil
	})
}
