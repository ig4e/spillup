package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/atomicfile"
	"github.com/darkyeg/spinup/internal/filelock"
)

// The active copy of a private skill lives in the agent store. Only authored and
// fetched library trees travel between machines. The hidden marker is read even
// by offline sync, before installing or parking anything.
type privateSkill struct {
	Source string `json:"source"`
	Manual bool   `json:"manual,omitempty"`
}
type privateSkills map[string]privateSkill

func (m Manager) privateFile() string { return filepath.Join(m.paths.store, ".spinup-private.json") }

func (m Manager) privateSkills() (privateSkills, error) {
	raw, err := os.ReadFile(m.privateFile())
	if errors.Is(err, os.ErrNotExist) {
		return privateSkills{}, nil
	}
	if err != nil {
		return nil, err
	}
	var v privateSkills
	if json.Unmarshal(raw, &v) != nil || v == nil {
		return nil, errors.New("invalid machine-private skill settings")
	}
	for k := range v {
		if err := checkNames([]string{k}); err != nil {
			return nil, err
		}
	}
	return v, nil
}
func (m Manager) savePrivate(v privateSkills) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(m.privateFile(), append(data, '\n'), 0600)
}
func (p privateSkills) name(name string) string {
	if _, ok := p[name]; ok {
		return name
	}
	for k := range p {
		if strings.EqualFold(k, name) {
			return k
		}
	}
	return ""
}
func (p privateSkills) has(name string) bool { return p.name(name) != "" }
func (p privateSkills) filtered(m manifest) manifest {
	if len(p) == 0 {
		return m
	}
	out := manifest{}
	for _, v := range m.sources {
		names := slices.DeleteFunc(slices.Clone(v.Names), p.has)
		if len(names) > 0 {
			out.sources = append(out.sources, sourceEntry{v.Source, names})
		}
	}
	out.manual = slices.DeleteFunc(slices.Clone(m.manual), p.has)
	for name, v := range p {
		if v.Manual {
			out.manual = append(out.manual, name)
		}
	}
	return out
}

func (m Manager) requireShared(names []string) error {
	if err := checkNames(names); err != nil {
		return err
	}
	p, err := m.privateSkills()
	if err != nil {
		return err
	}
	for _, n := range names {
		if p.has(n) {
			return fmt.Errorf("%s is private on this machine; use skills sharing to publish it or resume the shared copy", n)
		}
	}
	return nil
}

func (m Manager) sharedSkill(name string, list manifest) (source, path string) {
	if p := m.ownPath(name); fileExists(filepath.Join(p, skillFile)) {
		return OwnSource, p
	}
	for _, entry := range list.sources {
		if slices.Contains(entry.Names, name) {
			return entry.Source, m.fetchedCopy(name)
		}
	}
	return "", ""
}

// A private copy must be self-contained. A preserved link could point
// straight back at the shared authored tree or another machine's mount.
func selfContained(files fs.FS) error {
	return fs.WalkDir(files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && !d.Type().IsRegular() {
			return fmt.Errorf("skill file %q must be a regular file to make a private copy", path)
		}
		return nil
	})
}

// Keep real agent folders displaced by a private link in local-only history.
// Shared trash can be restored into the fleet and is the wrong place for them.
func (p paths) savePrivateAgentFolders(name string) error {
	for _, dir := range p.agentSkills() {
		current := filepath.Join(dir, name)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if isLink(current) || !info.IsDir() {
			continue
		}
		history := p
		history.parked = filepath.Join(p.store, ".private-history")
		if err := os.MkdirAll(history.parked, 0700); err != nil {
			return err
		}
		if err := history.parkAside(current, name); err != nil {
			return err
		}
	}
	return nil
}

// Recheck the paths Spillup writes after detachment. A subsequent checkout or
// edit can introduce a link to a shared library path into the working copy.
func (m Manager) checkPrivateWrites(p privateSkills, names []string) error {
	for _, name := range names {
		n := p.name(name)
		if n == "" {
			continue
		}
		dir := filepath.Join(m.paths.store, n)
		for _, path := range []string{dir, filepath.Join(dir, skillFile), filepath.Join(dir, codexPolicyDirName), filepath.Join(dir, codexPolicyDirName, codexPolicyFile)} {
			st, err := os.Lstat(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return err
			}
			if st.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
				return fmt.Errorf("private skill %s has a linked working file; restore a regular file before changing it", n)
			}
		}
	}
	return nil
}

