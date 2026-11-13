package wiki_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/wiki"
)

func TestPatternPathsAndHeader(t *testing.T) {
	if p, err := wiki.PatternPath("go", "  Wrap errors   with context "); err != nil || p != "Patterns/Go/Wrap errors with context" {
		t.Fatalf("PatternPath = %q, %v", p, err)
	}
	if _, err := wiki.PatternPath("Cooking", "Boil water"); !invalid(err) {
		t.Fatalf("unknown topic: %v", err)
	}
	for _, bad := range []string{"", "a/b", "Has [[link]]", "Proposed", strings.Repeat("x", 81)} {
		if _, err := wiki.ValidPatternTitle(bad); !invalid(err) {
			t.Errorf("title %q accepted", bad)
		}
	}
	if p, err := wiki.ProposedPath("Wrap errors"); err != nil || p != "Patterns/Proposed/Wrap errors" {
		t.Fatalf("ProposedPath = %q, %v", p, err)
	}
	for path, want := range map[string][3]any{
		"Patterns/Go/Wrap errors":  {"Go", "Wrap errors", false},
		"patterns/proposed/Foo":    {"", "Foo", true},
		"Patterns/Cooking/Foo":     {},
		"Patterns/Go":              {},
		"agents/atlas/Patterns/Go": {},
	} {
		topic, title, proposed, ok := wiki.ParsePatternPath(path)
		if want == [3]any{} {
			if ok {
				t.Errorf("ParsePatternPath(%q) ok", path)
			}
			continue
		}
		if !ok || topic != want[0] || title != want[1] || proposed != want[2] {
			t.Errorf("ParsePatternPath(%q) = %q %q %v %v", path, topic, title, proposed, ok)
		}
	}

	tags, err := wiki.NormalizeTags([]string{"Errors", " #context", "errors", "ctx"})
	if err != nil || !reflect.DeepEqual(tags, []string{"errors", "context", "ctx"}) {
		t.Fatalf("NormalizeTags = %v, %v", tags, err)
	}
	if _, err := wiki.NormalizeTags([]string{"one"}); !invalid(err) {
		t.Fatal("one tag accepted")
	}
	if _, err := wiki.NormalizeTags([]string{"a", "b", "c", "d", "e", "f"}); !invalid(err) {
		t.Fatal("six tags accepted")
	}

	body := wiki.ComposeProposal("Go", tags, "## When to use\n\n- Any error crossing a package boundary.\n\n## Rule\n\nWrap it.\n\n## Related\n", "PAY-214")
	topic, ptags, rest := wiki.ParseHeader(body)
	if topic != "Go" || !reflect.DeepEqual(ptags, tags) || !strings.HasPrefix(rest, "## When to use") {
		t.Fatalf("ParseHeader = %q %v %q", topic, ptags, rest)
	}
	if !strings.HasPrefix(body, "Topic: Go\nTags: errors, context, ctx\n\n") || !strings.Contains(body, "## Related\n- Source: [[PAY-214]]") {
		t.Fatalf("body = %q", body)
	}
	if got := wiki.Summary(body); got != "Any error crossing a package boundary." {
		t.Fatalf("Summary = %q", got)
	}
	// A header the agent included itself isn't doubled; a missing Related section is added.
	again := wiki.ComposeProposal("Go", tags, body, "PAY-1")
	if strings.Count(again, "Topic:") != 1 || !strings.Contains(again, "[[PAY-1]]") {
		t.Fatalf("again = %q", again)
	}
	if !strings.Contains(wiki.ComposeProposal("Go", tags, "## Rule\n\nx", "PAY-2"), "## Related\n\n- Source: [[PAY-2]]") {
		t.Fatal("Related section not added")
	}
}

