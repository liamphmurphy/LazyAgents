package wiki

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

var TemplateSet = map[string]string{
	"Daily":          "# {{date}}\n\n## Notes\n\n## Tasks\n\n- [ ] \n",
	"Permanent note": "# {{title}}\n\n## Idea\n\n## Connections\n\n## Sources\n",
	"NPC":            "# {{title}}\n\n## Role\n\n## Motivation\n\n## Connections\n",
	"Location":       "# {{title}}\n\n## Description\n\n## People\n\n## Secrets\n",
	"Session":        "# {{title}}\n\nDate: {{date}}\n\n## Recap\n\n## Scenes\n\n## Follow-up\n",
	"Encounter":      "# {{title}}\n\n## Setup\n\n## Creatures\n\n## Terrain\n\n## Rewards\n",
}
var templateVar = regexp.MustCompile(`\{\{([\w.-]+)\}\}`)

func RenderTemplate(body, title, date string, props artifacts.Properties) string {
	return templateVar.ReplaceAllStringFunc(body, func(v string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(v, "{{"), "}}")
		switch key {
		case "title":
			return title
		case "date":
			return date
		}
		key = strings.TrimPrefix(key, "properties.")
		if value, ok := props[key]; ok {
			return fmt.Sprint(value)
		}
		return v
	})
}

func (s *Service) DailyPage(ctx context.Context, ws, user uuid.UUID, date, folder, template string) (Result, error) {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return Result{}, apperr.Invalid("date", "use YYYY-MM-DD")
	}
	inferFolder := folder == ""
	if folder == "" {
		folder = "Daily"
	}
	p, err := artifacts.ValidNotePath(folder + "/" + date)
	if err != nil {
		return Result{}, err
	}
	if template == "" {
		template = TemplateSet["Daily"]
	}
	ctx = pageview.Person(ctx, user)
	var out Result
	err = s.tx(ctx, ws, func(q *store.Queries) error {
		if inferFolder {
			found, err := QueryPages(ctx, q, ws, PageQuery{Audience: "private", Properties: artifacts.Properties{"type": "daily", "date": date}, Limit: 1})
			if err != nil {
				return err
			}
			if len(found.Rows) > 0 {
				a, v, err := OpenPage(ctx, q, ws, found.Rows[0].ID.String())
				out = Result{Note: a, Version: v, State: artifacts.Published}
				return err
			}
		}
		a, err := q.GetNoteByPath(ctx, store.GetNoteByPathParams{WorkspaceID: ws, Path: p, Viewer: &user})
		if err == nil {
			v, err := latestVersion(ctx, q, a)
			out = Result{Note: a, Version: v, State: artifacts.Published}
			return err
		}
		if err != pgx.ErrNoRows {
			return err
		}
		out, err = s.write(ctx, q, ws, artifacts.Actor{Type: "user", ID: user}, Save{Path: p, Body: RenderTemplate(template, date, date, nil), Mode: ModeCreate, Properties: artifacts.Properties{"type": "daily", "date": date}}, artifacts.Published)
		return err
	})
	return out, err
}

// FileCapture performs the merge and source removal atomically. A source with
// suggestions is retained so a person's review history cannot disappear.
func (s *Service) FileCapture(ctx context.Context, ws, user, id uuid.UUID, folder string, merge *uuid.UUID) (Result, error) {
	if merge == nil {
		p, _, err := OpenPage(pageview.Person(ctx, user), store.New(s.Pool), ws, id.String())
		if err != nil {
			return Result{}, err
		}
		a, err := s.Rename(pageview.Person(ctx, user), ws, id, strings.Trim(folder, "/")+"/"+linksTitle(*p.Path))
		return Result{Note: a}, err
	}
	if *merge == id {
		return Result{}, apperr.Invalid("merge", "cannot merge a page into itself")
	}
	ctx = pageview.Person(ctx, user)
	var out Result
	err := s.tx(ctx, ws, func(q *store.Queries) error {
		source, sv, err := OpenPage(ctx, q, ws, id.String())
		if err != nil {
			return err
		}
		target, tv, err := OpenPage(ctx, q, ws, merge.String())
		if err != nil {
			return err
		}
		if source.Audience != target.Audience {
			return apperr.Invalid("merge", "pages must have the same audience; share explicitly before merging")
		}
		reviews, err := q.ListPageReviews(ctx, store.ListPageReviewsParams{ID: id, WorkspaceID: ws, Viewer: &user})
		if err != nil {
			return err
		}
		if len(reviews) > 0 {
			return apperr.Conflict("review pending changes before merging this capture")
		}
		out, err = s.write(ctx, q, ws, artifacts.Actor{Type: "user", ID: user}, Save{PageID: merge, Path: *target.Path, Body: *tv.BodyMd + "\n\n" + *sv.BodyMd, BaseID: &tv.ID, Note: "Merged capture: " + source.Title}, artifacts.Published)
		if err != nil {
			return err
		}
		return q.DeleteArtifact(ctx, store.DeleteArtifactParams{ID: id, WorkspaceID: ws})
	})
	return out, err
}
func linksTitle(p string) string {
	_, last, ok := strings.Cut(p, "/")
	if !ok {
		return p
	}
	return linksTitle(last)
}