// Resolve a configured directory even if its final components have not been
// created yet. A custom library can point into the agent store via a symlink.
func realDirectory(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for here := abs; ; here = filepath.Dir(here) {
		resolved, err := filepath.EvalSymlinks(here)
		if err == nil {
			relative, err := filepath.Rel(here, abs)
			if err != nil {
				return "", err
			}
			return filepath.Join(resolved, relative), nil
		}
		if !errors.Is(err, os.ErrNotExist) || here == filepath.Dir(here) {
			return "", err
		}
	}
}
func (m Manager) separateLibrary() error {
	store, err := realDirectory(m.paths.store)
	if err != nil {
		return err
	}
	for _, tree := range []string{m.ownPath(""), m.fetchedCopy("")} {
		shared, err := realDirectory(tree)
		if err != nil {
			return err
		}
		nested, err := filepath.Rel(shared, store)
		if err == nil && filepath.IsLocal(nested) {
			return errors.New("private skills need an agent store outside the shared library skill directories")
		}
		nested, err = filepath.Rel(store, shared)
		if err == nil && filepath.IsLocal(nested) {
			return errors.New("private skills need a shared library outside the agent store")
		}
	}
	return nil
}

// A scope change should only relink its own skills. In particular, the first
// click on "private" must not fetch unrelated repositories or park the user's
// other unmanaged skills before they can choose a scope for them.
func (m Manager) activate(ctx context.Context, names []string) (Synced, error) {
	p, err := m.privateSkills()
	if err != nil {
		return Synced{}, err
	}
	list, err := m.manifest()
	if err != nil {
		return Synced{}, err
	}
	if err := m.checkPrivateWrites(p, names); err != nil {
		return Synced{}, err
	}
	for _, n := range names {
		if p.has(n) {
			if err := m.paths.savePrivateAgentFolders(n); err != nil {
				return Synced{}, err
			}
		}
	}
	if err := m.paths.linkAll(ctx, names); err != nil {
		return Synced{}, err
	}
	if err := m.paths.applyModes(names, p.filtered(list).manual); err != nil {
		return Synced{}, err
	}
	return Synced{Count: len(names), Agents: agentNames(m.paths.agentsHere())}, nil
}

// Sharing switches one named skill on this machine. A resume saves the working
// copy in hidden local history before installing the latest shared version.
func (m Manager) Sharing(ctx context.Context, name, action string) (Synced, error) {
	if err := checkNames([]string{name}); err != nil {
		return Synced{}, err
	}
	lock, err := m.paths.lockStore(ctx)
	if err != nil {
		return Synced{}, err
	}
	defer lock.Close()
	p, err := m.privateSkills()
	if err != nil {
		return Synced{}, err
	}
	if exact := p.name(name); exact != "" {
		name = exact
	}
	list, err := m.manifest()
	if err != nil {
		return Synced{}, err
	}
	shared, baseline := m.sharedSkill(name, list)
	working := filepath.Join(m.paths.store, name)
	switch action {
	case "private":
		if err := m.separateLibrary(); err != nil {
			return Synced{}, err
		}
		if p.has(name) {
			break
		}
		src, _ := m.localCopy(name)
		if src == "" {
			src = baseline
		}
		if src == "" || !fileExists(filepath.Join(src, skillFile)) {
			return Synced{}, fmt.Errorf("install %s before making it private", name)
		}
		if shared == "" {
			shared = "this machine"
		}
		// Even a previously symlinked install must become an independent writable directory.
		if err := selfContained(os.DirFS(src)); err != nil {
			return Synced{}, err
		}
		if err := replaceSkill(working, os.DirFS(src)); err != nil {
			return Synced{}, err
		}
		p[name] = privateSkill{Source: shared, Manual: slices.Contains(list.manual, name)}
	case "resume":
		if !p.has(name) {
			return Synced{}, errors.New("this skill already follows the fleet")
		}
		if shared == "" {
			return Synced{}, errors.New("there is no shared version to resume")
		}
		if !fileExists(filepath.Join(baseline, skillFile)) {
			if err := m.fetchMissing(ctx, sourceEntry{Source: shared, Names: []string{name}}); err != nil {
				return Synced{}, err
			}
		}
		if fileExists(filepath.Join(working, skillFile)) {
			history := filepath.Join(m.paths.store, ".private-history")
			if err := os.MkdirAll(history, 0700); err != nil {
				return Synced{}, err
			}
			archive, err := os.MkdirTemp(history, name+"-*")
			if err != nil {
				return Synced{}, err
			}
			if err := os.CopyFS(archive, os.DirFS(working)); err != nil {
				_ = os.RemoveAll(archive)
				return Synced{}, err
			}
		}
		if err := replaceSkill(working, os.DirFS(baseline)); err != nil {
			return Synced{}, err
		}
		delete(p, name)
	case "publish":
		v, ok := p[name]
		if err := m.checkPrivateWrites(p, []string{name}); err != nil {
			return Synced{}, err
		}
		if !ok {
			return Synced{}, errors.New("only a private working copy can be published")
		}
		if !fileExists(filepath.Join(working, skillFile)) {
			return Synced{}, errors.New("private skill files are missing")
		}
		// TakeIf uses this same library lock to apply a newer fleet snapshot.
		deadline, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		sharedLock, err := filelock.Acquire(deadline, m.lib.Path(".packages.lock"))
		if err != nil {
			return Synced{}, err
		}
		err = selfContained(os.DirFS(working))
		if err == nil {
			err = replaceSkill(m.ownPath(name), os.DirFS(working))
		}
		if err == nil {
			err = m.edit(func(d *document) error {
				d.remove([]string{name})
				mode := Auto
				if v.Manual {
					mode = Manual
				}
				d.setMode([]string{name}, mode)
				return nil
			})
		}
		sharedLock.Close()
		if err != nil {
			return Synced{}, err
		}
		delete(p, name)
	default:
		return Synced{}, errors.New("choose private, resume, or publish")
	}
	if err := m.savePrivate(p); err != nil {
		return Synced{}, err
	}
	return m.activate(ctx, []string{name})
}

