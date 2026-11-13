package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/wiki"
)

type pageTest struct {
	ctx     context.Context
	q       *store.Queries
	wk      *wiki.Service
	user    store.User
	agent   store.Agent
	chat    store.ChatSession
	persona store.Persona
	handler http.Handler
}

func pageSetup(t *testing.T) pageTest {
	t.Helper()
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	user, err := q.GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: user.WorkspaceID, Handle: "echo"})
	if err != nil {
		t.Fatal(err)
	}
	persona, err := q.CreatePersona(ctx, store.CreatePersonaParams{WorkspaceID: user.WorkspaceID, UserID: user.ID, Name: "Page editor", Tone: "friendly", Color: "#336699", Repos: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := q.CreateChatSession(ctx, store.CreateChatSessionParams{WorkspaceID: user.WorkspaceID, CreatedBy: user.ID, AgentID: agent.ID, PersonaID: &persona.ID, Title: "Page chat", Harness: "claude", Model: "sonnet", Effort: "medium"})
	if err != nil {
		t.Fatal(err)
	}
	wk := &wiki.Service{Pool: pool, Artifacts: arts}
	handler := (&Server{AnonymousAPI: true, Pool: pool, Q: q, Artifacts: arts, Wiki: wk}).Handler()
	return pageTest{ctx, q, wk, user, agent, chat, persona, handler}
}
func (e pageTest) request(t *testing.T, method, path string, body any, status int, out any) {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	if rec.Code != status {
		t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body.String())
	}
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatal(err)
		}
	}
}
func (e pageTest) create(t *testing.T, body string) api.Artifact {
	t.Helper()
	var a api.Artifact
	e.request(t, "POST", "/wiki/pages", map[string]any{"path": "Review/Page", "body_md": body, "properties": map[string]any{"status": "old", "tags": []string{"old"}}}, 201, &a)
	return a
}
func (e pageTest) propose(t *testing.T, a api.Artifact, body string, props artifacts.Properties) wiki.Result {
	t.Helper()
	r, err := e.wk.PersonaWrite(e.ctx, e.agent, e.chat.ID, nil, wiki.Save{Path: *a.Path, Body: body, Properties: props, Note: "Clarify the page"})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func (e pageTest) detail(t *testing.T) api.Artifact {
	t.Helper()
	var a api.Artifact
	e.request(t, "GET", "/wiki/page?path=Review/Page", nil, 200, &a)
	return a
}
func resolutionURL(a api.Artifact, v uuid.UUID) string {
	return fmt.Sprintf("/pages/%s/suggestions/%s/resolve", a.Id, v)
}
func (e pageTest) messages(t *testing.T) []store.ChatMessage {
	t.Helper()
	rows, err := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: e.chat.ID, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestPageSuggestionResolveAPI(t *testing.T) {
	e := pageSetup(t)
	base := "First.\n\nAnchor one.\n\nSecond.\n\nAnchor two.\n\nThird.\n"
	a := e.create(t, base)
	next := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(base, "First.", "First improved."), "Second.", "Second improved."), "Third.", "Third improved.")
	s := e.propose(t, a, next, artifacts.Properties{"status": "new", "tags": []string{"new"}})
	var pending struct {
		Suggestions []api.WikiSuggestion `json:"suggestions"`
	}
	e.request(t, "GET", "/wiki/suggestions?artifact_id="+a.Id.String(), nil, 200, &pending)
	if len(pending.Suggestions) != 1 || pending.Suggestions[0].Changes == nil || len(*pending.Suggestions[0].Changes) != 5 || pending.Suggestions[0].PersonaId == nil || *pending.Suggestions[0].PersonaId != e.persona.ID {
		t.Fatalf("stored changes/author missing: %+v", pending)
	}
	// A human edits away only the middle target and changes the status property.
	e.request(t, "PUT", "/wiki/pages/"+a.Id.String(), map[string]any{"body_md": strings.ReplaceAll(base, "Second.", "Person's second."), "properties": map[string]any{"status": "human", "tags": []string{"old"}}}, 200, nil)
	var changes []wiki.Change
	e.request(t, "POST", resolutionURL(a, s.Version.ID), wiki.Resolution{Decisions: []wiki.ChangeDecision{{I: 2, Accept: false, Reason: "Keep my ending"}}, Rest: "accept"}, 200, &changes)
	states := []string{}
	for _, c := range changes {
		states = append(states, c.State)
		if c.DecidedBy == nil || *c.DecidedBy != e.user.ID || c.DecidedAt == nil {
			t.Fatal("missing decision audit")
		}
	}
	if strings.Join(states, ",") != "accepted,stale,rejected,stale,accepted" {
		t.Fatalf("states: %v", states)
	}
	got := e.detail(t)
	want := strings.ReplaceAll(strings.ReplaceAll(base, "First.", "First improved."), "Second.", "Person's second.")
	if got.BodyMd.MustGet() != want || got.CurrentVersion.CreatedBy.Type != "user" || (got.CurrentVersion.CreatedBy.Id == nil || *got.CurrentVersion.CreatedBy.Id != e.user.ID) || (*got.Properties)["status"] != "human" {
		t.Fatalf("human edits lost: %+v", got)
	}
	if len(got.Versions) != 3 {
		t.Fatalf("expected initial + human edit + one review version, got %d", len(got.Versions))
	}
	if got.Tags == nil || strings.Join(*got.Tags, ",") != "new" {
		t.Fatal("accepted tag not indexed")
	}
	messages := e.messages(t)
	if len(messages) != 1 || messages[0].Role != "system" || messages[0].PersonaID == nil || *messages[0].PersonaID != e.persona.ID || !strings.Contains(messages[0].BodyMd, "Keep my ending") || !strings.Contains(messages[0].BodyMd, "stale") {
		t.Fatalf("feedback: %+v", messages)
	}
	e.request(t, "POST", resolutionURL(a, s.Version.ID), wiki.Resolution{Rest: "accept"}, 409, nil)
	if len(e.messages(t)) != 1 {
		t.Fatal("repeat decision duplicated feedback")
	}
}

