package wiki_test

import "github.com/liamphmurphy/vesper/server/internal/pageview"

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/links"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/wiki"
)

func TestFreeTitlesAndDuplicatePaths(t *testing.T) {
	e := setup(t)
	e.ctx = pageview.Person(e.ctx, e.me.ID)
	title := "A [title] # | ^"
	first, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Audience: "shared", Title: &title, Body: "body", Mode: wiki.ModeCreate})
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Audience: "shared", Title: &title, Body: "body", Mode: wiki.ModeCreate})
	if err != nil {
		t.Fatal(err)
	}
	if first.Note.Title != title || second.Note.Title != title || *first.Note.Path == *second.Note.Path {
		t.Fatalf("titles/paths: %+v %+v", first.Note, second.Note)
	}
	for _, p := range []string{*first.Note.Path, *second.Note.Path} {
		if _, err := artifacts.ValidNotePath(p); err != nil {
			t.Fatal(err)
		}
	}
	changed := "Renamed [display]"
	r, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Audience: "shared", Path: *first.Note.Path, Title: &changed, Body: "body"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Note.Title != changed || *r.Note.Path != "Renamed display" {
		t.Fatalf("title edit did not derive path: %+v", r.Note)
	}
	target, err := links.Resolve(e.ctx, e.q, e.ws, *first.Note.Path, "")
	if err != nil || target.ID == nil || *target.ID != r.Note.ID {
		t.Fatalf("title rename lost old path: %+v %v", target, err)
	}
}

func TestFolderMoveKeepsLinks(t *testing.T) {
	e := setup(t)
	e.ctx = pageview.Person(e.ctx, e.me.ID)
	for n := 0; n < 12; n++ {
		old := fmt.Sprintf("Old/Nested/Page %d", n)
		newPath := fmt.Sprintf("New/Nested/Page %d", n)
		page := e.save(t, old, "body")
		source := e.save(t, fmt.Sprintf("Sources/%d", n), "[["+old+"]]")
		if _, err := e.svc.Rename(e.ctx, e.ws, page.Note.ID, newPath); err != nil {
			t.Fatal(err)
		}
		// Reparse, as rendering and fresh saves do, instead of relying only on
		// a knowledge_links row that already happens to contain the page id.
		target, err := links.Resolve(e.ctx, e.q, e.ws, old, "")
		if err != nil || target.ID == nil || *target.ID != page.Note.ID || target.Path != newPath {
			t.Fatalf("resolve moved %s: %+v %v", old, target, err)
		}
		e.save(t, fmt.Sprintf("Sources/%d", n), "still [["+old+"]]")
		ls := e.linksFrom(t, source.Note.ID)
		if len(ls) != 1 || ls[0].TargetID == nil || *ls[0].TargetID != page.Note.ID {
			t.Fatalf("moved link: %+v", ls)
		}
	}
}

