package wiki

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/links"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

type vaultContent struct {
	Body             string
	Properties       artifacts.Properties
	Raw              json.RawMessage
	Aliases          []string
	Created, Updated time.Time
}

func vaultNoteContent(f *zip.File, e VaultEntry, entries []VaultEntry, assets map[string]string) (vaultContent, error) {
	body, props, err := readZipNote(f)
	if err != nil {
		return vaultContent{}, err
	}
	c := vaultContent{Created: parseVaultTime(props["created"], f.Modified), Updated: parseVaultTime(props["updated"], f.Modified)}
	for k := range props {
		if reservedFrontmatter(k) || k == "audience" {
			delete(props, k)
		}
	}
	if e.Daily {
		props["type"] = "daily"
		props["date"] = strings.TrimSuffix(path.Base(e.Source), path.Ext(e.Source))
	}
	if e.OriginalTitle != "" {
		props["obsidian_title"] = e.OriginalTitle
	}
	safe := e.IndexedAliases
	if safe == nil {
		safe = []string{}
		for _, alias := range e.Aliases {
			if _, err := artifacts.ValidNotePath(alias); err == nil {
				safe = append(safe, alias)
			}
		}
	}
	if _, ok := props["aliases"]; ok {
		props["aliases"] = safe
	}
	if len(safe) != len(e.Aliases) {
		props["vesper_unindexed_aliases"] = e.Aliases
	}
	c.Body = rewriteVaultLinks(body, e.Source, entries, assets)
	c.Properties = props
	c.Aliases = safe
	c.Raw, err = artifacts.NormalizeProperties(props)
	return c, err
}

// Final paths are chosen together before rewriting any bodies. Keep-both
// copies propagate through incoming links, preserving the imported graph while
// leaving the originals untouched. An unchanged re-import creates no copies.
func (s *Service) prepareVaultPlan(ctx context.Context, job store.KnowledgeImport, plan *VaultPlan, files map[string]*zip.File, assets map[string]string) error {
	return s.tx(ctx, job.WorkspaceID, func(q *store.Queries) error {
		if _, err := q.LockKnowledgeImportLease(ctx, store.LockKnowledgeImportLeaseParams{ID: job.ID, LeaseID: job.LeaseID}); err != nil {
			return err
		}
		if err := indexVaultAliases(ctx, q, job, plan); err != nil {
			return err
		}
		if job.Policy != "keep_both" {
			return nil
		}
		bySource := map[string]int{}
		parents := map[string][]string{}
		queue := []string{}
		copies := map[string]bool{}
		resolve := vaultResolver(plan.Entries)
		for i, e := range plan.Entries {
			if e.Kind != "note" || e.Error != "" {
				continue
			}
			bySource[e.Source] = i
			content, err := vaultNoteContent(files[e.Source], e, plan.Entries, assets)
			if err != nil {
				plan.Entries[i].Error = err.Error()
				continue
			}
			refs := links.Parse(content.Body)
			// Read raw references too: normalized paths resolve via the plan, while
			// relative Markdown file links still need their original vault location.
			raw, _, err := readZipNote(files[e.Source])
			if err != nil {
				return err
			}
			refs = append(refs, links.Parse(raw)...)
			for _, m := range markdownFileLink.FindAllStringSubmatch(raw, -1) {
				target, _, _ := strings.Cut(m[3], "#")
				refs = append(refs, links.Ref{Target: target})
			}
			for _, ref := range refs {
				if target, ok := resolve(e.Source, ref.Target); ok && target.Kind == "note" {
					parents[target.Source] = append(parents[target.Source], e.Source)
				}
			}
			if e.ExistingID == nil {
				continue
			}
			a, v, err := OpenPage(ctx, q, job.WorkspaceID, e.ExistingID.String())
			if err != nil {
				return err
			}
			old, _ := artifacts.NormalizeProperties(artifacts.DecodeProperties(v.Properties))
			if v.BodyMd == nil || *v.BodyMd != content.Body || !bytes.Equal(old, content.Raw) || a.Title != e.Title {
				copies[e.Source] = true
				queue = append(queue, e.Source)
			}
		}
		for len(queue) > 0 {
			source := queue[0]
			queue = queue[1:]
			for _, parent := range parents[source] {
				i, ok := bySource[parent]
				if ok && plan.Entries[i].ExistingID != nil && !copies[parent] {
					copies[parent] = true
					queue = append(queue, parent)
				}
			}
		}
		allocated := map[string]bool{}
		for _, e := range plan.Entries {
			allocated[strings.ToLower(e.Path)] = true
		}
		for i := range plan.Entries {
			e := &plan.Entries[i]
			if !copies[e.Source] {
				continue
			}
			folder := path.Dir(importPath(e.Path+".md", ""))
			if folder == "." {
				folder = ""
			}
			label := e.Title + " imported " + job.ID.String()[:8]
			p, err := artifacts.DerivedNotePathInFolder(ctx, q, job.WorkspaceID, label, folder)
			if err != nil {
				return err
			}
			for n := 2; allocated[strings.ToLower(p)]; n++ {
				p, err = artifacts.DerivedNotePathInFolder(ctx, q, job.WorkspaceID, strconv.Itoa(n)+" "+label, folder)
				if err != nil {
					return err
				}
			}
			allocated[strings.ToLower(p)] = true
			e.Path = p
			e.Duplicate = true
			e.Renamed = true
		}

		return indexVaultAliases(ctx, q, job, plan)
	})
}
func indexVaultAliases(ctx context.Context, q *store.Queries, job store.KnowledgeImport, plan *VaultPlan) error {
	claimed := map[string]string{}
	for i := range plan.Entries {
		e := &plan.Entries[i]
		if e.Kind != "note" || e.Error != "" {
			continue
		}
		e.IndexedAliases = []string{}
		for _, alias := range e.Aliases {
			if _, err := artifacts.ValidNotePath(alias); err != nil {
				continue
			}
			other, err := q.GetNoteByPath(ctx, store.GetNoteByPathParams{WorkspaceID: job.WorkspaceID, Path: alias, Viewer: &job.UserID})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if err == nil && (e.Duplicate || e.ExistingID == nil || other.ID != *e.ExistingID) {
				continue
			}
			other, err = q.FindNoteByAlias(ctx, store.FindNoteByAliasParams{WorkspaceID: job.WorkspaceID, Path: alias, Viewer: &job.UserID})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if err == nil && (e.Duplicate || e.ExistingID == nil || other.ID != *e.ExistingID) {
				continue
			}
			key := strings.ToLower(alias)
			if source, ok := claimed[key]; ok && source != e.Source {
				continue
			}
			claimed[key] = e.Source
			e.IndexedAliases = append(e.IndexedAliases, alias)
		}
	}
	return nil
}
