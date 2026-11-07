package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/api"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/personas"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// Persona endpoints (wiki: Architecture/ADR - Assistant personas). Personas
// are private to their owner: every call is scoped to the signed-in person,
// so another person's persona is a 404. The avatar endpoints still answer 501
// until the avatar ticket implements them.

func personasNotImplemented() (int, api.Error) {
	return http.StatusNotImplemented, api.Error{Code: api.Internal, Message: "persona avatars are not implemented yet"}
}

func (s *Server) personas() *personas.Service { return &personas.Service{Pool: s.Pool} }

func personaOwner(ctx context.Context) personas.Owner {
	u, _, ws := me(ctx)
	return personas.Owner{WorkspaceID: ws, UserID: u.ID}
}

// personaAvatarURL busts the browser cache on every revision.
func personaAvatarURL(p store.Persona) *string {
	if p.AvatarKey == nil {
		return nil
	}
	u := fmt.Sprintf("/api/v1/personas/%s/avatar?v=%d", p.ID, p.Revision)
	return &u
}

func toPersonaSummary(p store.Persona) *api.PersonaSummary {
	return &api.PersonaSummary{Id: p.ID, Name: p.Name, Color: p.Color, AvatarUrl: personaAvatarURL(p)}
}

// toPersonas renders personas with their chat counts in one query.
func (s *Server) toPersonas(ctx context.Context, rows []store.Persona) ([]api.Persona, error) {
	out := make([]api.Persona, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]uuid.UUID, len(rows))
	for i, p := range rows {
		ids[i] = p.ID
	}
	counts, err := s.Q.PersonaChatCounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	n := map[uuid.UUID]int{}
	for _, c := range counts {
		n[c.PersonaID] = int(c.N)
	}
	for _, p := range rows {
		out = append(out, api.Persona{
			Id: p.ID, Name: p.Name, Tone: api.PersonaTone(p.Tone), Color: p.Color, AvatarUrl: personaAvatarURL(p),
			CustomInstructions: p.CustomInstructions, IsDefault: p.IsDefault, ChatCount: n[p.ID], LastUsedAt: p.LastUsedAt,
			Revision: int(p.Revision), CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
		})
	}
	return out, nil
}

func (s *Server) toPersona(ctx context.Context, p store.Persona) (api.Persona, error) {
	out, err := s.toPersonas(ctx, []store.Persona{p})
	if err != nil {
		return api.Persona{}, err
	}
	return out[0], nil
}

func toneArg(t *api.PersonaTone) *string {
	if t == nil {
		return nil
	}
	v := string(*t)
	return &v
}

func (s *Server) ListPersonas(ctx context.Context, _ api.ListPersonasRequestObject) (api.ListPersonasResponseObject, error) {
	rows, err := s.personas().List(ctx, personaOwner(ctx))
	if err != nil {
		return nil, err
	}
	items, err := s.toPersonas(ctx, rows)
	if err != nil {
		return nil, err
	}
	out := api.ListPersonas200JSONResponse{Personas: items}
	for _, p := range rows {
		if p.IsDefault {
			id := p.ID
			out.DefaultPersonaId = &id
		}
	}
	return out, nil
}

func (s *Server) CreatePersona(ctx context.Context, req api.CreatePersonaRequestObject) (api.CreatePersonaResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("body", "required")
	}
	b := req.Body
	p, err := s.personas().Create(ctx, personaOwner(ctx), personas.NewInput{
		Name: b.Name, Tone: toneArg(b.Tone), Color: b.Color, Instructions: b.CustomInstructions, IsDefault: deref(b.IsDefault),
	})
	if err != nil {
		return nil, err
	}
	out, err := s.toPersona(ctx, p)
	return api.CreatePersona201JSONResponse(out), err
}

func (s *Server) GetPersona(ctx context.Context, req api.GetPersonaRequestObject) (api.GetPersonaResponseObject, error) {
	p, err := s.personas().Get(ctx, personaOwner(ctx), req.Id)
	if err != nil {
		return nil, err
	}
	out, err := s.toPersona(ctx, p)
	return api.GetPersona200JSONResponse(out), err
}

func (s *Server) UpdatePersona(ctx context.Context, req api.UpdatePersonaRequestObject) (api.UpdatePersonaResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("body", "required")
	}
	b := req.Body
	if b.Revision < 1 || b.Revision > 2147483647 {
		return nil, apperr.Invalid("revision", "invalid revision")
	}
	p, err := s.personas().Update(ctx, personaOwner(ctx), req.Id, personas.Patch{
		Revision: int32(b.Revision), Name: b.Name, Tone: toneArg(b.Tone), Color: b.Color, Instructions: b.CustomInstructions,
	})
	if err != nil {
		return nil, err
	}
	out, err := s.toPersona(ctx, p)
	return api.UpdatePersona200JSONResponse(out), err
}

func (s *Server) DeletePersona(ctx context.Context, req api.DeletePersonaRequestObject) (api.DeletePersonaResponseObject, error) {
	var rev *int32
	if r := req.Params.Revision; r != nil {
		if *r < 1 || *r > 2147483647 {
			return nil, apperr.Invalid("revision", "invalid revision")
		}
		v := int32(*r)
		rev = &v
	}
	p, err := s.personas().Delete(ctx, personaOwner(ctx), req.Id, rev)
	if err != nil {
		return nil, err
	}
	if p.AvatarKey != nil && s.Blobs != nil {
		// The row is gone; a leftover blob is unreachable, so a failure here only costs storage.
		_ = s.Blobs.Delete(context.WithoutCancel(ctx), s.ArtifactBucket, *p.AvatarKey)
	}
	return api.DeletePersona204Response{}, nil
}

func (s *Server) SetDefaultPersona(ctx context.Context, req api.SetDefaultPersonaRequestObject) (api.SetDefaultPersonaResponseObject, error) {
	p, err := s.personas().SetDefault(ctx, personaOwner(ctx), req.Id)
	if err != nil {
		return nil, err
	}
	out, err := s.toPersona(ctx, p)
	return api.SetDefaultPersona200JSONResponse(out), err
}

func (s *Server) GetPersonaAvatar(ctx context.Context, _ api.GetPersonaAvatarRequestObject) (api.GetPersonaAvatarResponseObject, error) {
	code, body := personasNotImplemented()
	return api.GetPersonaAvatardefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) UploadPersonaAvatar(ctx context.Context, _ api.UploadPersonaAvatarRequestObject) (api.UploadPersonaAvatarResponseObject, error) {
	code, body := personasNotImplemented()
	return api.UploadPersonaAvatardefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) DeletePersonaAvatar(ctx context.Context, _ api.DeletePersonaAvatarRequestObject) (api.DeletePersonaAvatarResponseObject, error) {
	code, body := personasNotImplemented()
	return api.DeletePersonaAvatardefaultJSONResponse{StatusCode: code, Body: body}, nil
}
