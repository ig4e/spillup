package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/darkyeg/spinup/internal/filelock"
)

const (
	revisionStore      = ".document-history.json"
	revisionBytes      = 128 << 10
	totalRevisionBytes = 4 << 20
	maxRevisions       = 200
	perDocument        = 12
)

type Revision struct {
	Path string    `json:"path"`
	ID   string    `json:"id"`
	At   time.Time `json:"at"`
}
type documentVersion struct {
	Revision
	Data []byte `json:"data"`
}
type documentArchive struct {
	Heads   map[string]string `json:"heads"`
	Entries []documentVersion `json:"entries"`
}
type RevisionPreview struct {
	Before string `json:"before"`
	Diff   string `json:"diff"`
	Same   bool   `json:"same"`
}

func documentPath(p string) bool {
	if p == Instructions {
		return true
	}
	parts := strings.Split(p, "/")
	return len(parts) == 3 && parts[0] == SkillsDir && parts[2] == skillFile && len(parts[1]) <= 128 && safePath(p) && strings.IndexFunc(parts[1], func(r rune) bool { return unicode.IsControl(r) || unicode.In(r, unicode.Cf) }) < 0
}
func documentText(b []byte) bool {
	return len(b) <= revisionBytes && utf8.Valid(b) && !slices.Contains(b, 0) && !strings.ContainsFunc(string(b), func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r' })
}
func documentID(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
func loadArchive(root *os.Root) (documentArchive, error) {
	var archive documentArchive
	data, _, err := readRevisionFile(root, revisionStore, 8<<20)
	if errors.Is(err, fs.ErrNotExist) {
		return documentArchive{Heads: map[string]string{}}, nil
	}
	if err != nil {
		return archive, err
	}
	if json.Unmarshal(data, &archive) != nil || archive.Heads == nil || len(archive.Entries) > maxRevisions {
		return archive, errors.New("invalid document history")
	}
	for _, v := range archive.Entries {
		if !documentPath(v.Path) || !documentText(v.Data) || documentID(v.Data) != v.ID {
			return archive, errors.New("invalid document revision")
		}
	}
	return archive, nil
}

func (a *documentArchive) update(files []File, now time.Time) bool {
	heads := map[string]string{}
	for _, f := range files {
		if !documentPath(f.Path) || !documentText(f.Data) {
			continue
		}
		id := documentID(f.Data)
		heads[f.Path] = id
		if a.Heads[f.Path] == id {
			continue
		}
		a.Entries = append(a.Entries, documentVersion{Revision{f.Path, id, now.UTC()}, f.Data})
	}
	if maps.Equal(a.Heads, heads) {
		return false
	}
	a.Heads = heads
	seen := map[string]int{}
	unique := map[[2]string]bool{}
	// Retain newest versions first so a busy document cannot consume every slot.
	kept := make([]documentVersion, 0, min(len(a.Entries), maxRevisions))
	used := 0
	for i := len(a.Entries) - 1; i >= 0 && len(kept) < maxRevisions; i-- {
		v := a.Entries[i]
		key := [2]string{v.Path, v.ID}
		if unique[key] || seen[v.Path] >= perDocument || used+len(v.Data) > totalRevisionBytes {
			continue
		}
		unique[key] = true
		seen[v.Path]++
		used += len(v.Data)
		kept = append(kept, v)
	}
	slices.Reverse(kept)
	a.Entries = kept
	return true
}

func saveArchive(root *os.Root, a documentArchive) error {
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	if len(data) > 8<<20 {
		return errors.New("document history exceeds the storage limit")
	}
	return writeRevisionFile(root, revisionStore, data, 0600)
}

// RememberDocuments captures the shared documents from a local library snapshot.
// The caller owns the library write lock so an older observation cannot follow a restore.
func (l Library) RememberDocuments(files []File) error {
	root, err := os.OpenRoot(l.Dir())
	if err != nil {
		return err
	}
	defer root.Close()
	archive, err := loadArchive(root)
	if err != nil {
		return err
	}
	if archive.update(files, time.Now()) {
		return saveArchive(root, archive)
	}
	return nil
}

func (l Library) withDocumentLock(ctx context.Context, do func(context.Context, *os.Root) error) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	lock, err := filelock.Acquire(ctx, l.Path(".packages.lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	root, err := os.OpenRoot(l.Dir())
	if err != nil {
		return err
	}
	defer root.Close()
	return do(ctx, root)
}

// documentInputs reads only authored skill instructions and the shared instructions.
// A linked document or skill directory is never an input.
func documentInputs(ctx context.Context, root *os.Root) ([]File, error) {
	paths := []string{Instructions}
	skills, err := childRevisionRoot(root, SkillsDir)
	if err == nil {
		defer skills.Close()
		dir, err := skills.Open(".")
		if err != nil {
			return nil, err
		}
		defer dir.Close()
		entries, err := dir.ReadDir(4097)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		if len(entries) > 4096 {
			return nil, errors.New("local history supports up to 4096 entries in the authored skills directory")
		}
		for _, entry := range entries {
			path := SkillsDir + "/" + entry.Name() + "/" + skillFile
			if entry.IsDir() && documentPath(path) {
				paths = append(paths, path)
			}
		}
		slices.Sort(paths)
	} else if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, errRevisionType) {
		return nil, err
	}
	var files []File
	total := 0
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		d, err := currentDocument(root, p)
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, errRevisionType) || errors.Is(err, errRevisionText) {
			continue
		}
		if err != nil {
			return nil, err
		}
		total += len(d)
		if total > SizeLimit || len(files) >= 4096 {
			return nil, errors.New("too many authored documents for local history")
		}
		files = append(files, File{Path: p, Data: d})
	}
	return files, nil
}

