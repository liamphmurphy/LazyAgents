package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/links"
	"github.com/liamphmurphy/vesper/server/internal/slides"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/wiki"
)

func (e *chatEnv) uploadAttachment(pageID, filename, declared string, data []byte) (int, map[string]any) {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	h.Set("Content-Type", declared)
	pw, err := mw.CreatePart(h)
	if err != nil {
		e.t.Fatal(err)
	}
	pw.Write(data)
	mw.Close()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/wiki/pages/"+pageID+"/attachments", &body)
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

// embeds resolves embed texts as the signed-in person, keyed by ref.
func (e *chatEnv) embeds(from string, refs ...string) map[string]map[string]any {
	e.t.Helper()
	v := url.Values{"ref": refs}
	if from != "" {
		v.Set("from", from)
	}
	out := e.want("GET", "/knowledge/embeds?"+v.Encode(), nil, http.StatusOK)
	byRef := map[string]map[string]any{}
	for _, x := range out["embeds"].([]any) {
		m := x.(map[string]any)
		byRef[m["ref"].(string)] = m
	}
	return byRef
}

// runToken is an MCP token for a ticket run (no person behind it).
func (e *chatEnv) runToken() string {
	e.t.Helper()
	ctx, ws := e.t.Context(), e.me.WorkspaceID
	atlas, err := e.q.GetAgentByHandle(ctx, store.GetAgentByHandleParams{WorkspaceID: ws, Handle: "atlas"})
	if err != nil {
		e.t.Fatal(err)
	}
	tk, err := e.q.GetTicketByKey(ctx, store.GetTicketByKeyParams{WorkspaceID: ws, Key: e.payKey})
	if err != nil {
		e.t.Fatal(err)
	}
	run, err := e.q.CreateRun(ctx, store.CreateRunParams{WorkspaceID: ws, TicketID: &tk.ID, AgentID: atlas.ID, Mode: "managed", State: "running", Purpose: "ticket", Harness: "claude", Model: "test", ResolvedSpec: json.RawMessage(`{}`)})
	if err != nil {
		e.t.Fatal(err)
	}
	token, _, err := agents.IssueToken(ctx, e.q, atlas.ID, "run", "embeds", &run.ID, nil, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return token
}

func (e *chatEnv) pageLinks(id string) []store.KnowledgeLink {
	e.t.Helper()
	ls, err := e.q.LinksFromSource(e.t.Context(), store.LinksFromSourceParams{SourceKind: links.SourceArtifact, SourceID: uuid.MustParse(id)})
	if err != nil {
		e.t.Fatal(err)
	}
	return ls
}

func (e *slidesEnv) pageBlobs() []string {
	e.s3.mu.Lock()
	defer e.s3.mu.Unlock()
	var keys []string
	for k := range e.s3.objs {
		if strings.Contains(k, "/pages/") {
			keys = append(keys, k)
		}
	}
	return keys
}

// Links with labels, headings and blocks are indexed and resolved; page,
// heading and block embeds return their content; a run never gets a private
// page's content through an embed.
func TestPageLinksBlocksAndEmbeds(t *testing.T) {
	e, _ := newDesignToolsEnv(t)
	runbook := e.want("POST", "/wiki/pages", map[string]any{"path": "Runbook", "audience": "shared", "body_md": strings.Join([]string{
		"# Runbook", "intro", "## Roll back", "Step one.", "", "Restart the ledger", "and wait. ^restart", "", "- check queues ^queues", "  - nested", "## Verify", "done",
	}, "\n")}, http.StatusCreated)
	e.want("POST", "/wiki/pages", map[string]any{"path": "Secret plan", "body_md": "Private payload 9f3a ^p1"}, http.StatusCreated)
	hub := e.want("POST", "/wiki/pages", map[string]any{"path": "Hub", "audience": "shared", "body_md": strings.Join([]string{
		"[[Runbook|the runbook]], [[Runbook#Roll back]] and [[Runbook#^restart]].",
		"![[Runbook#Roll back]]", "![[Runbook#^queues]]", "![[Secret plan]]", "![[Secret plan#^p1]]", "![[Later page#^x]]",
	}, "\n")}, http.StatusCreated)
	hubID, runbookID := hub["id"].(string), runbook["id"].(string)

	// Criterion 1: label, heading and block links are parsed, indexed and resolved.
	got := map[string]store.KnowledgeLink{}
	for _, l := range e.pageLinks(hubID) {
		got[l.RawText] = l
	}
	for raw, want := range map[string][2]string{
		"[[Runbook|the runbook]]": {links.KindArtifact, ""},
		"[[Runbook#Roll back]]":   {links.KindSection, "roll-back"},
		"[[Runbook#^restart]]":    {links.KindBlock, "^restart"},
		"![[Runbook#^queues]]":    {links.KindBlock, "^queues"},
		"![[Later page#^x]]":      {links.KindUnresolved, "^x"},
		"![[Runbook#Roll back]]":  {links.KindSection, "roll-back"},
		"![[Secret plan#^p1]]":    {links.KindBlock, "^p1"},
	} {
		l, ok := got[raw]
		if !ok || l.TargetKind != want[0] || l.TargetAnchor != want[1] {
			t.Errorf("%s indexed as %+v, want %v", raw, l, want)
			continue
		}
		if want[0] != links.KindUnresolved && (l.TargetID == nil || l.TargetID.String() != runbookID) && !strings.Contains(raw, "Secret") {
			t.Errorf("%s points at %v, want Runbook", raw, l.TargetID)
		}
	}
	res := e.want("GET", "/knowledge/resolve?ref="+url.QueryEscape("Runbook#^restart"), nil, http.StatusOK)
	if tg := res["links"].([]any)[0].(map[string]any)["target"].(map[string]any); tg["kind"] != "block" || tg["anchor"] != "^restart" || !strings.HasSuffix(tg["href"].(string), "#^restart") {
		t.Fatalf("resolve block link: %v", tg)
	}
	// An unresolved block link resolves once its page exists.
	e.want("POST", "/wiki/pages", map[string]any{"path": "Later page", "audience": "shared", "body_md": "Later text ^x"}, http.StatusCreated)
	for _, l := range e.pageLinks(hubID) {
		if l.RawText == "![[Later page#^x]]" && (l.TargetKind != links.KindBlock || l.TargetAnchor != "^x") {
			t.Fatalf("ghost block link after create: %+v", l)
		}
	}

	// Criterion 2/3: the API returns embedded content for the person.
	em := e.embeds(hubID, "Runbook", "Runbook#Roll back", "Runbook#^restart", "Runbook#^queues", "Secret plan", "Secret plan#^p1", "Later page#^x", "#Verify", "Runbook#^missing", e.payKey, "Nowhere")
	for ref, want := range map[string]string{
		"Runbook":           "# Runbook\nintro",
		"Runbook#Roll back": "## Roll back\nStep one.",
		"Runbook#^restart":  "Restart the ledger\nand wait.",
		"Runbook#^queues":   "- check queues\n  - nested",
		"Secret plan":       "Private payload 9f3a",
		"Secret plan#^p1":   "Private payload 9f3a",
		"Later page#^x":     "Later text",
	} {
		m := em[ref]
		if m["available"] != true || !strings.HasPrefix(m["markdown"].(string), want) {
			t.Errorf("embed %q: %v", ref, m)
		}
	}
	if m := em["Runbook#Roll back"]["markdown"].(string); strings.Contains(m, "## Verify") {
		t.Errorf("heading embed runs into the next section: %q", m)
	}
	if m := em["Runbook#^restart"]["markdown"].(string); strings.Contains(m, "^restart") {
		t.Errorf("block embed keeps its id marker: %q", m)
	}
	if em["#Verify"]["available"] != false {
		// Hub has no Verify heading: a same-page embed names the page it's on.
		t.Errorf("same-page heading: %v", em["#Verify"])
	}
	for _, ref := range []string{"Runbook#^missing", "Nowhere"} {
		if em[ref]["available"] != false {
			t.Errorf("%s should be unavailable: %v", ref, em[ref])
		}
	}
	if em[e.payKey]["available"] != true || em[e.payKey]["target"].(map[string]any)["kind"] != "ticket" {
		t.Errorf("ticket embed: %v", em[e.payKey])
	}

	// A run reads the shared hub: private embeds look exactly like missing ones.
	token := e.runToken()
	code, out := e.tool(token, "read_page", map[string]any{"ref": "Hub"})
	raw, _ := json.Marshal(out)
	if code != http.StatusOK || strings.Contains(string(raw), "Private payload") {
		t.Fatalf("run read_page leaked a private embed: %d %s", code, raw)
	}
	byRef := map[string]map[string]any{}
	for _, x := range out["embeds"].([]any) {
		m := x.(map[string]any)
		byRef[m["ref"].(string)] = m
	}
	if m := byRef["![[Secret plan]]"]; m["available"] != false || len(m) != 2 {
		t.Errorf("private page embed for a run: %v", m)
	}
	if m := byRef["![[Secret plan#^p1]]"]; m["available"] != false || len(m) != 2 {
		t.Errorf("private block embed for a run: %v", m)
	}
	if m := byRef["![[Runbook#^queues]]"]; m["available"] != true || !strings.HasPrefix(m["markdown"].(string), "- check queues") {
		t.Errorf("shared block embed for a run: %v", m)
	}
	// read_page follows a block link to just that block.
	if code, out := e.tool(token, "read_page", map[string]any{"ref": "[[Runbook#^restart]]"}); code != http.StatusOK || out["markdown"] != "Restart the ledger\nand wait." {
		t.Errorf("read_page block: %d %v", code, out)
	}
	if code, _ := e.tool(token, "read_page", map[string]any{"ref": "Secret plan#^p1"}); code == http.StatusOK {
		t.Error("run read a private block")
	}
}

// Attachments are stored by content hash, reused when identical, embedded
// as ![[name]] and swept once no page embeds them.
func TestPageAttachmentsStoreReuseAndSweep(t *testing.T) {
	e, _ := newDesignToolsEnv(t)
	ctx := t.Context()
	hub := e.want("POST", "/wiki/pages", map[string]any{"path": "Hub", "audience": "shared", "body_md": "Diagram: ![[diagram.png]]"}, http.StatusCreated)
	other := e.want("POST", "/wiki/pages", map[string]any{"path": "Other", "audience": "shared", "body_md": "Nothing embedded"}, http.StatusCreated)
	hubID, otherID := hub["id"].(string), other["id"].(string)
	if em := e.embeds(hubID, "diagram.png"); em["diagram.png"]["available"] != false {
		t.Fatalf("embed before upload: %v", em)
	}

	png := avatarPNG(t)
	code, a := e.uploadAttachment(hubID, "diagram.png", "image/png", png)
	if code != http.StatusCreated || a["content_type"] != "image/png" || a["name"] != "diagram.png" {
		t.Fatalf("upload: %d %v", code, a)
	}
	// The ghost ![[diagram.png]] resolved to the new attachment.
	if ls := e.pageLinks(hubID); len(ls) != 1 || ls[0].TargetKind != links.KindAttachment || ls[0].TargetID.String() != a["id"] {
		t.Fatalf("link after upload: %+v", ls)
	}
	em := e.embeds(hubID, "diagram.png")["diagram.png"]
	if em["available"] != true || em["content_type"] != "image/png" || em["content_url"] != a["href"] {
		t.Fatalf("file embed: %v", em)
	}
	res, err := http.Get(e.srv.URL + a["href"].(string))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !bytes.Equal(body, png) || res.Header.Get("Content-Type") != "image/png" || res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("content: %d %q", res.StatusCode, res.Header)
	}

	// Identical content: the same name on the same page is the same attachment;
	// on another page it's a new attachment sharing one blob.
	if code, again := e.uploadAttachment(hubID, "diagram.png", "image/png", png); code != http.StatusCreated || again["id"] != a["id"] {
		t.Fatalf("re-upload: %d %v", code, again)
	}
	code, copied := e.uploadAttachment(otherID, "copy.png", "", png)
	if code != http.StatusCreated || copied["id"] == a["id"] || copied["sha256"] != a["sha256"] {
		t.Fatalf("copy: %d %v", code, copied)
	}
	if keys := e.pageBlobs(); len(keys) != 1 || !strings.HasSuffix(keys[0], wiki.AttachmentBlobKey(e.me.WorkspaceID, a["sha256"].(string))) {
		t.Fatalf("blobs: %v", keys)
	}
	if code, out := e.uploadAttachment(otherID, "notes.txt", "text/plain", []byte("just text")); code != http.StatusUnprocessableEntity {
		t.Fatalf("text attachment: %d %v", code, out)
	}
	list := e.want("GET", "/wiki/pages/"+otherID+"/attachments", nil, http.StatusOK)["attachments"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["name"] != "copy.png" {
		t.Fatalf("list: %v", list)
	}

	sweep := func() (int, int) {
		t.Helper()
		later := time.Now().Add(time.Minute)
		n, err := wiki.SweepAttachments(ctx, e.q, later, 100)
		if err != nil {
			t.Fatal(err)
		}
		m, err := slides.SweepBlobs(ctx, e.pool, e.blobs, avatarBucket, later, 100)
		if err != nil {
			t.Fatal(err)
		}
		return n, m
	}
	// copy.png isn't embedded anywhere: it goes, but the blob stays for diagram.png.
	if n, m := sweep(); n != 1 || m != 0 || len(e.pageBlobs()) != 1 {
		t.Fatalf("first sweep: %d attachments, %d blobs, %v", n, m, e.pageBlobs())
	}
	if n, m := sweep(); n != 0 || m != 0 {
		t.Fatalf("a used attachment was swept: %d %d", n, m)
	}
	// Once the hub stops embedding it, the attachment and then its blob go.
	e.want("PUT", "/wiki/pages/"+hubID, map[string]any{"body_md": "No diagram any more"}, http.StatusOK)
	if n, m := sweep(); n != 1 || m != 1 || len(e.pageBlobs()) != 0 {
		t.Fatalf("final sweep: %d attachments, %d blobs, %v", n, m, e.pageBlobs())
	}
	if res, err := http.Get(e.srv.URL + a["href"].(string)); err != nil || res.StatusCode != http.StatusNotFound {
		t.Fatalf("swept attachment still served: %v %v", res, err)
	}
}

// A private page's attachment is invisible to a run, like the page.
func TestPrivatePageAttachmentHiddenFromRuns(t *testing.T) {
	e, _ := newDesignToolsEnv(t)
	private := e.want("POST", "/wiki/pages", map[string]any{"path": "Private scans", "body_md": "![[scan.png]]"}, http.StatusCreated)
	if code, out := e.uploadAttachment(private["id"].(string), "scan.png", "image/png", avatarPNG(t)); code != http.StatusCreated {
		t.Fatalf("upload: %d %v", code, out)
	}
	e.want("POST", "/wiki/pages", map[string]any{"path": "Shared scans", "audience": "shared", "body_md": "![[scan.png]]"}, http.StatusCreated)
	code, out := e.tool(e.runToken(), "read_page", map[string]any{"ref": "Shared scans"})
	raw, _ := json.Marshal(out)
	if code != http.StatusOK || strings.Contains(string(raw), "/wiki/attachments/") || !strings.Contains(string(raw), `"available":false`) {
		t.Fatalf("run saw a private page's attachment: %d %s", code, raw)
	}
}

// Criterion 5: a Designs frame and a Library item embed in a page for their
// owner, and are unavailable to a run.
func TestEmbedDesignFrameAndLibraryItem(t *testing.T) {
	e, _ := newDesignToolsEnv(t)
	designID, _, _ := e.acceptance()
	_, body := e.current(designID)
	frameID := ""
	for _, p := range body["pages"].([]any) {
		for _, c := range p.(map[string]any)["children"].([]any) {
			if n := c.(map[string]any); n["type"] == "frame" && frameID == "" {
				frameID = n["id"].(string)
			}
		}
	}
	if frameID == "" {
		t.Fatal("the acceptance design has no frame")
	}
	code, item := e.uploadLibrary("photo.png", avatarPNG(t))
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("library upload: %d %v", code, item)
	}
	itemID := item["id"].(string)

	designRef, libraryRef := "design:"+designID+"#"+frameID, "library:"+itemID
	p := e.want("POST", "/wiki/pages", map[string]any{"path": "Moodboard", "audience": "shared", "body_md": "![[" + designRef + "]]\n![[" + libraryRef + "]]\n![[design:" + designID + "#n_missing]]"}, http.StatusCreated)
	kinds := map[string]string{}
	for _, l := range e.pageLinks(p["id"].(string)) {
		kinds[l.RawText] = l.TargetKind
	}
	if kinds["![["+designRef+"]]"] != links.KindDesign || kinds["![["+libraryRef+"]]"] != links.KindLibrary {
		t.Fatalf("indexed: %v", kinds)
	}

	em := e.embeds(p["id"].(string), designRef, libraryRef, "design:"+designID+"#n_missing")
	d := em[designRef]
	if d["available"] != true || d["content_type"] != "image/png" || d["content_url"] != "/api/v1/designs/"+designID+"/render?node="+frameID {
		t.Fatalf("design embed: %v", d)
	}
	l := em[libraryRef]
	if l["available"] != true || l["content_url"] != "/api/v1/library/"+itemID+"/content" || l["content_type"] != "image/png" {
		t.Fatalf("library embed: %v", l)
	}
	if em["design:"+designID+"#n_missing"]["available"] != false {
		t.Fatalf("missing frame: %v", em)
	}

	code, out := e.tool(e.runToken(), "read_page", map[string]any{"ref": "Moodboard"})
	raw, _ := json.Marshal(out)
	if code != http.StatusOK || strings.Contains(string(raw), "/render") || strings.Contains(string(raw), "/content") {
		t.Fatalf("run saw a person's design or Library item: %d %s", code, raw)
	}
}
