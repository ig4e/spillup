package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDocumentRevisionsObserveRestoreAndRejectStaleReview(t *testing.T) {
	l := At(t.TempDir())
	ctx := t.Context()
	put(t, l.Path(Instructions), "first\ncommon\n")
	fingerprint, err := l.ObserveDocuments(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	put(t, l.Path(Instructions), "second\ncommon\n")
	if _, err = l.ObserveDocuments(ctx, fingerprint); err != nil {
		t.Fatal(err)
	}
	list, err := l.Revisions(ctx)
	if err != nil || len(list) != 2 {
		t.Fatal(list, err)
	}
	prior := list[1]
	p, err := l.PreviewRevision(ctx, prior.Path, prior.ID)
	if err != nil || !strings.Contains(p.Diff, "-second\n+first\n") || p.Same {
		t.Fatal(p, err)
	}
	put(t, l.Path(Instructions), "third\n")
	if err := l.RestoreRevision(ctx, prior.Path, prior.ID, p.Before); err == nil || read(t, l.Path(Instructions)) != "third\n" {
		t.Fatal("stale edit lost")
	}
	p, err = l.PreviewRevision(ctx, prior.Path, prior.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.RestoreRevision(ctx, prior.Path, prior.ID, p.Before); err != nil {
		t.Fatal(err)
	}
	if got := read(t, l.Path(Instructions)); got != "first\ncommon\n" {
		t.Fatal(got)
	}
	list, err = l.Revisions(ctx)
	if err != nil || len(list) != 3 || list[0].ID != prior.ID || list[1].ID != documentID([]byte("third\n")) {
		t.Fatal(list, err)
	}
	if err := os.Remove(l.Path(Instructions)); err != nil {
		t.Fatal(err)
	}
	p, err = l.PreviewRevision(ctx, prior.Path, prior.ID)
	if err != nil || p.Before != "missing" {
		t.Fatal(p, err)
	}
	if err := l.RestoreRevision(ctx, prior.Path, prior.ID, p.Before); err != nil {
		t.Fatal(err)
	}
}
func TestHistoryExcludesPrivateSkillsAndSensitiveFiles(t *testing.T) {
	l := At(t.TempDir())
	for _, p := range []string{"skills/ok/SKILL.md", "AGENTS.md"} {
		put(t, l.Path(p), "shared\n")
	}
	for _, p := range []string{"fetched/pkg/SKILL.md", "local/skills/secret/SKILL.md", "skills/.hidden/SKILL.md", "skills/ok/credentials.json", "skills/ok/private.md", "local/mcp-private.json", "mcp-servers.json", "skills/invalid/SKILL.md"} {
		text := "private-marker"
		if p == "skills/invalid/SKILL.md" {
			text += "\x00"
		}
		put(t, l.Path(p), text)
	}
	list, err := l.Revisions(t.Context())
	if err != nil || len(list) != 2 {
		t.Fatal(list, err)
	}
	for _, r := range list {
		if !documentPath(r.Path) {
			t.Fatal(r)
		}
	}
	for _, p := range []string{"../AGENTS.md", "skills/../../SKILL.md", "skills/.hidden/SKILL.md", "fetched/a/SKILL.md", "local/mcp-private.json"} {
		if _, err := l.PreviewRevision(t.Context(), p, list[0].ID); err == nil {
			t.Fatal("accepted", p)
		}
	}
	for _, p := range []string{"local/mcp-private.json", "fetched/pkg/SKILL.md", "skills/ok/private.md"} {
		if strings.Contains(read(t, l.Path(revisionStore)), p) {
			t.Fatal("captured private metadata")
		}
	}
	files, err := l.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(paths(files), revisionStore) {
		t.Fatal("archive left machine")
	}
}
func TestRevisionSymlinkCannotReadOrReplaceOutsideLibrary(t *testing.T) {
	l := At(t.TempDir())
	p := "skills/notes/SKILL.md"
	put(t, l.Path(p), "was")
	versions, err := l.Revisions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.RemoveAll(l.Path("skills/notes")); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	put(t, filepath.Join(external, skillFile), "external")
	if err = os.Symlink(external, l.Path("skills/notes")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if _, err = l.PreviewRevision(t.Context(), p, versions[0].ID); err == nil {
		t.Fatal("previewed a linked folder")
	}
	if err = l.RestoreRevision(t.Context(), p, versions[0].ID, documentID([]byte("external"))); err == nil {
		t.Fatal("restored through linked folder")
	}
	if read(t, filepath.Join(external, skillFile)) != "external" {
		t.Fatal("changed outside library")
	}
	if err = os.Remove(l.Path(revisionStore)); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(filepath.Join(external, skillFile), l.Path(revisionStore)); err != nil {
		t.Fatal(err)
	}
	if _, err = l.Revisions(t.Context()); err == nil {
		t.Fatal("followed linked archive")
	}
}
func TestDocumentHistoryIsBoundedAndContentDeduplicated(t *testing.T) {
	a := documentArchive{Heads: map[string]string{}}
	now := time.Now()
	for i := range perDocument + 5 {
		a.update([]File{{Path: Instructions, Data: []byte(fmt.Sprint(i))}}, now.Add(time.Duration(i)))
	}
	if len(a.Entries) != perDocument {
		t.Fatal(len(a.Entries))
	}
	for range 9 {
		a.update([]File{{Path: Instructions, Data: []byte("new")}}, now)
		a.update([]File{{Path: Instructions, Data: []byte("older")}}, now)
	}
	if len(a.Entries) != perDocument {
		t.Fatal(len(a.Entries))
	}
	seen := map[string]bool{}
	for _, v := range a.Entries {
		if seen[v.ID] {
			t.Fatal("evicted distinct version for duplicate")
		}
		seen[v.ID] = true
	}
	var files []File
	for i := range maxRevisions + 1 {
		files = append(files, File{Path: fmt.Sprintf("skills/s-%d/SKILL.md", i), Data: make([]byte, 1024)})
		for j := range files[i].Data {
			files[i].Data[j] = 'a'
		}
	}
	a.update(files, now)
	if len(a.Entries) != maxRevisions {
		t.Fatal(len(a.Entries))
	}
	for i := range files {
		files[i].Data = []byte(strings.Repeat("b", revisionBytes))
	}
	a.update(files, now)
	total := 0
	for _, v := range a.Entries {
		total += len(v.Data)
	}
	if total > totalRevisionBytes || total == 0 {
		t.Fatal(total)
	}
}
func TestLocalEditsPublishAfterAcceptedClockSkew(t *testing.T) {
	now := time.Now().UTC()
	tr, err := NewTracker(filepath.Join(t.TempDir(), "tracker.json"))
	if err != nil {
		t.Fatal(err)
	}
	incoming := Stamp{now.Add(20 * time.Second), "synced"}
	if err = tr.Took(incoming); err != nil {
		t.Fatal(err)
	}
	after, err := tr.Observe("restored", now, now)
	if err != nil || Compare(after, incoming) != Give || after.FromTheFuture(now) {
		t.Fatal(after, err)
	}
}
func TestHistoryObservesCancellation(t *testing.T) {
	l := At(t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := l.ObserveDocuments(ctx, ""); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestDocumentDiffEscapesDisplayControls(t *testing.T) {
	d := documentDiff([]byte("safe\n"), []byte("with\rreturn\u202Etext\r\n"))
	if !strings.Contains(d, `with\rreturn\u{202E}text`) || strings.ContainsRune(d, '\u202e') {
		t.Fatal("ambiguous terminal preview")
	}
}
