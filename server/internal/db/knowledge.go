package db

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/links"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// SQL preserves the data. This resumable backfill uses the normal Markdown
// parser to populate outlines, links, tags and review hunks after copying.
func backfillKnowledge(ctx context.Context, pool *pgxpool.Pool) error {
	q := store.New(pool)
	pages, err := q.UnindexedDocPages(ctx)
	if err != nil {
		return err
	}
	for _, page := range pages {
		err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			q := store.New(tx)
			ctx := ctx
			if page.ChatSessionID != nil {
				chat, err := q.GetChatSession(ctx, store.GetChatSessionParams{ID: *page.ChatSessionID, WorkspaceID: page.WorkspaceID})
				if err != nil {
					return err
				}
				ctx = pageview.Person(ctx, chat.CreatedBy)
			}
			if err := q.LockNotePaths(ctx, page.WorkspaceID); err != nil {
				return err
			}
			a, err := q.LockArtifact(ctx, store.LockArtifactParams{ID: page.ID, WorkspaceID: page.WorkspaceID, Viewer: pageview.From(ctx).UserID})
			if err != nil {
				return err
			}
			versions, err := q.ListArtifactVersions(ctx, store.ListArtifactVersionsParams{ArtifactID: a.ID, Viewer: pageview.From(ctx).UserID})
			if err != nil {
				return err
			}
			for _, ver := range versions {
				v, err := q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: ver.ID, ArtifactID: a.ID, WorkspaceID: a.WorkspaceID, Viewer: pageview.From(ctx).UserID})
				if err != nil {
					return err
				}
				if v.SourceCanvasVersionID == nil || v.BodyMd == nil {
					continue
				}
				sections, err := q.ListSectionsWithCitations(ctx, store.ListSectionsWithCitationsParams{ArtifactID: a.ID, VersionID: v.ID, Viewer: pageview.From(ctx).UserID})
				if err != nil {
					return err
				}
				if len(sections) == 0 {
					for i, sec := range artifacts.Outline(*v.BodyMd) {
						if err := q.InsertSection(ctx, store.InsertSectionParams{VersionID: v.ID, Position: int32(i + 1), Level: int32(sec.Level), Anchor: sec.Anchor, Heading: sec.Heading}); err != nil {
							return err
						}
					}
				}
			}
			if a.CurrentVersionID != nil {
				v, err := q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: *a.CurrentVersionID, ArtifactID: a.ID, WorkspaceID: a.WorkspaceID, Viewer: pageview.From(ctx).UserID})
				if err != nil {
					return err
				}
				if err = artifacts.Publish(ctx, q, a, v); err != nil {
					return err
				}
			}
			reviews, err := q.ListPageReviews(ctx, store.ListPageReviewsParams{ID: a.ID, WorkspaceID: a.WorkspaceID, Viewer: pageview.From(ctx).UserID})
			if err != nil {
				return err
			}
			for _, v := range reviews {
				if string(v.Changes) != "[]" || v.BodyMd == nil {
					continue
				}
				before := ""
				props := artifacts.Properties{}
				for _, ver := range versions {
					if ver.Seq >= v.Seq {
						continue
					}
					prev, err := q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: ver.ID, ArtifactID: a.ID, WorkspaceID: a.WorkspaceID, Viewer: pageview.From(ctx).UserID})
					if err != nil {
						return err
					}
					if prev.BodyMd != nil {
						before = *prev.BodyMd
					}
					props = artifacts.DecodeProperties(prev.Properties)
					break
				}
				if v.State == artifacts.Suggested && a.CurrentVersionID != nil {
					head, err := q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: *a.CurrentVersionID, ArtifactID: a.ID, WorkspaceID: a.WorkspaceID, Viewer: pageview.From(ctx).UserID})
					if err != nil {
						return err
					}
					if head.BodyMd != nil {
						before = *head.BodyMd
					}
					props = artifacts.DecodeProperties(head.Properties)
				}
				changes, err := json.Marshal(artifacts.PageChanges(before, *v.BodyMd, props, artifacts.DecodeProperties(v.Properties), v.Note))
				if err != nil {
					return err
				}
				pending := "pending"
				if _, err = q.SetPageChanges(ctx, store.SetPageChangesParams{ID: v.ID, WorkspaceID: a.WorkspaceID, Viewer: pageview.From(ctx).UserID, Changes: changes, ReviewState: &pending}); err != nil {
					return err
				}
			}
			if err = links.ResolvePending(ctx, q, a.WorkspaceID); err != nil {
				return err
			}
			if err = q.RestoreMigratedPageTimes(ctx, store.RestoreMigratedPageTimesParams{ID: a.ID, Viewer: pageview.From(ctx).UserID}); err != nil {
				return err
			}
			return q.MarkDocPageIndexed(ctx, a.ID)
		})
		if err != nil {
			return err
		}
	}
	return nil
}
