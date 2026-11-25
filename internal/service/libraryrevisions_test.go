package service

import (
	"os"
	"testing"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/library"
)

func TestLibraryExchangePreservesPriorDocumentBeforeFirstPoll(t *testing.T) {
	m := artifactMachine(t)
	l := m.library.lib
	if err := os.WriteFile(l.Path(library.Instructions), []byte("local original"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	files := []library.File{{Path: library.Instructions, Data: []byte("from peer")}}
	incoming := api.Library{Protocol: 2, Files: files, Stamp: library.Stamp{Hash: library.Fingerprint(files), ChangedAt: now.Add(time.Second)}}
	if err := m.library.take(t.Context(), incoming, now); err != nil {
		t.Fatal(err)
	}
	list, err := l.Revisions(t.Context())
	if err != nil || len(list) != 2 {
		t.Fatal(list, err)
	}
	p, err := l.PreviewRevision(t.Context(), library.Instructions, list[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.RestoreRevision(t.Context(), library.Instructions, list[1].ID, p.Before); err != nil {
		t.Fatal(err)
	}
	result, err := m.library.snapshot(time.Now())
	if err != nil || library.Compare(result.Stamp, incoming.Stamp) != library.Give {
		t.Fatal(result.Stamp, err)
	}
	for _, f := range result.Files {
		if f.Path != library.Instructions {
			t.Fatal(f.Path)
		}
	}
}
