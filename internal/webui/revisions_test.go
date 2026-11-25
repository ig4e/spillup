package webui

import (
	"context"
	"github.com/darkyeg/spinup/internal/library"
	"net/http"
	"os"
	"testing"
)

type documentFixture struct {
	*fakeBackend
	lib library.Library
}

func (b documentFixture) DocumentLibrary() library.Library { return b.lib }
func (b documentFixture) RestoreDocument(ctx context.Context, path, id, before string) error {
	return b.lib.RestoreRevision(ctx, path, id, before)
}
func TestRevisionHistoryNeedsLocalBrowserSession(t *testing.T) {
	lib := library.At(t.TempDir())
	if err := os.WriteFile(lib.Path(library.Instructions), []byte("personal text"), 0600); err != nil {
		t.Fatal(err)
	}
	s := New(documentFixture{&fakeBackend{}, lib}, testToken)
	if w := do(t, s.Handler(), "GET", "/api/library/revisions", nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := do(t, s.Handler(), "GET", "/api/library/revisions", signedIn); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, from := range []reach{device, bridged} {
		mux := http.NewServeMux()
		s.apiRoutes(mux, from)
		for _, route := range []string{"library/revisions", "library/revisions/preview", "library/revisions/restore"} {
			method := "POST"
			if route == "library/revisions" {
				method = "GET"
			}
			if w := do(t, mux, method, "/api/"+route, nil); w.Code != 404 {
				t.Fatal(from, w.Code)
			}
			if w := do(t, s.Handler(), method, "/api/remote/peer/"+route, func(r *http.Request) { signedIn(r); r.Header.Set("Content-Type", "application/json") }); w.Code != 404 {
				t.Fatal("proxied", method, route, w.Code)
			}
		}
	}
}