func TestPageSuggestionPartialAndValidationAPI(t *testing.T) {
	e := pageSetup(t)
	a := e.create(t, "One.\n\nAnchor.\n\nTwo.\n")
	s := e.propose(t, a, "One edited.\n\nAnchor.\n\nTwo edited.\n", nil)
	url := resolutionURL(a, s.Version.ID)
	e.request(t, "POST", url, wiki.Resolution{Decisions: []wiki.ChangeDecision{{I: 0, Accept: true}, {I: 0, Accept: false}}}, 422, nil)
	e.request(t, "POST", url, wiki.Resolution{Decisions: []wiki.ChangeDecision{{I: 99, Accept: true}}}, 422, nil)
	e.request(t, "POST", url, wiki.Resolution{}, 422, nil)
	var changes []wiki.Change
	e.request(t, "POST", url, wiki.Resolution{Decisions: []wiki.ChangeDecision{{I: 0, Accept: true}}}, 200, &changes)
	if changes[0].State != "accepted" || changes[1].State != "pending" {
		t.Fatal(changes)
	}
	e.request(t, "POST", url, wiki.Resolution{Decisions: []wiki.ChangeDecision{{I: 0, Accept: true}}}, 409, nil)
	e.request(t, "POST", url, wiki.Resolution{Rest: "reject", Reason: "Leave the rest"}, 200, &changes)
	if changes[1].State != "rejected" || !strings.Contains(e.messages(t)[0].BodyMd, "Leave the rest") {
		t.Fatal("reject rest")
	}
	got := e.detail(t)
	if got.BodyMd.MustGet() != "One edited.\n\nAnchor.\n\nTwo.\n" {
		t.Fatal(got.BodyMd)
	}
	e.request(t, "POST", fmt.Sprintf("/pages/%s/suggestions/%s/resolve", uuid.New(), s.Version.ID), wiki.Resolution{Rest: "accept"}, 404, nil)
	e.request(t, "POST", resolutionURL(a, uuid.New()), wiki.Resolution{Rest: "accept"}, 404, nil)
}