func TestPropertiesAliasesTagsVault(t *testing.T) {
	e := setup(t)
	e.ctx = pageview.Person(e.ctx, e.me.ID)
	props := artifacts.Properties{"text": "before\n---\nafter\n", "number": 12.5, "date": "2026-11-13", "checkbox": true, "list": []string{"one", "two"}, "link": "[[Target]]", "tags": []string{"#Explicit"}, "aliases": []string{"Alternate name"}}
	title := "A display [title]"
	ghost := e.save(t, "Alias source", "[[Alternate name]]")
	r, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Audience: "shared", Path: "Metadata/page", Title: &title, Body: "# heading\n\n#Inline/nested #inline/nested `#code`\n```\n#fenced\n```", Properties: props})
	if err != nil {
		t.Fatal(err)
	}
	target, err := links.Resolve(e.ctx, e.q, e.ws, "Alternate name", "")
	if err != nil || target.ID == nil || *target.ID != r.Note.ID {
		t.Fatalf("alias: %+v %v", target, err)
	}
	ls := e.linksFrom(t, ghost.Note.ID)
	if len(ls) != 1 || ls[0].TargetID == nil || *ls[0].TargetID != r.Note.ID {
		t.Fatalf("alias did not resolve ghost: %+v", ls)
	}
	tags, err := e.q.PageTags(e.ctx, store.PageTagsParams{ArtifactID: r.Note.ID, Viewer: pageview.From(e.ctx).UserID})
	if err != nil || !reflect.DeepEqual(tags, []string{"explicit", "inline/nested"}) {
		t.Fatalf("tags: %v %v", tags, err)
	}
	tag := "INLINE/nested"
	rows, err := e.q.ListNotes(e.ctx, store.ListNotesParams{WorkspaceID: e.ws, Tag: &tag, Viewer: pageview.From(e.ctx).UserID, IncludeAgent: pageview.From(e.ctx).IncludeAgent})
	if err != nil || len(rows) != 1 || rows[0].ID != r.Note.ID {
		t.Fatalf("tag filter: %+v %v", rows, err)
	}
	hits, err := wiki.Search(e.ctx, e.q, e.ws, "explicit", []string{"note"}, nil, 50)
	found := false
	for _, h := range hits {
		if h.ID != nil && *h.ID == r.Note.ID {
			found = true
		}
	}
	if err != nil || !found {
		t.Fatalf("tag search: %+v %v", hits, err)
	}
	var buf bytes.Buffer
	if err := wiki.Export(e.ctx, e.q, e.ws, &buf); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var one bytes.Buffer
	out := zip.NewWriter(&one)
	for _, f := range archive.File {
		if f.Name == "Metadata/page.md" {
			rd, _ := f.Open()
			b, _ := io.ReadAll(rd)
			rd.Close()
			if !strings.Contains(string(b), "checkbox: true") || !strings.Contains(string(b), "aliases:") {
				t.Fatalf("frontmatter: %s", b)
			}
			w, _ := out.Create(f.Name)
			_, _ = w.Write(b)
		}
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	// A different workspace database proves the values came from YAML, rather
	// than merely being inherited from an existing version.
	other := setup(t)
	other.ctx = pageview.Person(other.ctx, other.me.ID)
	res, err := other.svc.Import(other.ctx, other.ws, other.me.ID, one.Bytes())
	if err != nil || res.Created != 1 {
		t.Fatalf("import: %+v %v", res, err)
	}
	imported, err := other.q.GetNoteByPath(other.ctx, store.GetNoteByPathParams{WorkspaceID: other.ws, Path: "Metadata/page", Viewer: pageview.From(other.ctx).UserID})
	if err != nil {
		t.Fatal(err)
	}
	version, err := other.q.GetArtifactVersion(other.ctx, store.GetArtifactVersionParams{ID: *imported.CurrentVersionID, ArtifactID: imported.ID, WorkspaceID: other.ws, Viewer: pageview.From(other.ctx).UserID})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := artifacts.NormalizeProperties(props)
	got, _ := artifacts.NormalizeProperties(artifacts.DecodeProperties(version.Properties))
	if !bytes.Equal(want, got) || imported.Title != title || *version.BodyMd != *r.Version.BodyMd {
		t.Fatalf("roundtrip: %s != %s, title %q", got, want, imported.Title)
	}
	res, err = other.svc.Import(other.ctx, other.ws, other.me.ID, one.Bytes())
	if err != nil || res.Unchanged != 1 {
		t.Fatalf("idempotence: %+v %v", res, err)
	}
}

