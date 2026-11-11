package runner_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/runner"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

func TestChatCompletionRetriesWithoutRerunningHarness(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	chat := e.startChat("Answer once")
	_, err := e.pool.Exec(e.ctx, `CREATE SEQUENCE fail_reply_once;
	CREATE FUNCTION reject_first_reply() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
	 IF NEW.role = 'assistant' AND nextval('fail_reply_once') = 1 THEN
	  RAISE EXCEPTION 'injected completion failure';
	 END IF;
	 RETURN NEW; END $$;
	CREATE TRIGGER reject_first_reply BEFORE INSERT ON chat_messages
	FOR EACH ROW EXECUTE FUNCTION reject_first_reply()`)
	if err != nil {
		t.Fatal(err)
	}
	e.runner.Tick(e.ctx)
	e.runner.Wait()
	e.runner.Tick(e.ctx)
	e.runner.Wait()
	if c := e.chat(chat.ID); c.State != "idle" {
		t.Fatalf("completion failure stranded chat: %s", c.State)
	}
	turns, err := e.q.ListTurns(e.ctx, *chat.CurrentRunID)
	if err != nil || len(turns) != 1 || turns[0].Status != "done" {
		t.Fatalf("completion reran the harness or stranded a turn: %+v, %v", turns, err)
	}
	msgs, err := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 20})
	if err != nil || len(msgs) != 2 || msgs[1].Role != "assistant" {
		t.Fatalf("completion did not store exactly one reply: %+v, %v", msgs, err)
	}
}

