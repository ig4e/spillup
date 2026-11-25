// Package skills installs the global skill list for every agent, keeps your own skills linked in,
// parks the rest, and edits the list and each skill's manual or auto mode.
package skills

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/darkyeg/spinup/internal/atomicfile"
	"github.com/darkyeg/spinup/internal/library"
	"github.com/darkyeg/spinup/internal/shell"
	"github.com/darkyeg/spinup/internal/source"
)

// OwnSource is the Source of skills that live in your library (~/.spinup/skills): yours, and shared between your machines.
const OwnSource = "your library"

const (
	// suggestedList is spinup's list, used until you change yours.
	suggestedList   = "skills/skills.json"
	failureTailSize = 800
	tokenChars      = 4
)

// Mode says whether an agent may pick a skill itself (Auto) or only runs it when called (Manual).
type Mode int

const (
	Auto Mode = iota
	Manual
)

func (m Mode) String() string {
	if m == Manual {
		return "manual"
	}
	return "auto"
}

type Skill struct {
	Private          bool
	SharedAvailable  bool
	Path             string
	Name             string
	Source           string
	Mode             Mode
	Installed        bool
	DescriptionChars int
}

// TokenEstimate is what the installed auto skills' descriptions cost in every session.
func TokenEstimate(skills []Skill) int {
	chars := 0
	for _, s := range skills {
		if s.Mode == Auto && s.Installed {
			chars += s.DescriptionChars
		}
	}
	return chars / tokenChars
}

// Synced reports a finished Sync.
type Synced struct {
	Count  int
	Agents []string
	Parked []string
	Failed []string
}

// installer fetches skills from a GitHub repo into the skill store; nil means this machine doesn't fetch.
type installer func(ctx context.Context, project, source string, names []string) (output string, err error)

type Manager struct {
	src     source.Source
	lib     library.Library
	paths   paths
	install installer
	logf    func(format string, a ...any)
}

// New reads spinup's suggested list from src and yours from lib; it reports progress through logf, which may be nil.
func New(src source.Source, lib library.Library, logf func(format string, a ...any)) Manager {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return Manager{src: src, lib: lib, paths: hostPaths(), install: npxInstall, logf: logf}
}

// Offline never fetches: it installs only the copies your library already has. The service uses it,
// since it may run without Node.js or a network.
func (m Manager) Offline() Manager {
	m.install = nil
	return m
}

// Sync installs every listed skill, links your own, parks the unlisted and applies the modes.
// When an install fails it still returns what was done, with an error naming the failed sources.
func (m Manager) Sync(ctx context.Context) (Synced, error) {
	lock, err := m.paths.lockStore(ctx)
	if err != nil {
		return Synced{}, err
	}
	defer lock.Close()
	return m.syncLocked(ctx)
}

func (m Manager) syncLocked(ctx context.Context) (Synced, error) {
	private, err := m.privateSkills()
	if err != nil {
		return Synced{}, err
	}
	list, err := m.manifest()
	if err != nil {
		return Synced{}, err
	}
	if err := m.checkPrivateWrites(private, slices.Collect(maps.Keys(private))); err != nil {
		return Synced{}, err
	}
	list = private.filtered(list)
	failed := m.installSources(ctx, list)
	own, err := m.publishOwn(ctx, private)
	if err != nil {
		return Synced{}, err
	}
	keep := sortedUnion(keepList(list, own), slices.Collect(maps.Keys(private)))
	for name := range private {
		if fileExists(filepath.Join(m.paths.store, name, skillFile)) {
			if err := m.paths.savePrivateAgentFolders(name); err != nil {
				return Synced{}, err
			}
		}
	}
	if err := m.paths.linkAll(ctx, keep); err != nil {
		return Synced{}, err
	}
	parked, err := m.parkUnlisted(keep, len(failed))
	if err != nil {
		return Synced{}, err
	}
	if err := m.paths.removeDanglingLinks(); err != nil {
		return Synced{}, err
	}
	if err := m.paths.applyModes(keep, list.manual); err != nil {
		return Synced{}, err
	}
	synced := Synced{Count: len(keep), Agents: agentNames(m.paths.agentsHere()), Parked: parked, Failed: failed}
	if len(failed) > 0 {
		return synced, fmt.Errorf("install failed for: %s (nothing was parked)", strings.Join(failed, ", "))
	}
	return synced, nil
}

// installSources installs each listed skill from your library's copy, fetching (and keeping a copy of)
// only the ones the library doesn't have yet.
func (m Manager) installSources(ctx context.Context, list manifest) (failed []string) {
	for _, entry := range list.sources {
		if err := m.fetchMissing(ctx, entry); err != nil {
			failed = append(failed, entry.Source)
			m.logf("%s", err)
			continue
		}
		if err := m.installFetched(ctx, entry.Names); err != nil {
			failed = append(failed, entry.Source)
			m.logf("%s: %v", entry.Source, err)
		}
	}
	return failed
}

