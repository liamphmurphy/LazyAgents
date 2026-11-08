package agentapi

import (
	"context"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

type ListSavedLinksIn struct {
	Status           string `json:"status,omitempty" jsonschema:"to_read or done; empty lists both"`
	Limit            int    `json:"limit,omitempty" jsonschema:"Up to 50, default 20"`
	IncludeSummaries bool   `json:"include_summaries,omitempty" jsonschema:"Also return each link's saved summary"`
}

type savedLink struct {
	Title      string `json:"title"`
	URL        string `json:"url"`
	Site       string `json:"site,omitempty"`
	Note       string `json:"note,omitempty"`
	Video      bool   `json:"video"`
	Status     string `json:"status"`
	HasSummary bool   `json:"has_summary"`
	Summary    string `json:"summary,omitempty"`
	SavedAt    string `json:"saved_at"`
}

// listSavedLinks shows a chat the Library links of the person it's talking to.
func listSavedLinks(ctx context.Context, d Deps, c agents.Caller, in ListSavedLinksIn) (any, error) {
	s, err := chatScopeFor(ctx, d, c)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, apperr.Conflict("list_saved_links only works in a chat with a person")
	}
	var status *string
	switch in.Status {
	case "":
	case "to_read", "done":
		status = &in.Status
	default:
		return nil, apperr.Invalid("status", "to_read, done or empty")
	}
	if in.Limit <= 0 || in.Limit > 50 {
		in.Limit = 20
	}
	rows, err := d.Q.ListSavedLinks(ctx, store.ListSavedLinksParams{WorkspaceID: c.Agent.WorkspaceID, UserID: s.UserID, Status: status, RowLimit: int32(in.Limit)})
	if err != nil {
		return nil, err
	}
	out := make([]savedLink, len(rows))
	for i, r := range rows {
		out[i] = savedLink{Title: r.Title, URL: r.Url, Site: r.Site, Note: r.Note, Video: r.VideoID != "", Status: r.LinkStatus, HasSummary: r.Summary != "", SavedAt: r.CreatedAt.Format("2006-01-02")}
		if in.IncludeSummaries {
			out[i].Summary = r.Summary
		}
	}
	return map[string]any{"links": out}, nil
}