func TestPropertiesStayWithVersions(t *testing.T) {
	e := setup(t)
	e.ctx = pageview.Person(e.ctx, e.me.ID)
	displayTitle := "A stable display title"
	r, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Audience: "shared", Path: "Versioned", Title: &displayTitle, Body: "body #old", Properties: artifacts.Properties{"aliases": []string{"Old alias"}, "tags": []string{"old"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Rename(e.ctx, e.ws, r.Note.ID, "Versions/Moved"); err != nil {
		t.Fatal(err)
	}
	// Body-only saves retain explicit frontmatter while replacing inline tags.
	r, err = e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Audience: "shared", Path: "Versioned", Body: "edited #new"})
	if err != nil {
		t.Fatal(err)
	}
	tags, _ := e.q.PageTags(e.ctx, store.PageTagsParams{ArtifactID: r.Note.ID, Viewer: pageview.From(e.ctx).UserID})
	if !reflect.DeepEqual(tags, []string{"new", "old"}) {
		t.Fatal(tags)
	}
	suggested, err := e.svc.AgentWrite(e.ctx, e.atlas, map[string]agentdef.Mode{}, wiki.Save{Path: "Versioned", Body: "suggestion #pending", Properties: artifacts.Properties{"aliases": []string{"Pending alias"}, "tags": []string{"pending"}}})
	if err != nil {
		t.Fatal(err)
	}
	pending, _ := links.Resolve(e.ctx, e.q, e.ws, "Pending alias", "")
	if pending.Kind != links.KindUnresolved {
		t.Fatal("suggested alias published")
	}
	tags, _ = e.q.PageTags(e.ctx, store.PageTagsParams{ArtifactID: r.Note.ID, Viewer: pageview.From(e.ctx).UserID})
	if !reflect.DeepEqual(tags, []string{"new", "old"}) {
		t.Fatal(tags)
	}
	if _, err := e.svc.DecideSuggestion(e.ctx, e.ws, e.me.ID, suggested.Version.ID, true); err != nil {
		t.Fatal(err)
	}
	pending, _ = links.Resolve(e.ctx, e.q, e.ws, "Pending alias", "")
	if pending.ID == nil || *pending.ID != r.Note.ID {
		t.Fatal("accepted alias missing")
	}
	moved, err := e.q.FindNoteByAlias(e.ctx, store.FindNoteByAliasParams{WorkspaceID: e.ws, Path: "Versioned", Viewer: pageview.From(e.ctx).UserID})
	if err != nil || moved.ID != r.Note.ID {
		t.Fatalf("moved alias lost during property replacement: %+v %v", moved, err)
	}
	old, _ := links.Resolve(e.ctx, e.q, e.ws, "Old alias", "")
	if old.Kind != links.KindUnresolved {
		t.Fatal("removed alias still resolves")
	}
	tags, _ = e.q.PageTags(e.ctx, store.PageTagsParams{ArtifactID: r.Note.ID, Viewer: pageview.From(e.ctx).UserID})
	if !reflect.DeepEqual(tags, []string{"pending"}) {
		t.Fatal(tags)
	}
	if _, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Audience: "shared", Path: "Versioned", Body: "stale", BaseID: &r.Version.ID, Properties: artifacts.Properties{"tags": []string{"stale"}}}); err == nil {
		t.Fatal("stale metadata accepted")
	}
}

func TestDerivedPathNormalizesBeforeCheckingUniqueness(t *testing.T) {
	e := setup(t)
	e.ctx = pageview.Person(e.ctx, e.me.ID)
	e.save(t, "Suffix", "original")
	for _, title := range []string{"Suffix.md", "Suffix.md.md"} {
		r, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Audience: "shared", Title: &title, Body: "new", Mode: wiki.ModeCreate})
		if err != nil {
			t.Fatal(err)
		}
		if r.Note.Title != title || *r.Note.Path == "Suffix" {
			t.Fatalf("derived path replaced existing: %+v", r.Note)
		}
	}
}

func TestConcurrentDuplicateTitles(t *testing.T) {
	e := setup(t)
	e.ctx = pageview.Person(e.ctx, e.me.ID)
	title := "Concurrent [title]"
	type saved struct {
		result wiki.Result
		err    error
	}
	out := make(chan saved, 8)
	for n := 0; n < 8; n++ {
		go func() {
			r, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Audience: "shared", Title: &title, Body: "body", Mode: wiki.ModeCreate})
			out <- saved{r, err}
		}()
	}
	paths := map[string]bool{}
	for n := 0; n < 8; n++ {
		r := <-out
		if r.err != nil {
			t.Fatal(r.err)
		}
		if paths[*r.result.Note.Path] {
			t.Fatalf("duplicate path %s", *r.result.Note.Path)
		}
		paths[*r.result.Note.Path] = true
	}
}