// ObserveDocuments checkpoints edits between library exchanges, including on a single machine.
func (l Library) ObserveDocuments(ctx context.Context, last string) (string, error) {
	if _, err := os.Stat(l.dir); errors.Is(err, fs.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return last, err
	}
	fingerprint := last
	err := l.withDocumentLock(ctx, func(ctx context.Context, root *os.Root) error {
		files, err := documentInputs(ctx, root)
		if err != nil {
			return err
		}
		current := "seen-" + Fingerprint(files)
		if last == current {
			return nil
		}
		a, err := loadArchive(root)
		if err != nil {
			return err
		}
		if a.update(files, time.Now()) {
			if err := saveArchive(root, a); err != nil {
				return err
			}
		}
		fingerprint = current
		return nil
	})
	return fingerprint, err
}

func (l Library) Revisions(ctx context.Context) ([]Revision, error) {
	result := []Revision{}
	err := l.withDocumentLock(ctx, func(ctx context.Context, root *os.Root) error {
		files, err := documentInputs(ctx, root)
		if err != nil {
			return err
		}
		a, err := loadArchive(root)
		if err != nil {
			return err
		}
		if a.update(files, time.Now()) {
			if err := saveArchive(root, a); err != nil {
				return err
			}
		}
		for i := len(a.Entries) - 1; i >= 0; i-- {
			result = append(result, a.Entries[i].Revision)
		}
		return nil
	})
	return result, err
}

func (a documentArchive) find(path, id string) (documentVersion, error) {
	if documentPath(path) && len(id) == 64 {
		for _, v := range a.Entries {
			if v.Path == path && v.ID == id {
				return v, nil
			}
		}
	}
	return documentVersion{}, errors.New("document revision unavailable")
}
func previewVersion(root *os.Root, path, id string) (documentVersion, RevisionPreview, error) {
	a, err := loadArchive(root)
	if err != nil {
		return documentVersion{}, RevisionPreview{}, err
	}
	v, err := a.find(path, id)
	if err != nil {
		return v, RevisionPreview{}, err
	}
	data, err := currentDocument(root, path)
	before := documentID(data)
	if errors.Is(err, fs.ErrNotExist) {
		before = "missing"
	} else if err != nil {
		return v, RevisionPreview{}, err
	}
	return v, RevisionPreview{before, documentDiff(data, v.Data), before == id}, nil
}
func (l Library) PreviewRevision(ctx context.Context, path, id string) (p RevisionPreview, err error) {
	err = l.withDocumentLock(ctx, func(ctx context.Context, root *os.Root) error {
		_, preview, err := previewVersion(root, path, id)
		p = preview
		return err
	})
	return
}
func (l Library) RestoreRevision(ctx context.Context, path, id, before string) error {
	return l.withDocumentLock(ctx, func(ctx context.Context, root *os.Root) error {
		v, p, err := previewVersion(root, path, id)
		if err != nil {
			return err
		}
		if p.Before != before {
			return errors.New("the document changed; preview it again")
		}
		if p.Same {
			return nil
		}
		// Capture the exact content about to be replaced, even between two daemon ticks.
		files, err := documentInputs(ctx, root)
		if err != nil {
			return err
		}
		archive, err := loadArchive(root)
		if err != nil {
			return err
		}
		if archive.update(files, time.Now()) {
			if err := saveArchive(root, archive); err != nil {
				return err
			}
		}
		parent, base, err := revisionParent(root, path, true)
		if err != nil {
			return err
		}
		if parent != root {
			defer parent.Close()
		}
		mode := fs.FileMode(0600)
		current, info, err := readRevisionFile(parent, base, revisionBytes)
		if err == nil {
			if documentID(current) != before {
				return errors.New("the document changed; preview it again")
			}
			mode = info.Mode().Perm()
		} else if !errors.Is(err, fs.ErrNotExist) || before != "missing" {
			return errors.New("the document changed; preview it again")
		}
		if err := writeRevisionFile(parent, base, v.Data, mode); err != nil {
			return err
		}
		for i, f := range files {
			if f.Path == path {
				files = slices.Delete(files, i, i+1)
				break
			}
		}
		files = append(files, File{Path: path, Data: v.Data})
		if archive.update(files, time.Now()) {
			return saveArchive(root, archive)
		}
		return nil
	})
}
