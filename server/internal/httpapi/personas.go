package httpapi

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/personas"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Persona endpoints (wiki: Architecture/ADR - Assistant personas). Personas
// are private to their owner: every call is scoped to the signed-in person,
// so another person's persona is a 404. Avatars are in persona_avatar.go.

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
	return &api.PersonaSummary{Id: p.ID, Name: p.Name, Color: p.Color, AvatarUrl: personaAvatarURL(p), Capabilities: toPersonaCapabilities(p)}
}

func toPersonaCapabilities(p store.Persona) *api.PersonaCapabilities {
	return &api.PersonaCapabilities{Code: p.CanCode, Images: p.CanGenerateImages}
}

// reposOrEmpty keeps the contract's required repos array from being null.
func reposOrEmpty(r []string) []string {
	if r == nil {
		return []string{}
	}
	return r
}

// capabilityArgs splits the contract's capabilities object; absent means unchanged.
func capabilityArgs(c *api.PersonaCapabilities) (code, images *bool) {
	if c == nil {
		return nil, nil
	}
	return &c.Code, &c.Images
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
			CustomInstructions: p.CustomInstructions, Capabilities: api.PersonaCapabilities{Code: p.CanCode, Images: p.CanGenerateImages}, Repos: reposOrEmpty(p.Repos), IsDefault: p.IsDefault, ChatCount: n[p.ID], LastUsedAt: p.LastUsedAt,
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
	code, images := capabilityArgs(b.Capabilities)
	p, err := s.personas().Create(ctx, personaOwner(ctx), personas.NewInput{
		Name: b.Name, Tone: toneArg(b.Tone), Color: b.Color, Instructions: b.CustomInstructions, IsDefault: deref(b.IsDefault),
		Code: code, Images: images, Repos: b.Repos,
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
	code, images := capabilityArgs(b.Capabilities)
	p, err := s.personas().Update(ctx, personaOwner(ctx), req.Id, personas.Patch{
		Revision: int32(b.Revision), Name: b.Name, Tone: toneArg(b.Tone), Color: b.Color, Instructions: b.CustomInstructions,
		Code: code, Images: images, Repos: b.Repos,
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
	s.deleteAvatarBlob(ctx, p.AvatarKey)
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