func TestImportNativeYAMLAndMalformedRollback(t *testing.T) {
	e := setup(t)
	e.ctx = pageview.Person(e.ctx, e.me.ID)
	vault := func(files map[string]string) []byte {
		var buf bytes.Buffer
		z := zip.NewWriter(&buf)
		for name, body := range files {
			w, err := z.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(w, body); err != nil {
				t.Fatal(err)
			}
		}
		if err := z.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	data := vault(map[string]string{"Yaml.md": "---\ntags: [YAML]\naliases: [From YAML]\ndate: 2026-11-13\nnumber: 2\ncheckbox: true\nlist: [one, two]\nlink: '[[Target]]'\nunknown:\n  nested: value\ntext: |\n  before\n  ---\n  after\n---\n\nbody"})
	res, err := e.svc.Import(e.ctx, e.ws, e.me.ID, data)
	if err != nil || res.Created != 1 {
		t.Fatalf("native YAML: %+v %v", res, err)
	}
	a, err := e.q.GetNoteByPath(e.ctx, store.GetNoteByPathParams{WorkspaceID: e.ws, Path: "Yaml", Viewer: pageview.From(e.ctx).UserID})
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.q.GetArtifactVersion(e.ctx, store.GetArtifactVersionParams{ID: *a.CurrentVersionID, ArtifactID: a.ID, WorkspaceID: e.ws, Viewer: pageview.From(e.ctx).UserID})
	if err != nil {
		t.Fatal(err)
	}
	p := artifacts.DecodeProperties(v.Properties)
	if p["text"] != "before\n---\nafter\n" || p["date"] != "2026-11-13" || p["number"] != float64(2) || p["checkbox"] != true || !strings.Contains(p["unknown"].(string), "nested: value") {
		t.Fatalf("native YAML properties: %+v", p)
	}
	var buf bytes.Buffer
	if err := wiki.Export(e.ctx, e.q, e.ws, &buf); err != nil {
		t.Fatal(err)
	}
	res, err = e.svc.Import(e.ctx, e.ws, e.me.ID, buf.Bytes())
	if err != nil || res.Updated != 0 || res.Created != 0 {
		t.Fatalf("native roundtrip: %+v %v", res, err)
	}
	bad := vault(map[string]string{"Bad.md": "---\ntags: [unclosed\n---\nbody"})
	if _, err := e.svc.Import(e.ctx, e.ws, e.me.ID, bad); !invalid(err) {
		t.Fatalf("malformed YAML accepted: %v", err)
	}
}

func TestExplicitCreationPreservesPropertyAliases(t *testing.T) {
	e := setup(t)
	e.ctx = pageview.Person(e.ctx, e.me.ID)
	r, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Audience: "shared", Path: "Original", Body: "body", Aliases: &[]string{"Alternate"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Audience: "shared", Path: "alternate", Body: "steal", Mode: wiki.ModeCreate}); !invalid(err) {
		t.Fatalf("explicit create stole property alias: %v", err)
	}
	if _, err := e.svc.AgentWrite(e.ctx, e.atlas, map[string]agentdef.Mode{}, wiki.Save{Path: "Alternate", Body: "suggestion", Mode: wiki.ModeCreate}); !invalid(err) {
		t.Fatalf("suggested create stole property alias: %v", err)
	}
	target, err := links.Resolve(e.ctx, e.q, e.ws, "Alternate", "")
	if err != nil || target.ID == nil || *target.ID != r.Note.ID {
		t.Fatalf("alias ownership: %+v %v", target, err)
	}
	if _, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Audience: "shared", Path: "Original", Body: "body-only edit"}); err != nil {
		t.Fatalf("inherited alias broke body edit: %v", err)
	}
}

