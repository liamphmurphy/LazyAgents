package runner_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/blob"
	"github.com/liamphmurphy/vesper/server/internal/personas"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

func (e env) persona(name string, code, images bool, repos ...string) store.Persona {
	e.t.Helper()
	p, err := (&personas.Service{Pool: e.pool}).Create(e.ctx, personas.Owner{WorkspaceID: e.ws, UserID: e.me.ID},
		personas.NewInput{Name: name, Code: &code, Images: &images, Repos: &repos})
	if err != nil {
		e.t.Fatal(err)
	}
	return p
}

// git runs the real git (the only tool on the test PATH besides sh).
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// A coding chat gets the persona's repo on its own branch, can push that
// branch and nothing else, and a fresh segment resumes the pushed work.
func TestCodingChatRepoSetupPushAndResume(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	if err := e.setOverride(agentdef.Override{Harness: agentdef.HarnessClaudeCode, Model: "opus-5.5"}); err != nil {
		t.Fatal(err)
	}
	// A local bare repository stands in for GitHub; a persona can't list one,
	// so the chat's repo is repointed once it exists.
	bare, seed := filepath.Join(t.TempDir(), "origin.git"), t.TempDir()
	gitIn(t, seed, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, seed, "add", ".")
	gitIn(t, seed, "commit", "-qm", "init")
	gitIn(t, filepath.Dir(bare), "clone", "-q", "--bare", seed, bare)

	p := e.persona("Pair Programmer", true, false, "https://github.com/acme/app")
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: "Code chat", PersonaID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	// No branch until a change is asked for.
	if chat.RepoUrl == nil || *chat.RepoUrl != "https://github.com/acme/app" || len(chat.WorkBranches) != 0 {
		t.Fatalf("chat repo %v branches %v", chat.RepoUrl, chat.WorkBranches)
	}
	if _, err := e.pool.Exec(e.ctx, `WITH attached AS (UPDATE chat_repositories SET repo_url=$2 WHERE chat_id=$1) UPDATE chat_sessions SET repo_url=$2 WHERE id=$1`, chat.ID, bare); err != nil {
		t.Fatal(err)
	}
	// A repository the persona no longer lists would revoke coding.
	if _, err := e.pool.Exec(e.ctx, `UPDATE personas SET repos=ARRAY[$2]::text[] WHERE id=$1`, p.ID, bare); err != nil {
		t.Fatal(err)
	}

	reply := e.postSh(chat.ID, "git rev-parse --abbrev-ref HEAD; git config user.name; git config user.email; test -x .git/hooks/pre-push && echo hook; git check-ignore .lazyagents && echo ignored")
	// Commits are authored as the person who owns the chat, never the persona.
	for _, want := range []string{"main", e.me.Name, e.me.Email, "hook", "ignored"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("workspace setup: missing %q in %q", want, reply)
		}
	}
	if strings.Contains(reply, "Pair Programmer") || strings.Contains(reply, "personas.lazyagents.local") {
		t.Fatalf("workspace setup: commits authored as the persona: %q", reply)
	}
	spec, _ := runs.Spec(e.chatRun(chat.ID))
	if spec.Mode("repo:write") != agentdef.Allowed || spec.Mode("deploy:staging") != agentdef.Denied || spec.HarnessOptions["readonly"] != "" {
		t.Fatalf("coding chat spec: %+v", spec)
	}

	// The persona makes a branch per change, and the chat keeps each it pushed.
	e.postSh(chat.ID, "git checkout -q -b feat/add-work && echo done > work.txt && git add work.txt && git commit -qm work && git push -q -u origin feat/add-work")
	e.postSh(chat.ID, "git checkout -q -b fix/typo origin/HEAD && echo fixed > fix.txt && git add fix.txt && git commit -qm fix && git push -q -u origin fix/typo"+
		" && git checkout -q feat/add-work && echo more >> work.txt && git commit -qam more && git push -q")
	for _, b := range []string{"feat/add-work", "fix/typo"} {
		if got := gitIn(t, bare, "rev-parse", "--verify", "refs/heads/"+b); got == "" {
			t.Fatalf("%s not pushed", b)
		}
	}
	if got := e.chat(chat.ID).WorkBranches; !slices.Equal(got, []string{"feat/add-work", "fix/typo"}) {
		t.Fatalf("chat branches %v", got)
	}
	// The default branch, a misnamed branch, a tag, someone else's branch and
	// a delete are refused by the pre-push hook.
	gitIn(t, bare, "branch", "feat/theirs", "main")
	reply = e.postSh(chat.ID, "for r in refs/heads/main refs/heads/other refs/tags/v1 refs/heads/feat/theirs; do git push origin HEAD:$r 2>&1; done; git push origin :refs/heads/fix/typo 2>&1")
	for _, want := range []string{"isn't one", "wasn't made in this chat", "may not delete"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("hook didn't refuse (%q): %s", want, reply)
		}
	}
	out := gitIn(t, bare, "for-each-ref", "--format=%(refname) %(subject)")
	if strings.Contains(out, "other") || strings.Contains(out, "v1") || !strings.Contains(out, "refs/heads/main init") ||
		!strings.Contains(out, "refs/heads/feat/theirs init") || !strings.Contains(out, "refs/heads/fix/typo") {
		t.Fatalf("a foreign push landed on origin:\n%s", out)
	}

	// A new segment (here: a harness switch) starts in a fresh clone on the
	// default branch, is told its branches, and may keep pushing them.
	first := e.chatRun(chat.ID)
	e.selectChat(chat.ID, agentdef.HarnessCodex, "gpt-5.5")
	reply = e.postSh(chat.ID, "git rev-parse --abbrev-ref HEAD; git checkout -q feat/add-work && cat work.txt && echo again >> work.txt && git commit -qam again && git push -q 2>&1 && echo pushed")
	if e.chatRun(chat.ID).ID == first.ID || !strings.Contains(reply, "main") || !strings.Contains(reply, "more") || !strings.Contains(reply, "pushed") {
		t.Fatalf("new segment didn't pick up pushed work (same run %v): %s", e.chatRun(chat.ID).ID == first.ID, reply)
	}
	agentsMD, err := os.ReadFile(filepath.Join(e.runner.Backend.WorkspaceDir(e.chatRun(chat.ID).ID.String()), "AGENTS.md"))
	if err != nil || !strings.Contains(string(agentsMD), "feat/add-work, fix/typo") {
		t.Fatalf("live checkout context lost branches: %v\n%s", err, agentsMD)
	}
}

