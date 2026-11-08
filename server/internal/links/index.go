package links

import (
	"context"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Source kinds stored in knowledge_links.source_kind.
const (
	SourceArtifact = "artifact"
	SourceTicket   = "ticket"
	SourceComment  = "comment"
)

// Source is where links were written.
type Source struct {
	Kind      string
	ID        uuid.UUID
	VersionID *uuid.UUID // artifacts
	TicketID  *uuid.UUID // tickets and comments
}

// Index replaces a source's stored links with those in md. Call it in the
// same transaction as the save, with the content that is now current.
func Index(ctx context.Context, q *store.Queries, ws uuid.UUID, src Source, md string) error {
	if err := q.DeleteLinksFromSource(ctx, store.DeleteLinksFromSourceParams{SourceKind: src.Kind, SourceID: src.ID}); err != nil {
		return err
	}
	for i, r := range Parse(md) {
		if r.Target == "" {
			continue // [[#Heading]]: a link within the same page
		}
		t, err := Resolve(ctx, q, ws, r.Target, r.Anchor)
		if err != nil {
			return err
		}
		anchor := t.Anchor
		if t.Kind == KindUnresolved && r.Anchor != "" {
			anchor = Slug(r.Anchor) // kept so the link becomes a section link once resolved
		}
		if err := q.InsertLink(ctx, store.InsertLinkParams{
			WorkspaceID: ws, SourceKind: src.Kind, SourceID: src.ID, SourceVersionID: src.VersionID, SourceTicketID: src.TicketID,
			TargetKind: t.Kind, TargetID: t.ID, TargetAnchor: anchor, TargetText: r.Target, RawText: r.Raw,
			Snippet: r.Snippet, Position: int32(i),
		}); err != nil {
			return err
		}
	}
	return nil
}

// ResolvePending re-resolves every ghost link in the workspace. Call it after
// creating or renaming anything a link can name (notes, artifacts, tickets,
// epics, agents). Workspaces are laptop-sized, so this is cheap.
func ResolvePending(ctx context.Context, q *store.Queries, ws uuid.UUID) error {
	texts, err := q.UnresolvedTargets(ctx, ws)
	if err != nil {
		return err
	}
	for _, text := range texts {
		t, err := Resolve(ctx, q, ws, text, "")
		if err != nil {
			return err
		}
		if t.Kind == KindUnresolved {
			continue
		}
		if _, err := q.ResolveLinks(ctx, store.ResolveLinksParams{WorkspaceID: ws, TargetKind: t.Kind, TargetID: t.ID, TargetText: text}); err != nil {
			return err
		}
	}
	return nil
}
