package runs

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/chatmodels"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Image engines: the harness that makes an image. A workspace chooses one
// (EngineAuto uses the first connected, Codex before Antigravity), a persona or
// a workflow step may override it.
const (
	EngineAuto        = "auto"
	EngineCodex       = agentdef.HarnessCodex
	EngineAntigravity = agentdef.HarnessAntigravity
)

// ValidImageEngine reports whether s names an engine (or auto).
func ValidImageEngine(s string) bool {
	return s == EngineAuto || slices.Contains(agentdef.ImageHarnesses, s)
}

// ImageEngineLabel is how an engine reads to the person.
func ImageEngineLabel(engine string) string {
	switch engine {
	case EngineAntigravity:
		return "Antigravity"
	case EngineCodex:
		return "Codex"
	}
	return "an image engine"
}

// PickImageEngine chooses the harness and model that make an image. preference
// is an engine, EngineAuto or "" (auto). A non-empty reason is why none can,
// worded for the person; it names the engine they asked for, or both when auto.
func PickImageEngine(catalog []chatmodels.Harness, preference string) (harness, model, reason string) {
	candidates := agentdef.ImageHarnesses
	if preference != "" && preference != EngineAuto {
		candidates = []string{preference}
	}
	why := map[string]string{}
	for _, name := range candidates {
		var found *chatmodels.Harness
		for i := range catalog {
			if catalog[i].Harness == name {
				found = &catalog[i]
			}
		}
		switch {
		case found == nil:
			why[name] = "isn't available"
		case !found.Available:
			why[name] = "isn't available: " + found.UnavailableReason
		default:
			picked := ""
			for _, m := range found.Models {
				if picked == "" || m.Default {
					picked = m.ID
				}
			}
			if picked != "" {
				return name, picked, ""
			}
			why[name] = "has no models available"
		}
	}
	if len(candidates) == 1 {
		return "", "", ImageEngineLabel(candidates[0]) + " " + why[candidates[0]]
	}
	var parts []string
	for _, name := range candidates {
		parts = append(parts, ImageEngineLabel(name)+" "+why[name])
	}
	return "", "", "no image engine is connected (" + strings.Join(parts, "; ") + ")"
}

// ImageEnginePreference is the engine a request should use: the persona's own
// choice when it has one, else the workspace's.
func ImageEnginePreference(ctx context.Context, q *store.Queries, ws uuid.UUID, personaEngine string) (string, error) {
	if personaEngine != "" && personaEngine != EngineAuto {
		return personaEngine, nil
	}
	engine, err := q.WorkspaceImageEngine(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return EngineAuto, nil
	}
	if err != nil {
		return "", err
	}
	if personaEngine == EngineAuto {
		return EngineAuto, nil
	}
	return engine, nil
}

// chatPersonaImageEngine is the image engine the persona answering a chat has
// chosen ("" inherits the workspace's).
func chatPersonaImageEngine(ctx context.Context, q *store.Queries, chat store.ChatSession) (string, error) {
	id := ChatSpeakerID(chat)
	if id == nil {
		return "", nil
	}
	p, err := q.GetPersona(ctx, store.GetPersonaParams{ID: *id, WorkspaceID: chat.WorkspaceID, UserID: chat.CreatedBy})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return p.ImageEngine, err
}

// SetWorkspaceImageEngine saves the workspace's choice.
func (s *Service) SetWorkspaceImageEngine(ctx context.Context, ws uuid.UUID, engine string) error {
	if !ValidImageEngine(engine) {
		return apperr.Invalid("engine", "must be auto, %s", strings.Join(agentdef.ImageHarnesses, " or "))
	}
	return store.New(s.Pool).SetWorkspaceImageEngine(ctx, store.SetWorkspaceImageEngineParams{ID: ws, ImageEngine: engine})
}

// WorkspaceImageEngine reads the workspace's choice.
func (s *Service) WorkspaceImageEngine(ctx context.Context, ws uuid.UUID) (string, error) {
	return ImageEnginePreference(ctx, store.New(s.Pool), ws, "")
}

// ImageEngineOptions lists what the person can choose from and whether each is
// connected right now, for the Settings and persona pickers.
func (s *Service) ImageEngineOptions(ctx context.Context, ws uuid.UUID) ([]ImageEngineOption, error) {
	catalog, err := s.ChatCatalog().List(ctx, store.New(s.Pool), ws)
	if err != nil {
		return nil, err
	}
	var out []ImageEngineOption
	for _, name := range agentdef.ImageHarnesses {
		_, _, reason := PickImageEngine(catalog, name)
		out = append(out, ImageEngineOption{Engine: name, Label: ImageEngineLabel(name), Available: reason == "", Reason: strings.TrimPrefix(reason, ImageEngineLabel(name)+" ")})
	}
	return out, nil
}

// ImageEngineOption is one engine and whether it can make images now.
type ImageEngineOption struct {
	Engine    string
	Label     string
	Available bool
	Reason    string
}
