package webui

import (
	"context"
	"net/http"

	"github.com/darkyeg/spinup/internal/library"
)

type documentBackend interface {
	DocumentLibrary() library.Library
	RestoreDocument(context.Context, string, string, string) error
}

func (s *Server) revisionRoutes(read func(string, http.HandlerFunc), post func(string, string, http.HandlerFunc)) {
	read("library/revisions", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.b.DocumentLibrary().Revisions(r.Context())
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, list)
	})
	post(http.MethodPost, "library/revisions/preview", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Path, ID string }
		if !decodeBody(w, r, &in) {
			return
		}
		out, err := s.b.DocumentLibrary().PreviewRevision(r.Context(), in.Path, in.ID)
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, out)
	})
	post(http.MethodPost, "library/revisions/restore", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Path, ID, Before string }
		if !decodeBody(w, r, &in) {
			return
		}
		if err := s.b.RestoreDocument(r.Context(), in.Path, in.ID, in.Before); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, Done{Message: "Shared document restored."})
	})
}
