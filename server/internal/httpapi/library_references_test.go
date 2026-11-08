package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/chatmodels"
	"github.com/liamphmurphy/vesper/server/internal/personas"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

// uploadLibrary POSTs data to /library as the multipart file part.
func (e *chatEnv) uploadLibrary(filename string, data []byte) (int, map[string]any) {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	pw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		e.t.Fatal(err)
	}
	pw.Write(data)
	mw.Close()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/library", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	raw, _ := io.ReadAll(res.Body)
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

// libraryRefs maps each listed item's id to its reference ("" for none, else
// role or role:persona name).
func (e *chatEnv) libraryRefs() map[string]string {
	e.t.Helper()
	out := map[string]string{}
	for _, it := range e.want("GET", "/library", nil, http.StatusOK)["items"].([]any) {
		m := it.(map[string]any)
		ref, _ := m["reference"].(string)
		if name, ok := m["reference_persona_name"].(string); ok {
			ref += ":" + name
		}
		out[m["id"].(string)] = ref
	}
	return out
}

// The person uploads their own images (images only); each persona and the
// person have at most one reference, a new one replaces the old, and only the
// owner can set one, for their own personas.
func TestLibraryUploadAndReferences(t *testing.T) {
	e, s3 := newAvatarEnv(t)
	code, me := e.uploadLibrary("Me at the beach.png", avatarPNG(t))
	if code != http.StatusCreated || me["title"] != "Me at the beach" || me["content_type"] != "image/png" || me["chat_id"] != nil || me["reference"] != nil {
		t.Fatalf("upload %d %v", code, me)
	}
	if keys := s3.keys(avatarBucket + "/chat-attachments/"); len(keys) != 1 {
		t.Fatalf("stored %v", keys)
	}
	_, other := e.uploadLibrary("other.jpg", avatarJPEG(t))
	if code, out := e.uploadLibrary("notes.txt", []byte("hello")); code != http.StatusUnprocessableEntity || out["field"] != "file" {
		t.Fatalf("text upload %d %v", code, out)
	}
	if code, out := e.uploadLibrary("fake.png", []byte("not an image")); code != http.StatusUnprocessableEntity {
		t.Fatalf("fake png %d %v", code, out)
	}
	meID, otherID := me["id"].(string), other["id"].(string)

	gwen := e.want("POST", "/personas", map[string]any{"name": "Gwen"}, http.StatusCreated)
	gwenID := gwen["id"].(string)
	e.invalid("PUT", "/library/"+meID+"/reference", map[string]any{"role": "persona"}, "persona_id", "required")
	e.invalid("PUT", "/library/"+meID+"/reference", map[string]any{"role": "self", "persona_id": gwenID}, "persona_id", "only for role persona")
	e.invalid("PUT", "/library/"+meID+"/reference", map[string]any{"role": "persona", "persona_id": e.strangerPersona()}, "persona_id", "not one of your personas")
	e.invalid("PUT", "/library/"+meID+"/reference", map[string]any{"role": "persona", "persona_id": uuid.NewString()}, "persona_id", "not one of your personas")
	e.want("PUT", "/library/"+uuid.NewString()+"/reference", map[string]any{"role": "self"}, http.StatusNotFound)

	e.want("PUT", "/library/"+meID+"/reference", map[string]any{"role": "self"}, http.StatusNoContent)
	e.want("PUT", "/library/"+otherID+"/reference", map[string]any{"role": "persona", "persona_id": gwenID}, http.StatusNoContent)
	if refs := e.libraryRefs(); refs[meID] != "self" || refs[otherID] != "persona:Gwen" {
		t.Fatalf("refs %v", refs)
	}
	// A new self reference replaces the old one, and the image stops being Gwen's.
	e.want("PUT", "/library/"+otherID+"/reference", map[string]any{"role": "self"}, http.StatusNoContent)
	if refs := e.libraryRefs(); refs[meID] != "" || refs[otherID] != "self" {
		t.Fatalf("after replacing %v", refs)
	}
	// A new persona reference replaces that persona's previous one only.
	e.want("PUT", "/library/"+meID+"/reference", map[string]any{"role": "persona", "persona_id": gwenID}, http.StatusNoContent)
	_, third := e.uploadLibrary("gwen2.png", avatarPNG(t))
	thirdID := third["id"].(string)
	e.want("PUT", "/library/"+thirdID+"/reference", map[string]any{"role": "persona", "persona_id": gwenID}, http.StatusNoContent)
	if refs := e.libraryRefs(); refs[meID] != "" || refs[otherID] != "self" || refs[thirdID] != "persona:Gwen" {
		t.Fatalf("after replacing the persona's %v", refs)
	}

	// Someone else can't see, set or clear them.
	ctx := t.Context()
	stranger, err := e.q.CreateUser(ctx, store.CreateUserParams{WorkspaceID: e.me.WorkspaceID, Name: "Other", Email: "o@example.com", Initials: "OT"})
	if err != nil {
		t.Fatal(err)
	}
	if refs, err := e.q.LibraryReferences(ctx, store.LibraryReferencesParams{WorkspaceID: e.me.WorkspaceID, UserID: stranger.ID, PersonaID: new(uuid.MustParse(gwenID))}); err != nil || len(refs) != 0 {
		t.Fatalf("stranger's references %v %v", refs, err)
	}
	if err := (&Server{Pool: e.pool, Q: e.q}).setLibraryReference(ctx, e.me.WorkspaceID, stranger.ID, uuid.MustParse(otherID), nil, nil); err == nil {
		t.Fatal("a stranger cleared the reference")
	}

	// Clearing keeps the image; deleting the persona unlinks its reference.
	e.want("DELETE", "/library/"+otherID+"/reference", nil, http.StatusNoContent)
	e.want("DELETE", "/personas/"+gwenID, nil, http.StatusNoContent)
	if refs := e.libraryRefs(); len(refs) != 3 || refs[otherID] != "" || refs[thirdID] != "" {
		t.Fatalf("after clearing %v", refs)
	}
	e.want("DELETE", "/library/"+uuid.NewString()+"/reference", nil, http.StatusNotFound)
}