// checkNewPrivate prevents a private creation from replacing an existing local
// installation. An existing shared name must be explicitly detached.
func (m Manager) checkNewPrivate(names []string) error {
	if err := m.separateLibrary(); err != nil {
		return err
	}
	if err := checkNames(names); err != nil {
		return err
	}
	listed, err := m.List()
	if err != nil {
		return err
	}
	for i, name := range names {
		if slices.ContainsFunc(names[:i], func(s string) bool { return strings.EqualFold(s, name) }) {
			return fmt.Errorf("duplicate private skill name %q", name)
		}
		for _, existing := range listed {
			if strings.EqualFold(existing.Name, name) && !(existing.Private && !existing.Installed && !existing.SharedAvailable && existing.Name == name) {
				return fmt.Errorf("%s already exists; use skills sharing %s private to keep a private working copy", name, existing.Name)
			}
		}
		local, _ := m.localCopy(name)
		if taken(filepath.Join(m.paths.store, name)) || local != "" {
			return fmt.Errorf("%s already exists; use skills sharing %s private to keep a private working copy", name, name)
		}
	}

	return nil
}

// Commit the privacy marker before exposing the new files in the installed
// store. After a crash an empty reservation is safe to retry.
func (m Manager) installPrivate(name string, meta privateSkill, install func() error) error {
	p, err := m.privateSkills()
	if err != nil {
		return err
	}
	p[name] = meta
	if err := m.savePrivate(p); err != nil {
		return err
	}
	if err := install(); err != nil {
		delete(p, name)
		return errors.Join(err, m.savePrivate(p))
	}
	return nil
}

// AddPrivate stages the download outside every agent and the fleet library.
// Each selected skill is registered locally before sync can act on it.
func (m Manager) AddPrivate(ctx context.Context, from string, names []string, mode Mode) (Synced, error) {
	lock, err := m.paths.lockStore(ctx)
	if err != nil {
		return Synced{}, err
	}
	defer lock.Close()
	if err := checkNames(names); err != nil {
		return Synced{}, err
	}
	requested := slices.Clone(names)
	existing, err := m.privateSkills()
	if err != nil {
		return Synced{}, err
	}
	names = slices.DeleteFunc(slices.Clone(names), func(n string) bool {
		v, ok := existing[n]
		return ok && v.Source == from && fileExists(filepath.Join(m.paths.store, n, skillFile))
	})
	if len(names) == 0 {
		return m.activate(ctx, requested)
	}
	if err := m.checkNewPrivate(names); err != nil {
		return Synced{}, err
	}
	err = m.fetch(ctx, from, names, func(name string, files fs.FS) error {
		if err := selfContained(files); err != nil {
			return err
		}
		return m.installPrivate(name, privateSkill{Source: from, Manual: mode == Manual}, func() error { return replaceSkill(filepath.Join(m.paths.store, name), files) })
	})
	if err != nil {
		return Synced{}, err
	}
	return m.activate(ctx, requested)
}