func TestFindPatterns(t *testing.T) {
	e := setup(t)
	mk := func(path, topic, tags, when string) {
		e.save(t, path, "Topic: "+topic+"\nTags: "+tags+"\n\n## When to use\n\n"+when+"\n\n## Rule\n\nDo the thing.\n")
	}
	mk("Patterns/Go/Wrap errors with context", "Go", "errors, context, ctx, wrapping", "Returning an error across a function boundary.")
	mk("Patterns/Postgres/Use pgx for database access", "Postgres", "pgx, postgres, database", "Any code talking to Postgres.")
	mk("Patterns/Testing/Use throwaway databases in tests", "Testing", "testing, database, integration", "Integration tests that need real SQL.")
	mk("Patterns/React/Keep server state out of components", "React", "react, state, query", "Fetching data in the UI.")
	// Never returned: proposals, notes outside Patterns/, agent memory.
	mk("Patterns/Proposed/Wrap errors in handlers", "Go", "errors, handlers", "Proposal text about pgx and errors.")
	e.save(t, "Architecture/ADR - pgx everywhere", "We use pgx and wrap errors with context. Tags: pgx")
	e.save(t, "agents/atlas/pgx notes", "Topic: Go\nTags: pgx, errors\n\npgx wrap errors")

	titles := func(q, topic string) []string {
		t.Helper()
		ps, err := wiki.FindPatterns(e.ctx, e.q, e.ws, q, topic, 0)
		if err != nil {
			t.Fatalf("FindPatterns(%q): %v", q, err)
		}
		var out []string
		for _, p := range ps {
			if !strings.HasPrefix(p.Path, "Patterns/") || strings.HasPrefix(p.Path, "Patterns/Proposed/") {
				t.Fatalf("%q returned %s", q, p.Path)
			}
			out = append(out, p.Title)
		}
		return out
	}
	first := func(q, want string) {
		t.Helper()
		if got := titles(q, ""); len(got) == 0 || got[0] != want {
			t.Errorf("FindPatterns(%q) = %v, want %q first", q, got, want)
		}
	}
	first("pgx", "Use pgx for database access")
	first("ctx", "Wrap errors with context")
	first("eror wrap", "Wrap errors with context")
	first("wrap err", "Wrap errors with context")
	first("pgs database", "Use pgx for database access")
	first("throwaway", "Use throwaway databases in tests")
	first("databse tests", "Use throwaway databases in tests")
	first("reac", "Keep server state out of components")
	if got := titles("kubernetes operators", ""); len(got) != 0 {
		t.Errorf("unrelated query matched %v", got)
	}

	// Browse: topic then title, with the parsed header and summary.
	all, err := wiki.FindPatterns(e.ctx, e.q, e.ws, "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, p := range all {
		order = append(order, p.Topic+"/"+p.Title)
	}
	want := []string{"Go/Wrap errors with context", "Postgres/Use pgx for database access", "React/Keep server state out of components", "Testing/Use throwaway databases in tests"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("browse = %v", order)
	}
	g := all[0]
	if !reflect.DeepEqual(g.Tags, []string{"errors", "context", "ctx", "wrapping"}) || g.Summary != "Returning an error across a function boundary." || g.Path != "Patterns/Go/Wrap errors with context" {
		t.Fatalf("pattern = %+v", g)
	}

	if got := titles("", "react"); !reflect.DeepEqual(got, []string{"Keep server state out of components"}) {
		t.Errorf("topic filter = %v", got)
	}
	if got := titles("database", "Testing"); !reflect.DeepEqual(got, []string{"Use throwaway databases in tests"}) {
		t.Errorf("topic + q = %v", got)
	}
	if _, err := wiki.FindPatterns(e.ctx, e.q, e.ws, "x", "Cooking", 5); !invalid(err) {
		t.Errorf("bad topic: %v", err)
	}
	if ps, _ := wiki.FindPatterns(e.ctx, e.q, e.ws, "", "", 2); len(ps) != 2 {
		t.Errorf("limit = %d", len(ps))
	}
}