func TestPageDirectPauseAndRestoreAPI(t *testing.T) {
	e := pageSetup(t)
	a := e.create(t, "Original.\n\nAnchor.\n\nEnding.\n")
	e.request(t, "PUT", "/pages/"+a.Id.String()+"/persona-edits", map[string]bool{"direct": true}, 204, nil)
	s := e.propose(t, a, "Improved.\n\nAnchor.\n\nEnding.\n", artifacts.Properties{"status": "new", "tags": []string{"new"}})
	got := e.detail(t)
	if s.State != "published" || got.CurrentVersion.CreatedBy.Type != "agent" || (got.CurrentVersion.CreatedBy.Id == nil || *got.CurrentVersion.CreatedBy.Id != e.agent.ID) || got.CurrentVersion.PersonaId == nil || *got.CurrentVersion.PersonaId != e.persona.ID || got.PersonaDirect == nil || !*got.PersonaDirect {
		t.Fatalf("direct author/controls: %+v", got)
	}
	if got.CurrentVersion.Changes == nil || string((*got.CurrentVersion.Changes)[0].State) != "applied" || got.CurrentVersion.Review == nil || got.CurrentVersion.Review.State != "pending" {
		t.Fatal("direct changes need review")
	}
	// Rejecting a direct edit creates an inverse human version and preserves
	// the person's newer edit to another passage and property.
	e.request(t, "PUT", "/wiki/pages/"+a.Id.String(), map[string]any{"body_md": "Improved.\n\nAnchor.\n\nHuman ending.\n", "properties": map[string]any{"status": "human", "tags": []string{"new"}}}, 200, nil)
	var changes []wiki.Change
	e.request(t, "POST", resolutionURL(a, s.Version.ID), wiki.Resolution{Rest: "reject", Reason: "Undo persona changes"}, 200, &changes)
	got = e.detail(t)
	if got.BodyMd.MustGet() != "Original.\n\nAnchor.\n\nHuman ending.\n" || (*got.Properties)["status"] != "human" || changes[1].State != "stale" || got.CurrentVersion.CreatedBy.Type != "user" {
		t.Fatalf("direct review lost human edit: %+v %+v", got, changes)
	}
	e.request(t, "PUT", "/pages/"+a.Id.String()+"/persona-edits", map[string]bool{"paused": true}, 204, nil)
	if _, err := e.wk.PersonaWrite(e.ctx, e.agent, e.chat.ID, nil, wiki.Save{Path: *a.Path, Body: "Refused"}); err == nil || !strings.Contains(err.Error(), "paused") {
		t.Fatalf("pause: %v", err)
	}
	if e.detail(t).CurrentVersion.Id != got.CurrentVersion.Id {
		t.Fatal("paused write changed version")
	}
	var restored api.Artifact
	e.request(t, "POST", fmt.Sprintf("/pages/%s/versions/%s/restore", a.Id, a.CurrentVersion.Id), nil, 200, &restored)
	if restored.BodyMd.MustGet() != a.BodyMd.MustGet() || restored.CurrentVersion.Id == a.CurrentVersion.Id || restored.CurrentVersion.CreatedBy.Type != "user" || (*restored.Properties)["status"] != "old" || strings.Join(*restored.Tags, ",") != "old" {
		t.Fatal("restore failed", restored)
	}
	e.request(t, "POST", fmt.Sprintf("/pages/%s/versions/%s/restore", a.Id, s.Version.ID), nil, 200, &restored)
	if restored.BodyMd.MustGet() != "Improved.\n\nAnchor.\n\nEnding.\n" || restored.CurrentVersion.PersonaId != nil {
		t.Fatal("restore persona history failed")
	}
	e.request(t, "POST", fmt.Sprintf("/pages/%s/versions/%s/restore", a.Id, uuid.New()), nil, 404, nil)
	e.request(t, "PUT", "/pages/"+uuid.NewString()+"/persona-edits", map[string]bool{"paused": true}, 404, nil)
}