func TestTickRecoversChatAfterOldHeartbeatExpires(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	chat := e.startChat("Continue after a crash")
	e.runner.Tick(e.ctx)
	e.runner.Wait() // The abandoned turn had a fully prepared workspace.
	run := e.chatRun(chat.ID)
	if _, err := e.pool.Exec(e.ctx, `UPDATE runs SET state='running',runner_id='old-runner' WHERE id=$1`, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(e.ctx, `UPDATE run_turns SET status='running',process_started_at=now() WHERE run_id=$1`, run.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.q.Heartbeat(e.ctx, store.HeartbeatParams{ID: "old-runner", Backend: "fake", MaxConcurrent: 2, Version: "test"}); err != nil {
		t.Fatal(err)
	}
	if err := e.runner.Recover(e.ctx); err != nil {
		t.Fatal(err)
	}
	turns, _ := e.q.ListTurns(e.ctx, run.ID)
	if turns[0].Status != "running" {
		t.Fatal("recovered work while its owner was still live")
	}
	if _, err := e.pool.Exec(e.ctx, `UPDATE runners SET heartbeat_at=now()-interval '60 seconds' WHERE id='old-runner'`); err != nil {
		t.Fatal(err)
	}
	e.runner.Tick(e.ctx)
	e.runner.Wait()
	if c := e.chat(chat.ID); c.State != "idle" {
		t.Fatalf("chat did not resume after the old heartbeat expired: %s", c.State)
	}
}

func TestExpiredTurnLeaseRecoversEvenWhileRunnerIsLive(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	chat := e.startChat("Recover a vanished worker")
	e.runner.Tick(e.ctx)
	e.runner.Wait()
	old, expired := uuid.New(), time.Now().Add(-time.Second)
	turn, err := e.q.ClaimPendingTurn(e.ctx, store.ClaimPendingTurnParams{ExecutionToken: &old, ExecutionExpiresAt: &expired})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.q.Heartbeat(e.ctx, store.HeartbeatParams{ID: e.runner.Cfg.ID, Backend: "fake", MaxConcurrent: 2, Version: "test"}); err != nil {
		t.Fatal(err)
	}
	e.runner.Tick(e.ctx)
	e.runner.Wait()
	got, err := e.q.GetTurn(e.ctx, turn.ID)
	if err != nil || got.Status != "done" || got.ExecutionToken == nil || *got.ExecutionToken == old || e.chat(chat.ID).State != "idle" {
		t.Fatalf("orphaned lease was not reclaimed: %+v, %v", got, err)
	}
}

func TestLiveTurnLeaseSurvivesAnotherRunnersRecovery(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	chat := e.startChat("Do not reclaim this worker")
	e.runner.Tick(e.ctx)
	e.runner.Wait()
	token, expires := uuid.New(), time.Now().Add(time.Minute)
	turn, err := e.q.ClaimPendingTurn(e.ctx, store.ClaimPendingTurnParams{ExecutionToken: &token, ExecutionExpiresAt: &expires})
	if err != nil {
		t.Fatal(err)
	}
	// No runner heartbeat exists. The worker's own lease is authoritative.
	e.runner.Cfg.ID = "replacement"
	if err := e.runner.Recover(e.ctx); err != nil {
		t.Fatal(err)
	}
	e.runner.Tick(e.ctx)
	e.runner.Wait()
	got, err := e.q.GetTurn(e.ctx, turn.ID)
	if err != nil || got.Status != "running" || got.ExecutionToken == nil || *got.ExecutionToken != token {
		t.Fatalf("recovered a live execution: %+v, %v", got, err)
	}
	if e.chat(chat.ID).State != "running" {
		t.Fatal("live chat changed state")
	}
}

func TestPersistentCompletionFailureLeavesChatRetryable(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	chat := e.startChat("Answer")
	_, err := e.pool.Exec(e.ctx, `CREATE FUNCTION reject_replies() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
	 IF NEW.role = 'assistant' THEN RAISE EXCEPTION 'injected completion failure'; END IF;
	 RETURN NEW; END $$;
	CREATE TRIGGER reject_replies BEFORE INSERT ON chat_messages FOR EACH ROW EXECUTE FUNCTION reject_replies()`)
	if err != nil {
		t.Fatal(err)
	}
	e.runner.Tick(e.ctx)
	e.runner.Wait()
	e.runner.Tick(e.ctx)
	e.runner.Wait()
	if e.chat(chat.ID).State != "failed" || e.chatRun(chat.ID).State != "waiting" {
		t.Fatal("failed completion left chat busy")
	}
	if _, err := e.pool.Exec(e.ctx, "DROP TRIGGER reject_replies ON chat_messages"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.runs.PostChatMessage(e.ctx, e.ws, e.me.ID, chat.ID, runs.ChatMessageInput{Body: "Try again"}); err != nil {
		t.Fatal(err)
	}
	e.until("retry reply", func() bool { return e.chat(chat.ID).State == "idle" })
}

type setupScriptBackend struct {
	runner.Backend
	script  string
	started chan runner.Process
}

func (b *setupScriptBackend) Start(ctx context.Context, id string, spec runner.ExecSpec) (runner.Process, error) {
	setup := strings.HasPrefix(spec.Name, "lazyagents-setup-")
	if setup {
		spec.Argv = []string{"sh", "-c", b.script}
	}
	p, err := b.Backend.Start(ctx, id, spec)
	if err == nil && setup {
		b.started <- p
	}
	return p, err
}

func TestChatSetupDrainsOutputPastCaptureLimit(t *testing.T) {
	e := setup(t)
	b := &setupScriptBackend{Backend: e.runner.Backend, script: "head -c 2097152 /dev/zero", started: make(chan runner.Process, 1)}
	e.runner.Backend = b
	e.startChat("Setup output")
	e.runner.Tick(e.ctx)
	p := <-b.started
	defer p.Kill()
	done := make(chan struct{})
	go func() { e.runner.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = p.Kill()
		<-done
		t.Fatal("setup blocked writing output after the capture limit")
	}
}

func TestChatStopKillsWorkspaceSetup(t *testing.T) {
	e := setup(t)
	b := &setupScriptBackend{Backend: e.runner.Backend, script: "sleep 30", started: make(chan runner.Process, 1)}
	e.runner.Backend = b
	chat := e.startChat("Stop setup")
	e.runner.Tick(e.ctx)
	p := <-b.started
	defer p.Kill()
	if _, err := e.runs.CancelChat(e.ctx, e.ws, e.me.ID, chat.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { e.runner.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = p.Kill()
		<-done
		t.Fatal("Stop did not kill workspace setup")
	}
	if c := e.chat(chat.ID); c.State != "idle" {
		t.Fatalf("stopped setup left chat %s", c.State)
	}
}

func TestChatSetupTimeoutLeavesChatUsable(t *testing.T) {
	e := setup(t)
	old := runner.ScriptTimeout
	runner.ScriptTimeout = 150 * time.Millisecond
	t.Cleanup(func() { runner.ScriptTimeout = old })
	b := &setupScriptBackend{Backend: e.runner.Backend, script: "sleep 30", started: make(chan runner.Process, 1)}
	e.runner.Backend = b
	chat := e.startChat("Setup timeout")
	e.runner.Tick(e.ctx)
	p := <-b.started
	defer p.Kill()
	done := make(chan struct{})
	go func() { e.runner.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = p.Kill()
		<-done
		t.Fatal("setup had no runtime deadline")
	}
	if e.chat(chat.ID).State != "failed" || e.chatRun(chat.ID).State != "waiting" {
		t.Fatal("setup timeout left chat busy")
	}
}

func TestRecoveryDeliversMessageThatInterruptedDeadTurn(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	chat := e.startChat("Original question")
	e.runner.Tick(e.ctx)
	e.runner.Wait()
	token, expires := uuid.New(), time.Now().Add(time.Minute)
	turn, err := e.q.ClaimPendingTurn(e.ctx, store.ClaimPendingTurnParams{ExecutionToken: &token, ExecutionExpiresAt: &expires})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.runs.PostChatMessage(e.ctx, e.ws, e.me.ID, chat.ID, runs.ChatMessageInput{Body: "Interrupting question"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(e.ctx, "UPDATE run_turns SET execution_expires_at=now()-interval '1 second' WHERE id=$1", turn.ID); err != nil {
		t.Fatal(err)
	}
	e.until("interrupted reply recovered", func() bool { return e.chat(chat.ID).State == "idle" })
	msgs, err := e.q.ListChatMessages(e.ctx, store.ListChatMessagesParams{SessionID: chat.ID, Limit: 10})
	if err != nil || len(msgs) != 3 || msgs[2].Role != "assistant" || !strings.Contains(msgs[2].BodyMd, "Interrupting question") {
		t.Fatalf("recovery lost interrupted message: %+v, %v", msgs, err)
	}
}