// A coding chat commits as the address its owner picked for it, from the next
// turn on (even within a segment), and goes back to their primary once that
// address leaves their profile.
func TestCodingChatCommitsAsPickedEmail(t *testing.T) {
	const commitAuthor = "git commit -q --allow-empty -m turn && git log -1 --format='author=%ae committer=%ce'"
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	if err := e.setOverride(agentdef.Override{Harness: agentdef.HarnessClaudeCode, Model: "opus-5.5"}); err != nil {
		t.Fatal(err)
	}
	bare, seed := filepath.Join(t.TempDir(), "origin.git"), t.TempDir()
	gitIn(t, seed, "init", "-q", "-b", "main")
	gitIn(t, seed, "commit", "-q", "--allow-empty", "-m", "init")
	gitIn(t, filepath.Dir(bare), "clone", "-q", "--bare", seed, bare)
	setEmails := func(extra ...string) {
		t.Helper()
		if _, err := e.pool.Exec(e.ctx, `UPDATE users SET extra_emails=$2 WHERE id=$1`, e.me.ID, append([]string{}, extra...)); err != nil {
			t.Fatal(err)
		}
	}
	setEmails("Ada@Work.example")

	p := e.persona("Pair Programmer", true, false, "https://github.com/acme/app")
	// Picked when the chat starts, matched ignoring case and stored as saved.
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Title: "Code chat", PersonaID: &p.ID, CommitEmail: "ada@work.example"})
	if err != nil {
		t.Fatal(err)
	}
	if chat.CommitEmail == nil || *chat.CommitEmail != "Ada@Work.example" {
		t.Fatalf("commit email %v", chat.CommitEmail)
	}
	// A persona can't list a local repository, so both are repointed at it.
	if _, err := e.pool.Exec(e.ctx, `WITH attached AS (UPDATE chat_repositories SET repo_url=$2 WHERE chat_id=$1) UPDATE chat_sessions SET repo_url=$2 WHERE id=$1`, chat.ID, bare); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(e.ctx, `UPDATE personas SET repos=ARRAY[$2]::text[] WHERE id=$1`, p.ID, bare); err != nil {
		t.Fatal(err)
	}
	if got := e.postSh(chat.ID, commitAuthor); !strings.Contains(got, "author=Ada@Work.example committer=Ada@Work.example") {
		t.Fatalf("first turn commits as %q", got)
	}
	segment := e.chatRun(chat.ID).ID
	if _, err := e.runs.SetChatCommitEmail(e.ctx, e.ws, e.me.ID, chat.ID, e.me.Email); err != nil {
		t.Fatal(err)
	}
	if got := e.postSh(chat.ID, commitAuthor); !strings.Contains(got, "author="+e.me.Email+" committer="+e.me.Email) {
		t.Fatalf("after switching to the primary, commits as %q", got)
	}
	if e.chatRun(chat.ID).ID != segment {
		t.Fatal("the switch should apply within the running segment, not by starting a new one")
	}
	if _, err := e.runs.SetChatCommitEmail(e.ctx, e.ws, e.me.ID, chat.ID, "Ada@Work.example"); err != nil {
		t.Fatal(err)
	}
	setEmails()
	if got := e.postSh(chat.ID, commitAuthor); !strings.Contains(got, "author="+e.me.Email) || strings.Contains(got, "Work.example") {
		t.Fatalf("a removed address still authors commits: %q", got)
	}
}