func (m Manager) fetchMissing(ctx context.Context, entry sourceEntry) error {
	var missing []string
	for _, name := range entry.Names {
		if !fileExists(filepath.Join(m.fetchedCopy(name), skillFile)) {
			missing = append(missing, name)
		}
	}
	switch {
	case len(missing) == 0:
		return nil
	case m.install == nil:
		return fmt.Errorf("%s: %s not in your library yet; run `spinup skills sync` on a machine with Node.js",
			entry.Source, strings.Join(missing, ", "))
	}
	m.logf("Fetching from %s: %s", entry.Source, strings.Join(missing, ", "))
	return m.fetch(ctx, entry.Source, missing, func(name string, files fs.FS) error { return replaceSkill(m.fetchedCopy(name), files) })
}

// Fetch into an isolated project. A folder alias or missing upstream skill must never
// pick up a similarly named private skill from the machine's installed store.
func (m Manager) fetch(ctx context.Context, source string, names []string, save func(string, fs.FS) error) error {
	if m.install == nil {
		return errors.New("skill fetching is unavailable in offline mode")
	}
	project, err := os.MkdirTemp("", "spinup-skills-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(project)
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if output, err := m.install(ctx, project, source, names); err != nil {
		return fmt.Errorf("%s", tail(output+err.Error(), failureTailSize))
	}
	dir := filepath.Join(project, ".agents", "skills")
	root, err := filepath.EvalSymlinks(project)
	if err != nil {
		return err
	}
	if !insideProject(root, dir) {
		return fmt.Errorf("%s did not install skills into its temporary project", source)
	}
	folders := skillFolders(dir)
	selected := make([]string, 0, len(names))
	for _, name := range names {
		folder := name
		if !slices.Contains(folders, name) {
			i := slices.IndexFunc(folders, func(s string) bool { return calledName(dir, s) == name })
			if i < 0 {
				return fmt.Errorf("%s has no skill called %q", source, name)
			}
			folder = folders[i]
		}
		src := filepath.Join(dir, folder)
		if !insideProject(root, src) {
			return fmt.Errorf("downloaded skill %s must have its own project directory", name)
		}
		selected = append(selected, src)
	}
	for i, name := range names {
		if err := save(name, os.DirFS(selected[i])); err != nil {
			return fmt.Errorf("installing %s: %w", name, err)
		}
	}
	return nil
}

func insideProject(root, dir string) bool {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, resolved)
	return err == nil && filepath.IsLocal(rel) && rel != "."
}

func replaceSkill(dest string, files fs.FS) error {
	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, ".skill-stage-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	staged := filepath.Join(tmp, "skill")
	if err := os.CopyFS(staged, files); err != nil {
		return err
	}
	return swapDir(staged, dest, parent)
}

