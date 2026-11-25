package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"

	"github.com/darkyeg/spinup/internal/atomicfile"
	"github.com/darkyeg/spinup/internal/filelock"
	"github.com/darkyeg/spinup/internal/library"
	"github.com/darkyeg/spinup/internal/mcpvault"
)

type Manager struct {
	Library library.Library
	State   string
	// Launcher selects the stable installed copy of this executable, when available.
	Launcher func(self string) string
}

type Server struct {
	Definition
	Sharing        string   `json:"sharing"`
	HasShared      bool     `json:"hasShared"`
	Enabled        []string `json:"enabled"`
	AvailableKeys  []string `json:"availableKeys"`
	Revision       string   `json:"revision"`
	ChangedTargets []string `json:"changedTargets,omitempty"`
	Changed        bool     `json:"changed,omitempty"`
	Removed        bool     `json:"removed,omitempty"`
}

type View struct {
	Servers          []Server `json:"servers"`
	AvailableTargets []string `json:"availableTargets"`
}

type catalogs struct{ shared, local map[string]Definition }

func readCatalog(path string) (map[string]Definition, error) {
	info, err := os.Lstat(path)
	defs := map[string]Definition{}
	if errors.Is(err, os.ErrNotExist) {
		return defs, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, errors.New("MCP catalog must be a regular file up to 1 MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := parse(data)
	if err != nil {
		return nil, err
	}
	for _, d := range c.Servers {
		defs[d.Name] = d
	}
	return defs, nil
}
func saveCatalog(path string, defs map[string]Definition) error {
	c := catalog{Servers: []Definition{}}
	for _, d := range defs {
		c.Servers = append(c.Servers, d)
	}
	sort.Slice(c.Servers, func(i, j int) bool { return c.Servers[i].Name < c.Servers[j].Name })
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := ValidateManifest(data); err != nil {
		return err
	}
	return atomicfile.Write(path, append(data, '\n'), 0600)
}
func (m Manager) localPath() string { return m.Library.Path("local/mcp-private.json") }
func (m Manager) catalogs() (catalogs, error) {
	shared, err := readCatalog(m.Library.Path(Manifest))
	if err != nil {
		return catalogs{}, err
	}
	local, err := readCatalog(m.localPath())
	return catalogs{shared, local}, err
}
func (c catalogs) resolve(name string) (Definition, string, bool) {
	if d, ok := c.local[name]; ok {
		return d, "local", true
	}
	d, ok := c.shared[name]
	return d, "shared", ok
}
func (m Manager) withCatalogs(ctx context.Context, edit func(catalogs) error) error {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	lock, err := filelock.Acquire(ctx, m.Library.Path(".packages.lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	c, err := m.catalogs()
	if err != nil {
		return err
	}
	return edit(c)
}
func (m Manager) Save(ctx context.Context, d Definition, sharing string) error {
	if err := d.Validate(); err != nil {
		return err
	}
	return m.withCatalogs(ctx, func(c catalogs) error {
		_, scope, exists := c.resolve(d.Name)
		if exists && sharing != scope {
			return errors.New("change sharing on the existing server first")
		}
		switch sharing {
		case "local":
			c.local[d.Name] = d
			return saveCatalog(m.localPath(), c.local)
		case "shared":
			c.shared[d.Name] = d
			return saveCatalog(m.Library.Path(Manifest), c.shared)
		default:
			return errors.New("choose local or shared")
		}
	})
}
func (m Manager) Sharing(ctx context.Context, name, action, revision string) error {
	return m.withCatalogs(ctx, func(c catalogs) error {
		d, scope, ok := c.resolve(name)
		if !ok || d.Revision() != revision {
			return errors.New("the server changed; review it again")
		}
		switch action {
		case "detach":
			if scope == "local" {
				return nil
			}
			c.local[name] = d
		case "unpublish":
			if _, ok := c.shared[name]; !ok {
				return nil
			}
			c.local[name] = d
			if err := saveCatalog(m.localPath(), c.local); err != nil {
				return err
			}
			delete(c.shared, name)
			return saveCatalog(m.Library.Path(Manifest), c.shared)
		case "follow":
			if _, ok := c.shared[name]; !ok {
				return errors.New("no shared definition exists")
			}
			delete(c.local, name)
		case "publish":
			if scope == "shared" {
				return nil
			}
			c.shared[name] = d
			if err := saveCatalog(m.Library.Path(Manifest), c.shared); err != nil {
				return err
			}
			delete(c.local, name)
		default:
			return errors.New("choose detach, publish, follow or unpublish")
		}
		return saveCatalog(m.localPath(), c.local)
	})
}

var targets = []string{"claude", "codex"}

func (m Manager) approvalPath(target string) string {
	id := sha256.Sum256([]byte(filepath.Clean(nativeConfigPath(target)) + "\x00" + filepath.Clean(m.Library.Dir())))
	return filepath.Join(m.State, ".mcp-approvals", fmt.Sprintf("%s-%x.json", target, id[:16]))
}

func (m Manager) List() (View, error) {
	c, err := m.catalogs()
	if err != nil {
		return View{}, err
	}
	approvals := map[string]map[string]Definition{}
	names := map[string]bool{}
	for k := range c.shared {
		names[k] = true
	}
	for k := range c.local {
		names[k] = true
	}
	for _, t := range targets {
		defs, err := readCatalog(m.approvalPath(t))
		if err != nil {
			return View{}, err
		}
		approvals[t] = defs
		for k := range defs {
			names[k] = true
		}
	}
	v := View{Servers: []Server{}, AvailableTargets: AvailableTargets()}
	keys, err := (mcpvault.Store{Dir: m.State}).Keys()
	if err != nil {
		return View{}, err
	}
	for name := range names {
		d, scope, exists := c.resolve(name)
		_, hasShared := c.shared[name]
		row := Server{Definition: d, Sharing: scope, HasShared: hasShared, Removed: !exists, Enabled: []string{}, AvailableKeys: []string{}}
		for _, t := range targets {
			pinned, ok := approvals[t][name]
			if !ok {
				continue
			}
			row.Enabled = append(row.Enabled, t)
			if !exists {
				row.Definition = pinned
			}
			if !exists || d.Revision() != pinned.Revision() {
				row.Changed = true
				row.ChangedTargets = append(row.ChangedTargets, t)
			}
		}
		row.Revision = row.Definition.Revision()
		for _, key := range row.SecretKeys {
			if slices.Contains(keys[name], key) {
				row.AvailableKeys = append(row.AvailableKeys, key)
			}
		}
		v.Servers = append(v.Servers, row)
	}
	sort.Slice(v.Servers, func(i, j int) bool { return v.Servers[i].Name < v.Servers[j].Name })
	return v, nil
}

func (m Manager) Remove(ctx context.Context, name string) error {
	return m.withCatalogs(ctx, func(c catalogs) error {
		if !namePattern.MatchString(name) {
			return errors.New("invalid MCP server name")
		}
		for _, t := range targets {
			path := m.approvalPath(t)
			if _, err := os.Stat(path + "." + name + ".pending"); err == nil {
				return errors.New("finish or disable this server's pending agent registration before removing it")
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			defs, err := readCatalog(path)
			if err != nil {
				return err
			}
			if _, ok := defs[name]; ok {
				return errors.New("disable the server in each agent on this machine before removing it")
			}
		}
		_, scope, ok := c.resolve(name)
		if !ok {
			return nil
		}
		if scope == "local" {
			delete(c.local, name)
			return saveCatalog(m.localPath(), c.local)
		}
		delete(c.shared, name)
		return saveCatalog(m.Library.Path(Manifest), c.shared)
	})
}

func (m Manager) Target(ctx context.Context, name, target, revision string, enable bool) error {
	if !slices.Contains(targets, target) || !namePattern.MatchString(name) {
		return errors.New("invalid server or agent")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return m.withCatalogs(ctx, func(c catalogs) error {
		path := m.approvalPath(target)
		defs, err := readCatalog(path)
		if err != nil {
			return err
		}
		var previous *Definition
		if old, ok := defs[name]; ok {
			previous = &old
		}
		d, _, exists := c.resolve(name)
		if enable && (!exists || d.Revision() != revision) {
			return errors.New("the server changed; review it again before enabling")
		}
		pending := path + "." + name + ".pending"
		if !enable {
			if previous != nil {
				d = *previous
			} else {
				// A first native registration may have committed before its local
				// approval. Only its exact pending receipt can authorize removal.
				if _, err := os.Stat(pending); errors.Is(err, os.ErrNotExist) {
					return nil
				} else if err != nil {
					return err
				}
				d = Definition{Name: name}
			}
		}
		if err := nativeTarget(ctx, target, d, enable, previous, pending, m.Launcher); err != nil {
			return err
		}
		if enable {
			defs[name] = d
		} else {
			delete(defs, name)
		}
		if err := saveCatalog(path, defs); err != nil {
			return err
		}
		registered := registeredPath(pending)
		if enable {
			return atomicfile.Rename(pending, registered)
		}
		for _, receipt := range []string{pending, registered} {
			if err := os.Remove(receipt); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		return nil
	})
}

func (m Manager) DeclaresSecret(name, key string) error {
	c, err := m.catalogs()
	if err != nil {
		return err
	}
	d, _, ok := c.resolve(name)
	if !ok || !slices.Contains(d.SecretKeys, key) {
		return errors.New("declare this secret key on the local MCP definition first")
	}
	return nil
}
