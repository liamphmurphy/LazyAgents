package runner_test

import (
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// A researcher is a real run: the runner claims it, runs one turn with no
// repository, ends it, and the finished job hands the write-up to the chat.
func TestResearchRunsEndAndHandTheWriteUpToTheChat(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	chat := e.startChat("Compare local TTS")
	e.until("first reply", func() bool { return e.chat(chat.ID).State == "idle" })
	msg, err := e.q.AppendChatMessage(e.ctx, store.AppendChatMessageParams{SessionID: chat.ID, Role: "assistant",
		BodyMd: "```research-plan\nLocal TTS\n- Quality | Which sounds best?\n- Latency | How fast?\n```", ContextRefs: []byte("[]"), Citations: []byte("[]")})
	if err != nil {
		t.Fatal(err)
	}
	job, err := e.runs.StartResearch(e.ctx, e.ws, e.me.ID, chat.ID, runs.ResearchInput{MessageID: msg.ID, Title: "Local TTS", MaxSources: 40, MaxMinutes: 30,
		Questions: []runs.ResearchQuestion{{Label: "Quality", Question: "Which sounds best?"}, {Label: "Latency", Question: "How fast?"}}})
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := e.q.ListResearchTasks(e.ctx, job.ID)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("%v %v", tasks, err)
	}
	if _, err := e.runs.SubmitResearchFindings(e.ctx, tasks[0].RunID, []runs.ResearchFinding{{Claim: "Kokoro is fast", Sources: []runs.ResearchSource{{URL: "https://example.com"}}}}); err != nil {
		t.Fatal(err)
	}
	state := func() string {
		j, err := e.q.GetResearchJob(e.ctx, store.GetResearchJobParams{ID: job.ID, ChatID: chat.ID})
		if err != nil {
			t.Fatal(err)
		}
		return j.State
	}
	// The fake reply never saves a report, so the job ends here, past the write-up.
	e.until("the write-up turn to end", func() bool {
		// The runner's sweep does this every 30 seconds.
		if err := e.runs.AdvanceResearch(e.ctx); err != nil {
			t.Fatal(err)
		}
		return state() == "failed" || state() == "done"
	})
	j, _ := e.q.GetResearchJob(e.ctx, store.GetResearchJobParams{ID: job.ID, ChatID: chat.ID})
	if j.State != "failed" || !strings.Contains(j.Error, "without saving the report") {
		t.Fatalf("job %+v", j)
	}
	after, err := e.q.ListResearchTasks(e.ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range after {
		if task.RunState != "completed" {
			t.Fatalf("researcher %s ended %s", task.Label, task.RunState)
		}
	}
	var prompt string
	if err := e.pool.QueryRow(e.ctx, `SELECT t.input_message FROM run_turns t JOIN chat_sessions c ON c.current_run_id = t.run_id WHERE c.id = $1 ORDER BY t.seq DESC LIMIT 1`, chat.ID).Scan(&prompt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "Kokoro is fast") {
		t.Fatalf("write-up prompt %q", prompt)
	}
}