// installFetched installs the library's copy of each skill this machine doesn't have installed.
func (m Manager) installFetched(ctx context.Context, names []string) error {
	for _, name := range names {
		if fileExists(filepath.Join(m.paths.store, name, skillFile)) {
			continue
		}
		if err := m.paths.publish(ctx, ownSkill{name, os.DirFS(m.fetchedCopy(name))}); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func (m Manager) fetchedCopy(name string) string {
	return m.lib.Path(library.Fetched + "/" + name)
}

func (m Manager) publishOwn(ctx context.Context, private privateSkills) ([]ownSkill, error) {
	own, err := findOwn(m.lib.Files())
	if err != nil {
		return nil, err
	}
	own = slices.DeleteFunc(own, func(s ownSkill) bool { return private.has(s.name) })
	for _, skill := range own {
		if err := m.paths.publish(ctx, skill); err != nil {
			return nil, fmt.Errorf("own skill %s: %w", skill.name, err)
		}
	}
	if len(own) > 0 {
		m.logf("Own skills: %s", strings.Join(ownNames(own), ", "))
	}
	return own, nil
}

func (m Manager) parkUnlisted(keep []string, failedInstalls int) ([]string, error) {
	installed, err := m.paths.installed()
	if err != nil {
		return nil, err
	}
	parked := parkable(installed, keep, failedInstalls)
	if err := m.paths.park(parked); err != nil {
		return nil, err
	}
	if len(parked) > 0 {
		m.logf("Parked %d unlisted skills in %s (move one back to re-enable it)", len(parked), m.paths.parked)
	}
	return parked, nil
}

// Unlisted returns the installed skills that Sync would park.
func (m Manager) Unlisted() ([]string, error) {
	list, err := m.manifest()
	if err != nil {
		return nil, err
	}
	own, err := findOwn(m.lib.Files())
	if err != nil {
		return nil, err
	}
	installed, err := m.paths.installed()
	if err != nil {
		return nil, err
	}
	private, err := m.privateSkills()
	if err != nil {
		return nil, err
	}
	return parkable(installed, sortedUnion(keepList(list, own), slices.Collect(maps.Keys(private))), 0), nil
}

// List returns every listed and own skill, sorted by name.
func (m Manager) List() ([]Skill, error) {
	list, err := m.manifest()
	if err != nil {
		return nil, err
	}
	own, err := findOwn(m.lib.Files())
	if err != nil {
		return nil, err
	}
	sourceOf := map[string]string{}
	for _, entry := range list.sources {
		for _, name := range entry.Names {
			sourceOf[name] = entry.Source
		}
	}
	for _, skill := range own {
		sourceOf[skill.name] = OwnSource
	}
	private, err := m.privateSkills()
	if err != nil {
		return nil, err
	}
	var skills []Skill
	privateManual := private.filtered(manifest{}).manual
	for name, v := range private {
		sk := m.describe(name, v.Source, privateManual)
		sk.Private = true
		sk.Path = filepath.Join(m.paths.store, name)
		for shared := range sourceOf {
			if strings.EqualFold(shared, name) {
				sk.SharedAvailable = true
				delete(sourceOf, shared)
			}
		}
		skills = append(skills, sk)
	}
	for name, from := range sourceOf {
		sk := m.describe(name, from, list.manual)
		sk.SharedAvailable = true
		skills = append(skills, sk)
	}
	slices.SortFunc(skills, func(a, b Skill) int { return strings.Compare(a.Name, b.Name) })
	return skills, nil
}

func (m Manager) describe(name, from string, manual []string) Skill {
	skill := Skill{Name: name, Source: from}
	skill.Installed = dirExists(filepath.Join(m.paths.store, name))
	text, _ := os.ReadFile(filepath.Join(m.paths.store, name, skillFile))
	head := parseFrontmatter(string(text))
	skill.DescriptionChars = utf8.RuneCountInString(head.description)
	if slices.Contains(manual, name) || head.authorManual {
		skill.Mode = Manual
	}
	return skill
}

// Add lists names from source, as manual skills when mode is Manual, then runs Sync.
func (m Manager) Add(ctx context.Context, from string, names []string, mode Mode) (Synced, error) {
	lock, err := m.paths.lockStore(ctx)
	if err != nil {
		return Synced{}, err
	}
	defer lock.Close()
	if err := m.requireShared(names); err != nil {
		return Synced{}, err
	}

	err = m.edit(func(d *document) error {
		d.add(from, names, mode)
		return nil
	})
	if err != nil {
		return Synced{}, err
	}
	return m.syncLocked(ctx)
}

// Remove unlists names and drops the library folder of any that are your own, then runs Sync, which
// parks the installed copies. Nothing is lost: the parked copy is the skill as it was.
func (m Manager) Remove(ctx context.Context, names []string) (Synced, error) {
	lock, err := m.paths.lockStore(ctx)
	if err != nil {
		return Synced{}, err
	}
	defer lock.Close()
	if err := m.requireShared(names); err != nil {
		return Synced{}, err
	}

	mine, listed, err := m.splitOwn(names)
	if err != nil {
		return Synced{}, err
	}
	if len(mine) == 0 && len(listed) == 0 {
		return Synced{}, fmt.Errorf("none of %s is in your skills list or your own skills",
			strings.Join(names, ", "))
	}
	if len(listed) > 0 {
		if err = m.edit(func(d *document) error { d.remove(listed); return nil }); err != nil {
			return Synced{}, err
		}
	}
	for _, name := range mine {
		if err := os.RemoveAll(m.ownPath(name)); err != nil {
			return Synced{}, fmt.Errorf("removing your skill %s: %w", name, err)
		}
	}
	return m.syncLocked(ctx)
}

// splitOwn sorts names into the ones that are your own skills and the ones on your list, dropping any
// that are neither so the caller can tell nothing was matched.
func (m Manager) splitOwn(names []string) (mine, listed []string, err error) {
	own, err := findOwn(m.lib.Files())
	if err != nil {
		return nil, nil, err
	}
	onList, err := m.manifest()
	if err != nil {
		return nil, nil, err
	}
	for _, name := range names {
		switch {
		case slices.ContainsFunc(own, func(s ownSkill) bool { return s.name == name }):
			mine = append(mine, name)
		case slices.Contains(onList.listed(), name):
			listed = append(listed, name)
		}
	}
	return mine, listed, nil
}

// SetMode switches names between auto and manual and re-applies the modes without installing.
func (m Manager) SetMode(names []string, mode Mode) error { return m.SetModeScoped(names, mode, true) }

func (m Manager) SetModeScoped(names []string, mode Mode, allowPrivate bool) error {
	if err := checkNames(names); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	lock, err := m.paths.lockStore(ctx)
	if err != nil {
		return err
	}
	defer lock.Close()
	private, err := m.privateSkills()
	if err != nil {
		return err
	}
	if err := m.checkPrivateWrites(private, slices.Collect(maps.Keys(private))); err != nil {
		return err
	}
	shared := slices.DeleteFunc(slices.Clone(names), private.has)
	if len(shared) != len(names) && !allowPrivate {
		return errors.New("change private skills from the local dashboard")
	}
	if len(shared) > 0 {
		if err := m.edit(func(d *document) error { d.setMode(shared, mode); return nil }); err != nil {
			return err
		}
	}
	if len(shared) != len(names) {
		for _, name := range names {
			if n := private.name(name); n != "" {
				v := private[n]
				v.Manual = mode == Manual
				private[n] = v
			}
		}
		if err := m.savePrivate(private); err != nil {
			return err
		}
	}
	list, err := m.manifest()
	if err != nil {
		return err
	}
	own, err := findOwn(m.lib.Files())
	if err != nil {
		return err
	}
	list = private.filtered(list)
	keep := sortedUnion(keepList(list, own), slices.Collect(maps.Keys(private)))
	if err := m.paths.applyModes(keep, list.manual); err != nil {
		return err
	}
	m.logf("%s: %s. Restart Claude Code / Codex to apply.", strings.Join(names, ", "), mode)
	return nil
}

// edit changes your list; the first edit starts it as a copy of spinup's.
func (m Manager) edit(change func(*document) error) error {
	doc, err := m.document()
	if err != nil {
		return err
	}
	doc.obj.remove(keyComment)
	if err := change(&doc); err != nil {
		return err
	}
	path := m.lib.Path(library.SkillsList)
	if err := atomicfile.Write(path, doc.render(), 0o644); err != nil {
		return err
	}
	m.logf("Updated %s", path)
	return nil
}

func (m Manager) manifest() (manifest, error) {
	doc, err := m.document()
	return doc.manifest(), err
}

// document is your list when you have one, otherwise spinup's.
func (m Manager) document() (document, error) {
	data, err := fs.ReadFile(m.lib.Files(), library.SkillsList)
	if errors.Is(err, fs.ErrNotExist) {
		return readDocument(m.src.Data(), suggestedList)
	}
	if err != nil {
		return document{}, err
	}
	return parsed(m.lib.Path(library.SkillsList), data)
}

func readDocument(root fs.FS, name string) (document, error) {
	data, err := fs.ReadFile(root, name)
	if err != nil {
		return document{}, err
	}
	return parsed(name, data)
}

func parsed(name string, data []byte) (document, error) {
	doc, err := parseDocument(data)
	if err != nil {
		return document{}, fmt.Errorf("%s: %w", name, err)
	}
	return doc, nil
}

func keepList(list manifest, own []ownSkill) []string {
	keep := append(list.listed(), ownNames(own)...)
	slices.Sort(keep)
	return slices.Compact(keep)
}

func ownNames(own []ownSkill) []string {
	names := make([]string, len(own))
	for i, skill := range own {
		names[i] = skill.name
	}
	return names
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func tail(text string, size int) string {
	text = strings.TrimSpace(text)
	if len(text) > size {
		return text[len(text)-size:]
	}
	return text
}

func npxInstall(ctx context.Context, project, from string, names []string) (string, error) {
	npx, ok := shell.Find("npx")
	if !ok {
		shell.RefreshPath()
		if npx, ok = shell.Find("npx"); !ok {
			return "", errors.New("Node.js is needed for skills (npx not found). Install Node.js, then re-run")
		}
	}
	if from == "" || strings.HasPrefix(from, "-") {
		return "", errors.New("the skill source must be a repository")
	}
	for _, n := range names {
		if strings.HasPrefix(n, "-") {
			return "", errors.New("fetched skill names cannot start with a dash")
		}
	}
	args := []string{"--yes", "skills", "add", from, "-a", "codex"}
	for _, name := range names {
		args = append(args, "-s", name)
	}
	cmd := shell.CommandContext(ctx, npx, append(args, "-y")...)
	cmd.Dir = project
	cmd.Env = append(os.Environ(), "DISABLE_TELEMETRY=1")
	cmd.WaitDelay = 3 * time.Second
	output, err := cmd.CombinedOutput()
	return string(output), err
}
