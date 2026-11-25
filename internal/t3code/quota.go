package t3code

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// QuotaEndpoint names the exact HTTP origins a verified Spillup service manages.
type QuotaEndpoint struct {
	Port  int
	Hosts []string
}

func (q QuotaEndpoint) matches(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || q.Port <= 0 || u.Scheme != "http" || u.User != nil ||
		u.Port() != strconv.Itoa(q.Port) || (u.EscapedPath() != "" && u.EscapedPath() != "/") ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") {
		return false
	}
	host := strings.TrimSuffix(u.Hostname(), ".")
	for _, h := range q.Hosts {
		if h != "" && strings.EqualFold(host, strings.TrimSuffix(h, ".")) {
			return true
		}
	}
	return false
}

func (t T3) StaleQuotaSources(endpoints []QuotaEndpoint, key string) (int, error) {
	return t.quotaKeys(endpoints, key, false)
}

func (t T3) FixQuotaKeys(endpoints []QuotaEndpoint, key string) (int, error) {
	return t.quotaKeys(endpoints, key, true)
}

func (t T3) settingsPath() string { return filepath.Join(t.Home, "userdata", "settings.json") }

func (t T3) quotaKeys(endpoints []QuotaEndpoint, key string, repair bool) (int, error) {
	if key == "" || len(endpoints) == 0 {
		return 0, nil
	}
	root, err := os.OpenRoot(t.Home)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer root.Close()
	dir, err := safeQuotaDir(root, "userdata")
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer dir.Close()
	raw, snapshot, err := quotaFile(dir, "settings.json", 8<<20)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var settings struct {
		Sources map[string]struct{ Kind, URL string } `json:"usageLimitSources"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return 0, fmt.Errorf("read T3 quota settings: %w", err)
	}
	var files []string
	for id, src := range settings.Sources {
		if src.Kind != "cliproxy" || id == "" || len(id) > 170 {
			continue
		}
		if slices.ContainsFunc(endpoints, func(q QuotaEndpoint) bool { return q.matches(src.URL) }) {
			files = append(files, quotaSecretName(id))
		}
	}
	if len(files) == 0 {
		return 0, nil
	}
	secrets, err := safeQuotaDir(dir, "secrets")
	if errors.Is(err, fs.ErrNotExist) {
		if !repair {
			return len(files), nil
		}
		if err := dir.Mkdir("secrets", 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return 0, err
		}
		secrets, err = safeQuotaDir(dir, "secrets")
	}
	if err != nil {
		return 0, err
	}
	defer secrets.Close()
	n := 0
	var issue error
	refresh := false
	for _, name := range files {
		got, modified, err := quotaFile(secrets, name, 64<<10)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			issue = err
			continue
		}
		if string(bytes.TrimSpace(got)) == key {
			refresh = refresh || modified.ModTime().After(snapshot.ModTime())
			continue
		}
		if repair {
			// T3 can edit a source while this pass is running. Verify its settings before replacing its secret.
			current, err := dir.Lstat("settings.json")
			if err != nil {
				return n, err
			}
			if !os.SameFile(snapshot, current) || snapshot.Size() != current.Size() || !snapshot.ModTime().Equal(current.ModTime()) {
				return n, errors.New("T3 quota settings changed during repair; retrying on the next pass")
			}
			if err := replaceQuotaKey(secrets, name, key); err != nil {
				issue = err
				continue
			}
		}
		n++
	}
	if repair && (n > 0 || refresh) {
		now := time.Now()
		if err := dir.Chtimes("settings.json", now, now); err != nil {
			return n, fmt.Errorf("reload T3 quota keys: %w", err)
		}
	}
	return n, issue
}

func quotaSecretName(id string) string {
	return "usage-limit-source-" + base64.RawURLEncoding.EncodeToString([]byte(id)) + ".bin"
}

func safeQuotaDir(root *os.Root, name string) (*os.Root, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("unexpected T3 quota directory")
	}
	return root.OpenRoot(name)
}

func quotaFile(dir *os.Root, name string, limit int64) ([]byte, os.FileInfo, error) {
	info, err := dir.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, nil, errors.New("unexpected T3 quota file")
	}
	f, err := dir.Open(name)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(raw)) > limit {
		return nil, nil, errors.New("T3 quota file too large")
	}
	return raw, info, err
}

func replaceQuotaKey(dir *os.Root, name, key string) error {
	tmp := ".quota-" + rand.Text() + ".tmp"
	f, err := dir.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer dir.Remove(tmp)
	if _, err = f.WriteString(key); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err = errors.Join(err, closeErr); err != nil {
		return err
	}
	return dir.Rename(tmp, name)
}