func TestPageConcurrentResolutionAPI(t *testing.T) {
	e := pageSetup(t)
	a := e.create(t, "Original")
	s := e.propose(t, a, "Suggested", nil)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Go(func() {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/api/v1"+resolutionURL(a, s.Version.ID), strings.NewReader(`{"rest":"accept"}`))
			req.Header.Set("Content-Type", "application/json")
			e.handler.ServeHTTP(rec, req)
			codes <- rec.Code
		})
	}
	wg.Wait()
	close(codes)
	success, conflict := 0, 0
	for code := range codes {
		if code == 200 {
			success++
		}
		if code == 409 {
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
	if len(e.detail(t).Versions) != 2 {
		t.Fatal("double resolution created extra version")
	}
}

func TestPageLegacySuggestionAPI(t *testing.T) {
	e := pageSetup(t)
	a := e.create(t, "Original.\n\nAnchor.\n\nEnding.\n")
	s := e.propose(t, a, "Suggested.\n\nAnchor.\n\nEnding.\n", nil)
	// Simulate an existing pre-migration suggestion without a stored changes array.
	if _, err := e.q.SetPageVersionChanges(e.ctx, store.SetPageVersionChangesParams{ID: s.Version.ID, Changes: nil, PersonaID: &e.persona.ID, SourceChatID: &e.chat.ID}); err != nil {
		t.Fatal(err)
	}
	e.request(t, "POST", "/wiki/suggestions/"+s.Version.ID.String()+"/accept", nil, 204, nil)
	got := e.detail(t)
	if got.BodyMd.MustGet() != "Suggested.\n\nAnchor.\n\nEnding.\n" || got.CurrentVersion.CreatedBy.Type != "user" {
		t.Fatal("legacy fallback failed")
	}
	e.request(t, "POST", "/wiki/suggestions/"+s.Version.ID.String()+"/accept", nil, 409, nil)
}

func TestPageFeedbackFailureRollsBackAPI(t *testing.T) {
	e := pageSetup(t)
	a := e.create(t, "Original.\n\nAnchor.\n\nEnding.\n")
	s := e.propose(t, a, "Edited.\n\nAnchor.\n\nOther ending.\n", nil)
	_, err := e.wk.Pool.Exec(e.ctx, `CREATE FUNCTION refuse_feedback() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'feedback failed'; END $$; CREATE TRIGGER refuse_feedback BEFORE INSERT ON chat_messages FOR EACH ROW EXECUTE FUNCTION refuse_feedback()`)
	if err != nil {
		t.Fatal(err)
	}
	e.request(t, "POST", resolutionURL(a, s.Version.ID), wiki.Resolution{Decisions: []wiki.ChangeDecision{{I: 0, Accept: true}, {I: 1, Accept: false, Reason: "Keep ending"}}}, 500, nil)
	got := e.detail(t)
	if got.CurrentVersion.Id != a.CurrentVersion.Id || got.BodyMd.MustGet() != a.BodyMd.MustGet() {
		t.Fatal("failed feedback committed version")
	}
	v, err := e.q.GetArtifactVersion(e.ctx, store.GetArtifactVersionParams{ID: s.Version.ID, ArtifactID: a.Id, WorkspaceID: e.user.WorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	var changes []wiki.Change
	if err := json.Unmarshal(v.Changes, &changes); err != nil {
		t.Fatal(err)
	}
	for _, c := range changes {
		if c.State != "pending" {
			t.Fatal("failed feedback committed decision")
		}
	}
}

func TestPageRunFeedbackAndScopesAPI(t *testing.T) {
	e := pageSetup(t)
	a := e.create(t, "Original")
	// A page chat is already linked, but a ticket suggestion must return
	// its feedback to that ticket rather than to the unrelated page chat.
	e.propose(t, a, "Chat proposal", nil)
	ticket, err := e.q.GetTicketByKey(e.ctx, store.GetTicketByKeyParams{WorkspaceID: e.user.WorkspaceID, Key: "PAY-214"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := e.q.CreateRun(e.ctx, store.CreateRunParams{WorkspaceID: e.user.WorkspaceID, TicketID: &ticket.ID, AgentID: e.agent.ID, Mode: "managed", State: "running", ResolvedSpec: json.RawMessage(`{}`), Harness: "claude", Model: "test", Purpose: "ticket"})
	if err != nil {
		t.Fatal(err)
	}
	e.request(t, "PUT", "/pages/"+a.Id.String()+"/persona-edits", map[string]bool{"direct": true, "paused": true}, 204, nil)
	// Page switches apply to personas, not ticket runs: no wiki:write scope
	// still produces a suggestion, while an allowed scope remains direct.
	s, err := e.wk.AgentWrite(e.ctx, e.agent, nil, wiki.Save{Path: *a.Path, Body: "Ticket proposal", RunID: &run.ID, Note: "Ticket explanation"})
	if err != nil || s.State != "suggested" {
		t.Fatalf("run scope behavior: %+v %v", s, err)
	}
	e.request(t, "POST", resolutionURL(a, s.Version.ID), wiki.Resolution{Rest: "reject", Reason: "Ticket feedback reason"}, 200, nil)
	var body string
	err = e.wk.Pool.QueryRow(e.ctx, `SELECT body_md FROM comments WHERE ticket_id=$1 AND body_md LIKE '%Ticket feedback reason%'`, ticket.ID).Scan(&body)
	if err != nil || !strings.Contains(body, "Ticket proposal") || !strings.Contains(body, "Ticket explanation") {
		t.Fatalf("run feedback: %q %v", body, err)
	}
	if len(e.messages(t)) != 0 {
		t.Fatal("ticket feedback went to page chat")
	}
}