// generate_image accepts references the person has set for this chat's
// persona and themselves, stores them on the job, and refuses the rest with a
// message the model can act on.
func TestGenerateImageReferences(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	q := store.New(pool)
	arts := &artifacts.Service{Pool: pool}
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true, Artifacts: arts}); err != nil {
		t.Fatal(err)
	}
	me, _ := q.GetLocalUser(ctx)
	ws := me.WorkspaceID
	tr := &tracker.Service{Pool: pool}
	rs := &runs.Service{Pool: pool, Tracker: tr, Artifacts: arts, ModelDiscovery: func(_ context.Context, h, _ string) ([]chatmodels.Model, error) {
		return []chatmodels.Model{{ID: h + "-m1", Default: true}}, nil
	}}
	handler := (&Server{Pool: pool, Q: q, Tracker: tr, Runs: rs, Artifacts: arts}).Handler()
	images := true
	ps := &personas.Service{Pool: pool}
	owner := personas.Owner{WorkspaceID: ws, UserID: me.ID}
	p, err := ps.Create(ctx, owner, personas.NewInput{Name: "Illustrator", Images: &images})
	if err != nil {
		t.Fatal(err)
	}
	otherPersona, err := ps.Create(ctx, owner, personas.NewInput{Name: "Other", Images: &images})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := rs.StartChat(ctx, ws, me.ID, runs.ChatInput{Project: "PAY", PersonaID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = rs.PostChatMessage(ctx, ws, me.ID, chat.ID, runs.ChatMessageInput{Body: "draw us"}); err != nil {
		t.Fatal(err)
	}
	chat, _ = q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat.ID, WorkspaceID: ws})
	runID := *chat.CurrentRunID
	assistant, _ := q.GetAgent(ctx, store.GetAgentParams{ID: chat.AgentID, WorkspaceID: ws})
	token, _, err := agents.IssueToken(ctx, q, assistant.ID, "run", "chat", &runID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE runs SET state='running', harness=$2 WHERE id=$1", runID, agentdef.HarnessClaudeCode); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE run_turns SET status='running' WHERE run_id=$1", runID); err != nil {
		t.Fatal(err)
	}
	call := func(args any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(args)
		req := httptest.NewRequest("POST", "/api/v1/agent/tools/generate_image", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	reference := func(role string, persona *uuid.UUID) {
		t.Helper()
		it, err := q.CreateLibraryItem(ctx, store.CreateLibraryItemParams{WorkspaceID: ws, UserID: me.ID, Kind: "image", Title: "ref", Filename: "ref.png", ContentType: "image/png", SizeBytes: 3,
			BlobKey: "chat-attachments/" + uuid.NewString()})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q.SetLibraryReference(ctx, store.SetLibraryReferenceParams{ID: it.ID, WorkspaceID: ws, UserID: me.ID, Role: &role, PersonaID: persona}); err != nil {
			t.Fatal(err)
		}
	}
	reject := func(label string, refs []string, code int, msg string) {
		t.Helper()
		got, out := call(map[string]any{"prompt": "us", "references": refs})
		if got != code || !strings.Contains(fmt.Sprint(out["message"]), msg) {
			t.Fatalf("%s: %d %v", label, got, out)
		}
	}

	reject("unknown role", []string{"dog"}, 422, `use "persona"`)
	reject("no person reference", []string{"person"}, 409, "hasn't set a reference image of themselves")
	// Another persona's reference doesn't count for this chat.
	reference("persona", &otherPersona.ID)
	reject("no persona reference", []string{"persona"}, 409, "no reference image of you")
	reference("persona", &p.ID)
	reference("self", nil)
	if code, out := call(map[string]any{"prompt": "us", "references": []string{"person", "persona", "person"}}); code != 200 {
		t.Fatalf("queue: %d %v", code, out)
	}
	if code, out := call(map[string]any{"prompt": "a tree"}); code != 200 {
		t.Fatalf("queue without references: %d %v", code, out)
	}
	var got []string
	rows, err := pool.Query(ctx, "SELECT array_to_string(reference_roles, ',') FROM chat_image_jobs WHERE run_id=$1 ORDER BY id", runID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		got = append(got, s)
	}
	rows.Close()
	if strings.Join(got, "|") != "person,persona|" {
		t.Fatalf("jobs' references %q", got)
	}
}
