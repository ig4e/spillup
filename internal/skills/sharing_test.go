package skills

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/darkyeg/spinup/internal/library"
)

func content(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func assertNoPrivateBytes(t *testing.T, lib library.Library, needles ...string) {
	t.Helper()
	files, err := lib.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range files {
		for _, needle := range needles {
			if strings.Contains(v.Path, needle) || bytes.Contains(v.Data, []byte(needle)) {
				t.Fatalf("unexpected private content in shared file %s", v.Path)
			}
		}
	}
}

func TestPrivateSkillsFromStartStayOutOfSnapshotsAndFetchedCache(t *testing.T) {
	f := newFixture(t, `{}`)
	m := f.manager
	ctx := t.Context()
	created, err := m.CreateScoped(ctx, "local-author", "private instructions", true)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "local-import")
	write(t, filepath.Join(path, skillFile), "private imported bytes")
	if _, err := m.ImportScoped(ctx, path, true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AddPrivate(ctx, "owner/private-repository", []string{"local-fetch"}, Manual); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(f.paths.store, "local-fetch", "notes"), "only this host")
	if res, err := m.Offline().Sync(ctx); err != nil || len(res.Parked) > 0 || res.Count != 3 {
		t.Fatal(res, err)
	}
	list, err := m.List()
	if err != nil || len(list) != 3 {
		t.Fatal(list, err)
	}
	for _, s := range list {
		if !s.Private || s.SharedAvailable || !s.Installed || s.Path == "" {
			t.Fatal(s)
		}
	}
	if _, err := m.Adopt(ctx, []string{"local-fetch"}); err == nil {
		t.Fatal("ordinary adopt published a private skill")
	}
	if _, err := m.Remove(ctx, []string{"local-author"}); err == nil {
		t.Fatal("ordinary removal touched a private skill")
	}
	if err := m.SetMode([]string{"local-author"}, Manual); err != nil {
		t.Fatal(err)
	}
	if err := m.SetModeScoped([]string{"local-author"}, Auto, false); err == nil {
		t.Fatal("remote mode changed a private skill")
	}
	if got := content(t, filepath.Join(created.Path, skillFile)); !strings.Contains(got, "private instructions") {
		t.Fatal(got)
	}
	assertNoPrivateBytes(t, f.lib, "local-author", "private imported bytes", "local-fetch", "local-import", "only this host", "owner/private-repository")
}

