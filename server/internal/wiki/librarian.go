package wiki

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/links"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

type LibrarianResult struct {
	Proposals  []map[string]any `json:"proposals"`
	Errors     []string         `json:"errors"`
	Orphans    PageTable        `json:"orphans"`
	Unresolved PageTable        `json:"unresolved"`
}

func (s *Service) LibrarianPass(ctx context.Context, agent store.Agent, scopes map[string]agentdef.Mode, query PageQuery) (LibrarianResult, error) {
	ctx = pageview.IncludeAgents(ctx, false)
	query.IncludeAgent = false
	query.Limit = min(20, max(1, query.Limit))
	q := store.New(s.Pool)
	out := LibrarianResult{Proposals: []map[string]any{}, Errors: []string{}}
	targets, err := QueryPages(ctx, q, agent.WorkspaceID, query)
	if err != nil {
		return out, err
	}
	type draft struct {
		Page    store.Artifact
		Version store.ArtifactVersion
		Body    string
	}
	drafts := map[uuid.UUID]*draft{}
	order := []uuid.UUID{}
	for _, target := range targets.Rows {
		ref := Ref{Kind: "artifact", ID: target.ID}
		linked, err := Backlinks(ctx, q, agent.WorkspaceID, ref)
		if err != nil {
			return out, err
		}
		mentions, err := UnlinkedMentions(ctx, q, agent.WorkspaceID, ref, linked)
		if err != nil {
			return out, err
		}
		phrases, link, err := Phrases(ctx, q, agent.WorkspaceID, ref)
		if err != nil {
			return out, err
		}
		for _, mention := range mentions {
			if mention.SourceKind != links.SourceArtifact || mention.SourceID == target.ID {
				continue
			}
			d := drafts[mention.SourceID]
			if d == nil {
				if len(drafts) >= 20 {
					continue
				}
				a, v, err := OpenPage(ctx, q, agent.WorkspaceID, mention.SourceID.String())
				if err != nil {
					continue
				}
				if a.Audience == "agent" {
					continue
				}
				d = &draft{Page: a, Version: v, Body: deref(v.BodyMd)}
				drafts[a.ID] = d
				order = append(order, a.ID)
			}
			for _, phrase := range phrases {
				at := findMention(d.Body, phrase)
				if at < 0 || at+len(phrase) > len(d.Body) {
					continue
				}
				written := d.Body[at : at+len(phrase)]
				if !utf8.ValidString(written) || !strings.EqualFold(written, phrase) {
					continue
				}
				replacement := "[[" + link + "]]"
				if written != link {
					replacement = "[[" + link + "|" + written + "]]"
				}
				d.Body = d.Body[:at] + replacement + d.Body[at+len(phrase):]
				break
			}
		}
	}
	for _, id := range order {
		d := drafts[id]
		if d.Body == deref(d.Version.BodyMd) {
			continue
		}
		result, err := s.AgentWrite(ctx, agent, scopes, Save{Path: *d.Page.Path, Body: d.Body, BaseID: &d.Version.ID, ForceSuggestion: true, Note: "Librarian: link unlinked mentions"})
		if err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("%s: %v", d.Page.Title, err))
			continue
		}
		out.Proposals = append(out.Proposals, map[string]any{"id": d.Page.ID, "path": d.Page.Path, "version_id": result.Version.ID, "state": result.State})
	}
	query.Orphans = true
	query.Unresolved = false
	out.Orphans, err = QueryPages(ctx, q, agent.WorkspaceID, query)
	if err != nil {
		return out, err
	}
	query.Orphans = false
	query.Unresolved = true
	out.Unresolved, err = QueryPages(ctx, q, agent.WorkspaceID, query)
	return out, err
}