func TestConcurrentCreationAndAliasPublication(t *testing.T) {
	e := setup(t)
	e.ctx = pageview.Person(e.ctx, e.me.ID)
	for n := 0; n < 6; n++ {
		original := fmt.Sprintf("Original %d", n)
		alias := fmt.Sprintf("Racing alias %d", n)
		r := e.save(t, original, "body")
		start := make(chan struct{})
		errs := make(chan error, 2)
		go func() {
			<-start
			_, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Audience: "shared", Path: original, Body: "body", Aliases: &[]string{alias}})
			errs <- err
		}()
		go func() {
			<-start
			_, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Audience: "shared", Path: alias, Body: "created", Mode: wiki.ModeCreate})
			errs <- err
		}()
		close(start)
		success := 0
		for i := 0; i < 2; i++ {
			err := <-errs
			if err == nil {
				success++
			} else if !invalid(err) {
				t.Fatal(err)
			}
		}
		if success != 1 {
			t.Fatalf("create and alias publication: %d successes", success)
		}
		if _, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Audience: "shared", Path: original, Body: "still editable"}); err != nil {
			t.Fatal(err)
		}
		v, err := e.q.GetArtifact(e.ctx, store.GetArtifactParams{ID: r.Note.ID, WorkspaceID: e.ws, Viewer: pageview.From(e.ctx).UserID})
		if err != nil {
			t.Fatal(err)
		}
		current, err := e.q.GetArtifactVersion(e.ctx, store.GetArtifactVersionParams{ID: *v.CurrentVersionID, ArtifactID: v.ID, WorkspaceID: e.ws, Viewer: pageview.From(e.ctx).UserID})
		if err != nil {
			t.Fatal(err)
		}
		aliases, _ := artifacts.StringList(artifacts.DecodeProperties(current.Properties)["aliases"])
		if len(aliases) > 0 {
			target, err := links.Resolve(e.ctx, e.q, e.ws, alias, "")
			if err != nil || target.ID == nil || *target.ID != r.Note.ID {
				t.Fatalf("publication lost race ownership: %+v %v", target, err)
			}
		}
	}
}

func TestTitleRenamesLastSegmentWithSuffix(t *testing.T) {
	e := setup(t)
	e.ctx = pageview.Person(e.ctx, e.me.ID)
	e.save(t, "Ops/New", "occupied")
	old := e.save(t, "Ops/Old", "body")
	title := "New"
	renamed, err := e.svc.UpdateAsUser(e.ctx, e.ws, e.me.ID, old.Note.ID, wiki.Save{Title: &title}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Note.Title != "New" || *renamed.Note.Path != "Ops/New 2" {
		t.Fatalf("rename path: %+v", renamed.Note)
	}
	target, err := links.Resolve(e.ctx, e.q, e.ws, "Ops/Old", "")
	if err != nil || target.ID == nil || *target.ID != old.Note.ID {
		t.Fatalf("old path: %+v %v", target, err)
	}
	// A generic artifact title edit must follow the same ADR behavior.
	title = "Third [name]"
	a, err := e.svc.Artifacts.UpdateMeta(e.ctx, e.ws, old.Note.ID, artifacts.MetaPatch{Title: &title})
	if err != nil {
		t.Fatal(err)
	}
	if a.Title != title || *a.Path != "Ops/Third name" {
		t.Fatalf("generic title rename: %+v", a)
	}
	for _, bad := range []string{"", strings.Repeat("界", 201)} {
		if _, err := e.svc.UpdateAsUser(e.ctx, e.ws, e.me.ID, a.ID, wiki.Save{Title: &bad}, nil, nil); !invalid(err) {
			t.Fatalf("invalid title length accepted: %v", err)
		}
		if _, err := e.svc.SaveAsUser(e.ctx, e.ws, e.me.ID, wiki.Save{Audience: "shared", Title: &bad, Body: "body", Mode: wiki.ModeCreate}); !invalid(err) {
			t.Fatalf("invalid create title length accepted: %v", err)
		}
	}
	valid := strings.Repeat("界", 200)
	if _, err := e.svc.UpdateAsUser(e.ctx, e.ws, e.me.ID, a.ID, wiki.Save{Title: &valid}, nil, nil); err != nil {
		t.Fatalf("200-rune title rejected: %v", err)
	}
}