func TestPatternProposeAndPublish(t *testing.T) {
	e := setup(t)
	curator := map[string]agentdef.Mode{"wiki:write": agentdef.Allowed}
	propose := func(a store.Agent, title string) (wiki.Result, error) {
		return e.svc.ProposePattern(e.ctx, a, wiki.Proposal{Title: title, Topic: "go", Tags: []string{"errors", "context"},
			Body: "## When to use\n\nAlways.\n\n## Rule\n\nWrap.", SourceTicket: "PAY-214"})
	}
	res, err := propose(e.atlas, "Wrap errors with context")
	if err != nil || res.Note.Path == nil || *res.Note.Path != "Patterns/Proposed/Wrap errors with context" || res.State != "published" {
		t.Fatalf("propose = %+v, %v", res, err)
	}
	if links := e.linksFrom(t, res.Note.ID); len(links) != 1 || links[0].RawText != "[[PAY-214]]" {
		t.Fatalf("links = %+v", links)
	}
	if _, err := propose(e.atlas, "Wrap errors with context"); err != nil { // same agent revises
		t.Fatalf("re-propose: %v", err)
	}
	if _, err := propose(e.echo, "Wrap errors with context"); !invalid(err) {
		t.Fatalf("other agent re-propose: %v", err)
	}
	if _, err := e.svc.ProposePattern(e.ctx, e.atlas, wiki.Proposal{Title: "X pattern", Topic: "Cooking", Tags: []string{"a", "b"}, Body: "x"}); !invalid(err) {
		t.Fatalf("bad topic: %v", err)
	}
	if ps, err := wiki.FindPatterns(e.ctx, e.q, e.ws, "wrap", "", 0); err != nil || len(ps) != 0 {
		t.Fatalf("proposal is searchable: %v %v", ps, err)
	}
	list, err := e.svc.ListProposed(e.ctx, e.ws)
	if err != nil || len(list) != 1 || list[0].Proposer != "@atlas" || list[0].Title != "Wrap errors with context" {
		t.Fatalf("ListProposed = %+v, %v", list, err)
	}

	denied := map[string]agentdef.Mode{"wiki:write": agentdef.RequiresApproval}
	if _, err := e.svc.PublishPattern(e.ctx, e.echo, denied, wiki.PublishInput{ProposedTitle: "Wrap errors with context"}); !invalid(err) {
		t.Fatalf("publish without scope: %v", err)
	}
	if _, err := e.svc.RejectPattern(e.ctx, e.echo, nil, "Wrap errors with context"); !invalid(err) {
		t.Fatalf("reject without scope: %v", err)
	}
	if _, err := e.svc.PublishPattern(e.ctx, e.echo, curator, wiki.PublishInput{ProposedTitle: "Nope"}); !invalid(err) {
		t.Fatalf("publish missing: %v", err)
	}

	pub, err := e.svc.PublishPattern(e.ctx, e.echo, curator, wiki.PublishInput{ProposedTitle: "Wrap errors with context", Topic: "Architecture", Title: "Wrap errors at boundaries"})
	if err != nil || *pub.Note.Path != "Patterns/Architecture/Wrap errors at boundaries" {
		t.Fatalf("publish = %+v, %v", pub, err)
	}
	ps, err := wiki.FindPatterns(e.ctx, e.q, e.ws, "boundaries", "", 0)
	if err != nil || len(ps) != 1 || ps[0].Topic != "Architecture" || !reflect.DeepEqual(ps[0].Tags, []string{"errors", "context"}) {
		t.Fatalf("published pattern = %+v, %v", ps, err)
	}
	if list, _ := e.svc.ListProposed(e.ctx, e.ws); len(list) != 0 {
		t.Fatalf("proposal still listed: %+v", list)
	}
	if _, err := e.q.GetNoteByPath(e.ctx, store.GetNoteByPathParams{WorkspaceID: e.ws, Path: "Patterns/Proposed/Wrap errors with context", Viewer: pageview.From(e.ctx).UserID}); err == nil {
		t.Fatal("proposal page still exists")
	}

	// A published title can't be proposed again or published over.
	if _, err := propose(e.atlas, "Wrap errors at boundaries"); !invalid(err) {
		t.Fatalf("propose published title: %v", err)
	}
	if _, err := propose(e.atlas, "Another idea"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.PublishPattern(e.ctx, e.echo, curator, wiki.PublishInput{ProposedTitle: "Another idea", Topic: "Architecture", Title: "Wrap errors at boundaries"}); err == nil {
		t.Fatal("published over an existing pattern")
	}
	// Edited body without a header keeps the proposal's tags.
	pub, err = e.svc.PublishPattern(e.ctx, e.echo, curator, wiki.PublishInput{ProposedTitle: "Another idea", Body: "## When to use\n\nEdited."})
	if err != nil || *pub.Note.Path != "Patterns/Go/Another idea" {
		t.Fatalf("publish edited = %+v, %v", pub, err)
	}

	if _, err := propose(e.atlas, "Bad idea"); err != nil {
		t.Fatal(err)
	}
	who, err := e.svc.RejectPattern(e.ctx, e.echo, curator, "Bad idea")
	if err != nil || who != "@atlas" {
		t.Fatalf("reject = %q, %v", who, err)
	}
	if list, _ := e.svc.ListProposed(e.ctx, e.ws); len(list) != 0 {
		t.Fatalf("rejected proposal listed: %+v", list)
	}
}
