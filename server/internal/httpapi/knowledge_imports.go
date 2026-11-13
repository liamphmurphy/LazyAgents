package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

func knowledgeJob(job store.KnowledgeImport) map[string]any {
	return map[string]any{"id": job.ID, "state": job.State, "folder": job.Folder, "policy": job.Policy, "plan": json.RawMessage(job.Plan), "report": json.RawMessage(job.Report), "files_done": job.FilesDone, "error": job.Error, "created_at": job.CreatedAt, "updated_at": job.UpdatedAt}
}
func (s *Server) knowledgeImportRoutes(mux *http.ServeMux) {
	respond := func(w http.ResponseWriter, r *http.Request, job store.KnowledgeImport, err error, status int) {
		if errors.Is(err, pgx.ErrNoRows) {
			// A missing job, someone else's, or one past the state the
			// action needs (confirming twice) is not a server error.
			err = apperr.ErrNotFound
		}
		if err != nil {
			s.responseError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(knowledgeJob(job))
	}
	mux.HandleFunc("POST /api/v1/knowledge/imports", func(w http.ResponseWriter, r *http.Request) {
		u, _, ws := me(r.Context())
		job, err := s.Wiki.QueueVault(r.Context(), ws, u.ID, r.URL.Query().Get("folder"), r.Body)
		respond(w, r, job, err, http.StatusAccepted)
	})
	mux.HandleFunc("GET /api/v1/knowledge/imports", func(w http.ResponseWriter, r *http.Request) {
		u, _, ws := me(r.Context())
		jobs, err := s.Q.ListKnowledgeImports(r.Context(), store.ListKnowledgeImportsParams{WorkspaceID: ws, UserID: u.ID})
		if err != nil {
			s.responseError(w, r, err)
			return
		}
		out := []map[string]any{}
		for _, job := range jobs {
			out = append(out, knowledgeJob(job))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(map[string]any{"imports": out})
	})
	mux.HandleFunc("GET /api/v1/knowledge/imports/{id}", func(w http.ResponseWriter, r *http.Request) {
		u, _, ws := me(r.Context())
		id, err := knowledgeID(r, "id")
		if err != nil {
			s.responseError(w, r, err)
			return
		}
		job, err := s.Q.GetKnowledgeImport(r.Context(), store.GetKnowledgeImportParams{ID: id, WorkspaceID: ws, UserID: u.ID})
		respond(w, r, job, err, 200)
	})
	mux.HandleFunc("POST /api/v1/knowledge/imports/{id}/confirm", func(w http.ResponseWriter, r *http.Request) {
		u, _, ws := me(r.Context())
		id, err := knowledgeID(r, "id")
		if err != nil {
			s.responseError(w, r, err)
			return
		}
		var in struct {
			Policy string `json:"policy"`
		}
		if err = knowledgeJSON(r, &in); err != nil {
			s.responseError(w, r, err)
			return
		}
		if in.Policy != "skip" && in.Policy != "replace" && in.Policy != "keep_both" {
			s.responseError(w, r, apperr.Invalid("policy", "choose skip, replace or keep_both"))
			return
		}
		job, err := s.Q.ConfirmKnowledgeImport(r.Context(), store.ConfirmKnowledgeImportParams{ID: id, WorkspaceID: ws, UserID: u.ID, Policy: in.Policy})
		respond(w, r, job, err, 202)
	})
	mux.HandleFunc("POST /api/v1/knowledge/imports/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		u, _, ws := me(r.Context())
		id, err := knowledgeID(r, "id")
		if err != nil {
			s.responseError(w, r, err)
			return
		}
		job, err := s.Q.CancelKnowledgeImport(r.Context(), store.CancelKnowledgeImportParams{ID: id, WorkspaceID: ws, UserID: u.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			if _, getErr := s.Q.GetKnowledgeImport(r.Context(), store.GetKnowledgeImportParams{ID: id, WorkspaceID: ws, UserID: u.ID}); getErr == nil {
				err = apperr.Conflict("only an import that hasn't been confirmed can be canceled; undo a finished one")
			} else {
				err = apperr.ErrNotFound
			}
		}
		respond(w, r, job, err, 200)
	})
	mux.HandleFunc("POST /api/v1/knowledge/imports/{id}/undo", func(w http.ResponseWriter, r *http.Request) {
		u, _, ws := me(r.Context())
		id, err := knowledgeID(r, "id")
		if err != nil {
			s.responseError(w, r, err)
			return
		}
		if err = s.Wiki.UndoVault(r.Context(), ws, u.ID, id); err != nil {
			s.responseError(w, r, err)
			return
		}
		job, err := s.Q.GetKnowledgeImport(r.Context(), store.GetKnowledgeImportParams{ID: id, WorkspaceID: ws, UserID: u.ID})
		respond(w, r, job, err, 200)
	})
}