func TestDetachedSkillSurvivesLibraryReplacementAndExplicitResumeOrPublish(t *testing.T) {
	for _, source := range []string{"authored", "fetched"} {
		t.Run(source, func(t *testing.T) {
			f := newFixture(t, `{}`)
			m := f.manager
			ctx := t.Context()
			var baseline string
			if source == "authored" {
				baseline = "skills/example/SKILL.md"
				write(t, f.lib.Path(baseline), "fleet original")
			} else {
				baseline = "fetched/example/SKILL.md"
				write(t, f.lib.Path(library.SkillsList), `{"sources":{"owner/example":["example"]}}`)
				write(t, f.lib.Path(baseline), "fleet original")
			}
			if _, err := m.Sync(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Sharing(ctx, "example", "private"); err != nil {
				t.Fatal(err)
			}
			active := filepath.Join(f.paths.store, "example", skillFile)
			write(t, active, "private marker")
			if err := m.SetMode([]string{"example"}, Manual); err != nil {
				t.Fatal(err)
			}
			assertNoPrivateBytes(t, f.lib, "private marker", "allow_implicit_invocation")
			files, err := f.lib.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			for i := range files {
				if files[i].Path == baseline {
					files[i].Data = []byte("new remote copy")
				}
			}
			if err := f.lib.Take(files, time.Now()); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Offline().Sync(ctx); err != nil {
				t.Fatal(err)
			}
			if got := content(t, active); got != "private marker" {
				t.Fatal(got)
			}
			if _, err := m.Sharing(ctx, "example", "resume"); err != nil {
				t.Fatal(err)
			}
			if got := content(t, active); got != "new remote copy" {
				t.Fatal(got)
			}
			hist, err := filepath.Glob(filepath.Join(f.paths.store, ".private-history", "*", skillFile))
			if err != nil || len(hist) != 1 || content(t, hist[0]) != "private marker" {
				t.Fatal(hist, err)
			}
			if _, err := m.Sharing(ctx, "example", "private"); err != nil {
				t.Fatal(err)
			}
			write(t, active, "explicitly published")
			if _, err := m.Sharing(ctx, "example", "publish"); err != nil {
				t.Fatal(err)
			}
			if got := content(t, m.ownPath("example")+"/"+skillFile); got != "explicitly published" {
				t.Fatal(got)
			}
			list, err := m.List()
			if err != nil || len(list) != 1 || list[0].Private || !list[0].SharedAvailable {
				t.Fatal(list, err)
			}
		})
	}
}

func TestPrivateWorkingCopySurvivesFleetRemovalAndMissingRepositoryAlias(t *testing.T) {
	f := newFixture(t, `{}`)
	ctx := t.Context()
	m := f.manager
	if _, err := m.CreateScoped(ctx, "hidden", "local-only-note", true); err != nil {
		t.Fatal(err)
	}
	// An unrelated upstream lookup must never adopt the installed hidden folder by a frontmatter alias.
	write(t, filepath.Join(f.paths.store, "hidden", skillFile), "---\nname: alias\n---\nlocal-only-note")
	m.install = func(context.Context, string, string, []string) (string, error) { return "", nil }
	if _, err := m.Add(ctx, "owner/public", []string{"alias"}, Auto); err == nil {
		t.Fatal("missing result was treated as installed")
	}
	assertNoPrivateBytes(t, f.lib, "local-only-note")
	if _, err := m.Sharing(ctx, "hidden", "resume"); err == nil {
		t.Fatal("resumed a nonexistent shared copy")
	}
	write(t, f.lib.Path(library.SkillsList), `{}`)
	if res, err := m.Sync(ctx); err != nil || slices.Contains(res.Parked, "hidden") {
		t.Fatal(res, err)
	}
}

func TestDetachedSkillRejectsLinkBackToSharedBaseline(t *testing.T) {
	f := newFixture(t, `{}`)
	m := f.manager
	ctx := t.Context()
	baseline := f.lib.Path("skills/review/SKILL.md")
	write(t, baseline, "fleet original")
	if _, err := m.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	active := filepath.Join(f.paths.store, "review", skillFile)
	if err := os.Remove(active); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(baseline, active); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := m.Sharing(ctx, "review", "private"); err == nil {
		t.Fatal("private skill retained a shared symlink")
	}
	list, err := m.List()
	if err != nil || len(list) != 1 || list[0].Private {
		t.Fatal(list, err)
	}
	if got := content(t, baseline); got != "fleet original" {
		t.Fatal(got)
	}
}

func TestFetchedSourceMustBeInsideTemporaryProject(t *testing.T) {
	f := newFixture(t, `{}`)
	m := f.manager
	other := t.TempDir()
	write(t, filepath.Join(other, "skills", "requested", skillFile), "machine-private bytes")
	m.install = func(_ context.Context, project, _ string, _ []string) (string, error) {
		if err := os.Symlink(other, filepath.Join(project, ".agents")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		return "", nil
	}
	if _, err := m.Add(t.Context(), "owner/source", []string{"requested"}, Auto); err == nil {
		t.Fatal("download followed an external project link")
	}
	assertNoPrivateBytes(t, f.lib, "machine-private bytes")
}

func TestCustomLibraryCannotExposePrivateStore(t *testing.T) {
	f := newFixture(t, `{}`)
	m := f.manager
	m.lib = library.At(filepath.Dir(f.paths.store))
	if _, err := m.CreateScoped(t.Context(), "hidden", "sensitive notes", true); err == nil {
		t.Fatal("private data would reside under the shared skills tree")
	}
	assertNoPrivateBytes(t, m.lib, "sensitive notes")
}

func TestPrivateSkillAdoptionAndInterruptedCreation(t *testing.T) {
	f := newFixture(t, `{}`)
	m := f.manager
	other := filepath.Join(f.paths.store, "other-local", skillFile)
	write(t, other, "a separate unmanaged skill")
	agentCopy := filepath.Join(f.paths.claudeSkills(), "draft", skillFile)
	write(t, agentCopy, "machine draft")
	if _, err := m.Sharing(t.Context(), "draft", "private"); err != nil {
		t.Fatal(err)
	}
	if content(t, other) != "a separate unmanaged skill" {
		t.Fatal("scope change parked another local skill")
	}
	if _, err := m.Sharing(t.Context(), "other-local", "private"); err != nil {
		t.Fatal(err)
	}
	if res, err := m.Offline().Sync(t.Context()); err != nil || len(res.Parked) != 0 {
		t.Fatal(res, err)
	}
	if got := content(t, agentCopy); got != "machine draft" {
		t.Fatal(got)
	}
	gone, err := m.Trash()
	if err != nil || len(gone) != 0 {
		t.Fatal(gone, err)
	}
	p, err := m.privateSkills()
	if err != nil {
		t.Fatal(err)
	}
	p["pending"] = privateSkill{Source: "this machine"}
	if err := m.savePrivate(p); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateScoped(t.Context(), "pending", "after interrupted creation", true); err != nil {
		t.Fatal(err)
	}
	assertNoPrivateBytes(t, f.lib, "machine draft", "pending", "after interrupted creation")
}

func TestPrivateSkillModeRechecksWorkingPathsBeforeWriting(t *testing.T) {
	f := newFixture(t, `{}`)
	m := f.manager
	ctx := t.Context()
	if _, err := m.Create(ctx, "notes", "shared"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Sharing(ctx, "notes", "private"); err != nil {
		t.Fatal(err)
	}
	sharedPolicy := filepath.Join(m.ownPath("notes"), codexPolicyDirName)
	if err := os.MkdirAll(sharedPolicy, 0755); err != nil {
		t.Fatal(err)
	}
	activePolicy := filepath.Join(f.paths.store, "notes", codexPolicyDirName)
	if err := os.Symlink(sharedPolicy, activePolicy); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := m.SetMode([]string{"notes"}, Manual); err == nil {
		t.Fatal("mode followed a link into fleet files")
	}
	if _, err := m.Sync(ctx); err == nil {
		t.Fatal("sync treated linked private settings as writable")
	}
	if _, err := os.Stat(filepath.Join(sharedPolicy, codexPolicyFile)); !os.IsNotExist(err) {
		t.Fatal("shared policy was changed")
	}
}

func TestRestoreSharedDocumentPreservesPrivateFork(t *testing.T) {
	f := newFixture(t, `{}`)
	m, ctx := f.manager, t.Context()
	if _, err := m.Create(ctx, "notes", "shared baseline"); err != nil {
		t.Fatal(err)
	}
	records, err := f.lib.Revisions(ctx)
	if err != nil || len(records) != 1 {
		t.Fatal(records, err)
	}
	original := records[0]
	if _, err := m.Sharing(ctx, "notes", "private"); err != nil {
		t.Fatal(err)
	}
	private := filepath.Join(f.paths.store, "notes", skillFile)
	write(t, private, "private working copy")
	shared := f.lib.Path(original.Path)
	write(t, shared, "next shared version")
	preview, err := f.lib.PreviewRevision(ctx, original.Path, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.RestoreAuthored(ctx, "notes", original.ID, preview.Before); err != nil {
		t.Fatal(err)
	}
	if got := content(t, private); got != "private working copy" {
		t.Fatal("changed private copy")
	}
	if got := content(t, shared); !strings.Contains(got, "shared baseline") {
		t.Fatal("failed to restore shared file")
	}
	assertNoPrivateBytes(t, f.lib, "private working copy")
}