// postSh has the fake harness run a shell command in the workspace and
// returns its reply (see cmd/fake-harness).
func (e env) postSh(id uuid.UUID, command string) string {
	e.t.Helper()
	return e.say(id, "sh: "+command).BodyMd
}

// Turning a capability on or off ends the segment, so the next message runs
// with the new spec; an unchanged persona keeps resuming.
func TestCapabilityChangeEndsSegment(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	if err := e.setOverride(agentdef.Override{Harness: agentdef.HarnessCodex, Model: "gpt-5.5"}); err != nil {
		t.Fatal(err)
	}
	ps := &personas.Service{Pool: e.pool}
	o := personas.Owner{WorkspaceID: e.ws, UserID: e.me.ID}
	p := e.persona("Artist", false, false)
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: "Art", PersonaID: &p.ID, Message: &runs.ChatMessageInput{Body: "one"}})
	if err != nil {
		t.Fatal(err)
	}
	e.until("first reply", func() bool { return e.chat(chat.ID).State == "idle" })
	first := e.chatRun(chat.ID)
	if reply := e.say(chat.ID, "two"); e.chatRun(chat.ID).ID != first.ID || !strings.Contains(reply.BodyMd, "resumed=true") {
		t.Fatal("an unchanged persona must keep its segment")
	}

	images := true
	if p, err = ps.Update(e.ctx, o, p.ID, personas.Patch{Revision: p.Revision, Images: &images}); err != nil {
		t.Fatal(err)
	}
	e.say(chat.ID, "three")
	second := e.chatRun(chat.ID)
	spec, _ := runs.Spec(second)
	if second.ID == first.ID || spec.HarnessOptions[agentdef.OptionImageGeneration] != "true" {
		t.Fatalf("turning images on kept the old segment: %v %+v", second.ID == first.ID, spec.HarnessOptions)
	}
	if old := e.runByID(first.ID); old.State != "completed" {
		t.Fatalf("old segment is %s", old.State)
	}
	var reason string
	if err := e.pool.QueryRow(e.ctx, `SELECT payload->>'reason' FROM run_events WHERE run_id=$1 AND payload->>'state'='chat_segment_ended'`, first.ID).Scan(&reason); err != nil || reason != "capabilities_changed" {
		t.Fatalf("segment end reason %q, %v", reason, err)
	}

	off := false
	if _, err = ps.Update(e.ctx, o, p.ID, personas.Patch{Revision: p.Revision, Images: &off}); err != nil {
		t.Fatal(err)
	}
	e.say(chat.ID, "four")
	third := e.chatRun(chat.ID)
	if spec, _ = runs.Spec(third); third.ID == second.ID || spec.HarnessOptions[agentdef.OptionImageGeneration] != "" {
		t.Fatalf("turning images off must take effect on the next message: %+v", spec.HarnessOptions)
	}
}

// putBlobs is an in-memory blob store for generated images.
type putBlobs struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (b *putBlobs) Put(_ context.Context, bucket, key string, r io.Reader, _ int64, _ string) error {
	data, err := io.ReadAll(r)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.m[bucket+"/"+key] = data
	return err
}
func (b *putBlobs) Get(_ context.Context, bucket, key string) (blob.Object, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	d, ok := b.m[bucket+"/"+key]
	if !ok {
		return blob.Object{}, blob.ErrNotFound
	}
	return blob.Object{Body: io.NopCloser(bytes.NewReader(d)), ContentLength: int64(len(d))}, nil
}
func (b *putBlobs) Delete(_ context.Context, bucket, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.m, bucket+"/"+key)
	return nil
}

// An image-generating chat's outbox is harvested after the turn: images are
// stored and bound to the assistant message as generated attachments; other
// files, links and extras past the limit are skipped.
func TestImageHarvest(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	if err := e.setOverride(agentdef.Override{Harness: agentdef.HarnessCodex, Model: "gpt-5.5"}); err != nil {
		t.Fatal(err)
	}
	blobs := &putBlobs{m: map[string][]byte{}}
	e.runner.Blobs, e.runner.Attachments, e.runner.Cfg.AttachmentBucket = blobs, blobs, "artifacts"
	p := e.persona("Artist", false, true)
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: "Art", PersonaID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	// 10 PNGs, then (sorted last) a text file, a link out of the workspace and a directory.
	script := `mkdir -p .lazyagents/outbox && cd .lazyagents/outbox && for i in 01 02 03 04 05 06 07 08 09 10; do printf '\211PNG\r\n\032\nimg%s' $i > p$i.png; done; echo hi > zz-notes.txt; ln -s /etc/hosts zz-link.png; mkdir zz-sub`
	reply := e.postSh(chat.ID, script)
	if !strings.Contains(reply, "err=<nil>") {
		t.Fatal(reply)
	}
	msgs, err := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	last := msgs[len(msgs)-1]
	atts, err := e.q.ListChatAttachmentsForMessages(e.ctx, []uuid.UUID{last.ID})
	if err != nil || last.Role != "assistant" {
		t.Fatalf("last message %+v, %v", last, err)
	}
	if len(atts) != 8 {
		t.Fatalf("%d attachments, want the 8 per-turn maximum", len(atts))
	}
	for i, a := range atts {
		wantName := "p0" + string(rune('1'+i)) + ".png"
		if a.Source != "generated" || a.ContentType != "image/png" || a.Filename != wantName || a.UserID != e.me.ID || a.MessageID == nil || *a.MessageID != last.ID {
			t.Fatalf("attachment %d = %+v", i, a)
		}
		if _, err := blobs.Get(e.ctx, "artifacts", a.BlobKey); err != nil {
			t.Fatalf("blob %s: %v", a.BlobKey, err)
		}
	}
	// Each one is in the person's Library too, sharing its attachment's object.
	lib, err := e.q.ListLibraryItems(e.ctx, store.ListLibraryItemsParams{WorkspaceID: e.ws, UserID: e.me.ID, RowLimit: 100})
	if err != nil || len(lib) != len(atts) {
		t.Fatalf("library %d items, %v", len(lib), err)
	}
	keys := map[string]bool{}
	for _, a := range atts {
		keys[a.BlobKey] = true
	}
	for _, l := range lib {
		if !keys[l.BlobKey] || l.Kind != "image" || l.Title != "Art" || l.ChatID == nil || *l.ChatID != chat.ID || l.PersonaID == nil || *l.PersonaID != p.ID || l.MessageID == nil || *l.MessageID != last.ID {
			t.Fatalf("library item %+v", l)
		}
	}
	// Harvested files leave the outbox; the rest stays (and was reported).
	outbox := filepath.Join(e.runner.Backend.WorkspaceDir(e.chatRun(chat.ID).ID.String()), ".lazyagents", "outbox")
	left := map[string]bool{}
	entries, _ := os.ReadDir(outbox)
	for _, en := range entries {
		left[en.Name()] = true
	}
	if len(left) != 5 || !left["p09.png"] || !left["p10.png"] || !left["zz-notes.txt"] || !left["zz-link.png"] || !left["zz-sub"] {
		t.Fatalf("outbox after harvest: %v", left)
	}
	// A Codex chat saw its own images in the turn: no follow-up turn.
	var followups int
	if err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM run_turns WHERE run_id=$1 AND trigger='image_followup'`, e.chatRun(chat.ID).ID).Scan(&followups); err != nil || followups != 0 {
		t.Fatalf("%d image follow-ups for a Codex chat, %v", followups, err)
	}
	var skipped string
	if err := e.pool.QueryRow(e.ctx, `SELECT payload->>'detail' FROM run_events WHERE run_id=$1 AND payload->>'state'='chat_images_skipped'`, e.chatRun(chat.ID).ID).Scan(&skipped); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"zz-link.png: not a regular file", "zz-sub: not a regular file", "p09.png: more than 8", "zz-notes.txt: more than 8"} {
		if !strings.Contains(skipped, want) {
			t.Fatalf("skip report missing %q: %s", want, skipped)
		}
	}
	images := 0
	for k := range blobs.m {
		if strings.HasPrefix(k, "artifacts/chat-attachments/") {
			images++
		}
	}
	if images != 8 {
		t.Fatalf("%d image blobs stored, want 8", images)
	}
}

// Generated images count toward their own per-person cap: past it nothing is
// harvested, the file stays in the outbox, and the run records why.
func TestImageHarvestQuota(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	if err := e.setOverride(agentdef.Override{Harness: agentdef.HarnessCodex, Model: "gpt-5.5"}); err != nil {
		t.Fatal(err)
	}
	blobs := &putBlobs{m: map[string][]byte{}}
	e.runner.Blobs, e.runner.Attachments, e.runner.Cfg.AttachmentBucket = blobs, blobs, "artifacts"
	p := e.persona("Artist", false, true)
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: "Art", PersonaID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.q.CreateGeneratedChatAttachment(e.ctx, store.CreateGeneratedChatAttachmentParams{WorkspaceID: e.ws, UserID: e.me.ID,
		Filename: "old.png", ContentType: "image/png", SizeBytes: runs.MaxGeneratedImageStorageBytes, BlobKey: "chat-attachments/old"}); err != nil {
		t.Fatal(err)
	}
	reply := e.postSh(chat.ID, `mkdir -p .lazyagents/outbox && printf '\211PNG\r\n\032\nimg' > .lazyagents/outbox/new.png`)
	if !strings.Contains(reply, "err=<nil>") {
		t.Fatal(reply)
	}
	last := e.lastAssistant(chat.ID)
	if atts, err := e.q.ListChatAttachmentsForMessages(e.ctx, []uuid.UUID{last.ID}); err != nil || len(atts) != 0 {
		t.Fatalf("attachments over the cap: %v, %v", atts, err)
	}
	var detail string
	if err := e.pool.QueryRow(e.ctx, `SELECT payload->>'detail' FROM run_events WHERE run_id=$1 AND payload->>'state'='chat_images_skipped'`, e.chatRun(chat.ID).ID).Scan(&detail); err != nil || detail != "quota" {
		t.Fatalf("skip reason %q, %v", detail, err)
	}
	if _, err := os.Stat(filepath.Join(e.runner.Backend.WorkspaceDir(e.chatRun(chat.ID).ID.String()), ".lazyagents", "outbox", "new.png")); err != nil {
		t.Fatalf("the unharvested image should stay in the outbox: %v", err)
	}
	for k := range blobs.m {
		if strings.HasPrefix(k, "artifacts/chat-attachments/") {
			t.Fatalf("image blob stored over the cap: %s", k)
		}
	}
}

// A reply keeps its svg-image fence only while its speaker has the capability.
func TestChatSVGFenceFollowsCapability(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	ps := &personas.Service{Pool: e.pool}
	o := personas.Owner{WorkspaceID: e.ws, UserID: e.me.ID}
	p := e.persona("Drafter", false, false)
	chat, err := e.runs.StartChat(e.ctx, e.ws, e.me.ID, runs.ChatInput{Project: "PAY", Title: "Svg", PersonaID: &p.ID, Message: &runs.ChatMessageInput{Body: "one"}})
	if err != nil {
		t.Fatal(err)
	}
	e.until("first reply", func() bool { return e.chat(chat.ID).State == "idle" })
	const reply = "```svg-image\n<svg/>\n```"
	q := store.New(e.pool)
	if got, err := runs.GateSVGFences(e.ctx, q, e.chat(chat.ID), reply); err != nil || got != "```svg\n<svg/>\n```" {
		t.Fatalf("without the capability: %q, %v", got, err)
	}
	on := true
	if _, err = ps.Update(e.ctx, o, p.ID, personas.Patch{Revision: p.Revision, SVG: &on}); err != nil {
		t.Fatal(err)
	}
	if got, err := runs.GateSVGFences(e.ctx, q, e.chat(chat.ID), reply); err != nil || got != reply {
		t.Fatalf("with the capability: %q, %v", got, err)
	}
}
