package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

func (e *chatEnv) courseRuns(t *testing.T, course, role string) []uuid.UUID {
	t.Helper()
	rows, err := e.pool.Query(t.Context(), `SELECT l.run_id FROM learning_runs l JOIN runs r ON r.id = l.run_id
		WHERE l.course_id = $1 AND l.role = $2 AND r.state NOT IN ('completed', 'failed', 'cancelled') ORDER BY l.unit_seq`, course, role)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out = append(out, id)
	}
	return out
}

func (e *chatEnv) finishCourseRuns(t *testing.T, ids []uuid.UUID) {
	t.Helper()
	for _, id := range ids {
		e.setRunState(t, id, "completed")
	}
	if err := runs.AdvanceResearchJobs(t.Context(), e.pool); err != nil {
		t.Fatal(err)
	}
	if err := runs.AdvanceLearningCourses(t.Context(), e.pool); err != nil {
		t.Fatal(err)
	}
}

func (e *chatEnv) courseState(t *testing.T, id string) map[string]any {
	t.Helper()
	code, c := e.do("GET", "/learn/courses/"+id, nil)
	if code != http.StatusOK {
		t.Fatalf("get: %d %v", code, c)
	}
	return c
}

func wantCourseState(t *testing.T, c map[string]any, want string) {
	t.Helper()
	if got := c["summary"].(map[string]any)["state"]; got != want {
		t.Fatalf("state = %v, want %s (%v)", got, want, c["summary"])
	}
}

func courseOutline() runs.CourseOutline {
	return runs.CourseOutline{Summary: "MVCC", Glossary: []runs.CourseTerm{{Term: "tuple", Definition: "a row version"}}, Units: []runs.CourseUnit{
		{Title: "Basics", Question: "What are tuples?", Lessons: []runs.CourseLesson{
			{Key: "known-one", Title: "Known", Objectives: []string{"Know it"}},
			{Key: "tuples", Title: "Tuples", Objectives: []string{"Find a tuple"}, Prereqs: []string{"known-one"}},
		}},
		{Title: "Vacuum", Question: "What does vacuum do?", Lessons: []runs.CourseLesson{
			{Key: "vacuum", Title: "Vacuum", Objectives: []string{"Say what vacuum does"}, Prereqs: []string{"tuples"}},
		}},
	}}
}

func courseLesson(key, objective string) runs.LessonSubmission {
	return runs.LessonSubmission{Key: key, Blocks: []runs.LessonBlock{{Kind: "text", Text: "A claim about " + key, Provenance: "sourced", Sources: []int{1}}, {Kind: "text", Text: "An analogy", Provenance: "model_knowledge"}},
		Checks: []runs.LessonCheckInput{{Objective: objective, Kind: "mcq", Prompt: "Which?", Options: []string{"secretright", "wrong one", "wrong two"}, Answer: "secretright", Explanation: "secretexplanation"}}}
}

// A module goes the whole way: scoped, edited, approved, researched, written,
// reviewed with one rewrite, then studied without the key reaching the browser early.
func TestCourseLifecycle(t *testing.T) {
	e := newChatEnv(t)
	ctx := context.Background()

	code, created := e.do("POST", "/learn/courses", map[string]any{"project": "PAY", "topic": "Postgres MVCC", "level": "some", "minutes_budget": 180, "harness": "claude-code"})
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, created)
	}
	id := created["summary"].(map[string]any)["id"].(string)
	wantCourseState(t, created, "scoping")
	chat := uuid.MustParse(created["chat_id"].(string))

	// The tutor chat doesn't appear among the person's chats.
	var hidden bool
	if err := e.pool.QueryRow(ctx, `SELECT hidden FROM chat_sessions WHERE id = $1`, chat).Scan(&hidden); err != nil || !hidden {
		t.Fatalf("hidden = %v, %v", hidden, err)
	}

	scope := e.courseRuns(t, id, "scope")
	if len(scope) != 1 {
		t.Fatalf("scope runs: %v", scope)
	}
	var spec string
	if err := e.pool.QueryRow(ctx, `SELECT resolved_spec::text FROM runs WHERE id = $1`, scope[0]).Scan(&spec); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(spec, `"web_search":"true"`) || strings.Contains(spec, `"web_search": "true"`) {
		t.Fatalf("a scoping run has no web: %s", spec)
	}
	bad := courseOutline()
	bad.Units[0].Lessons[0], bad.Units[0].Lessons[1] = bad.Units[0].Lessons[1], bad.Units[0].Lessons[0]
	if _, err := e.runs.SubmitCourseOutline(ctx, scope[0], bad); err == nil || !strings.Contains(err.Error(), "can't come before") {
		t.Fatalf("bad order: %v", err)
	}
	if _, err := e.runs.SubmitCourseOutline(ctx, scope[0], courseOutline()); err != nil {
		t.Fatal(err)
	}
	e.finishCourseRuns(t, scope)
	wantCourseState(t, e.courseState(t, id), "outline_ready")

	// A person's unsound reorder is refused and explained.
	code, res := e.do("PUT", "/learn/courses/"+id+"/outline", map[string]any{"known": []string{}, "units": []map[string]any{{"title": "Basics", "lessons": []string{"tuples", "known-one"}}, {"title": "Vacuum", "lessons": []string{"vacuum"}}}})
	if code != http.StatusUnprocessableEntity || !strings.Contains(fmt.Sprint(res), "can't come before") {
		t.Fatalf("reorder: %d %v", code, res)
	}
	if code, res = e.do("PUT", "/learn/courses/"+id+"/outline", map[string]any{"known": []string{"known-one"}, "units": []map[string]any{{"title": "Basics", "lessons": []string{"known-one", "tuples"}}, {"title": "Vacuum", "lessons": []string{"vacuum"}}}}); code != http.StatusOK {
		t.Fatalf("edit: %d %v", code, res)
	}
	if code, res = e.do("POST", "/learn/courses/"+id+"/approve", map[string]any{"max_sources": 20, "max_minutes": 15}); code != http.StatusOK {
		t.Fatalf("approve: %d %v", code, res)
	}
	wantCourseState(t, res, "researching")
	if code, _ = e.do("POST", "/learn/courses/"+id+"/approve", map[string]any{"max_sources": 20, "max_minutes": 15}); code != http.StatusConflict {
		t.Fatalf("approving twice: %d", code)
	}

	course, err := e.q.LockLearningCourse(ctx, uuid.MustParse(id))
	if err != nil {
		t.Fatal(err)
	}
	researchers := e.researchRuns(t, course.ResearchJobID.String())
	if len(researchers) != 2 {
		t.Fatalf("researchers: %v", researchers)
	}
	for _, r := range researchers {
		if _, err := e.runs.SubmitResearchFindings(ctx, r, []runs.ResearchFinding{{Claim: "c", Sources: []runs.ResearchSource{{Title: "Docs", URL: "https://example.com/docs"}}}}); err != nil {
			t.Fatal(err)
		}
	}
	e.finishCourseRuns(t, researchers)
	wantCourseState(t, e.courseState(t, id), "authoring")
	// A course job doesn't ask a persona for a report, and isn't shown in a chat.
	code, list := e.do("GET", "/chats/"+chat.String()+"/research", nil)
	if code != http.StatusOK || len(list["jobs"].([]any)) != 0 {
		t.Fatalf("chat research: %d %v", code, list)
	}

	authors := e.courseRuns(t, id, "author")
	if len(authors) != 2 {
		t.Fatalf("authors: %v", authors)
	}
	invented := courseLesson("tuples", "Find a tuple")
	invented.Blocks[0].Sources = []int{7}
	if err := e.runs.SubmitCourseLesson(ctx, authors[0], invented); err == nil {
		t.Fatal("an invented source must be refused")
	}
	if err := e.runs.SubmitCourseLesson(ctx, authors[1], courseLesson("tuples", "Find a tuple")); err == nil {
		t.Fatal("another unit's lesson must be refused")
	}
	if err := e.runs.SubmitCourseLesson(ctx, authors[0], courseLesson("tuples", "Find a tuple")); err != nil {
		t.Fatal(err)
	}
	if err := e.runs.SubmitCourseLesson(ctx, authors[1], courseLesson("vacuum", "Say what vacuum does")); err != nil {
		t.Fatal(err)
	}
	e.finishCourseRuns(t, authors)
	wantCourseState(t, e.courseState(t, id), "verifying")

	reviewers := e.courseRuns(t, id, "review")
	if len(reviewers) != 2 {
		t.Fatalf("reviewers: %v", reviewers)
	}
	// A reviewer may open only what the course cites, and can't search.
	agent, _ := agents.EnsureAssistant(ctx, e.q, e.me.WorkspaceID)
	e.setRunState(t, reviewers[0], "running")
	caller := runs.Caller{Agent: agent, RunID: &reviewers[0]}
	for url, want := range map[string]string{"https://example.com/docs": "allow", "https://evil.example/?x=secret": "deny"} {
		d, err := e.runs.CheckTool(ctx, caller, runs.ToolCheck{Tool: "WebFetch", Input: map[string]any{"url": url}})
		if err != nil || d.Decision != want {
			t.Fatalf("%s: %+v %v", url, d, err)
		}
	}
	if d, err := e.runs.CheckTool(ctx, caller, runs.ToolCheck{Tool: "WebSearch", Input: map[string]any{"query": "x"}}); err != nil || d.Decision != "deny" {
		t.Fatalf("search: %+v %v", d, err)
	}

	if err := e.runs.SubmitCourseReviews(ctx, reviewers[0], []runs.LessonReview{{Key: "tuples", Verdict: "pass"}}); err != nil {
		t.Fatal(err)
	}
	if err := e.runs.SubmitCourseReviews(ctx, reviewers[1], []runs.LessonReview{{Key: "vacuum", Verdict: "fail"}}); err == nil {
		t.Fatal("a failure needs its issues")
	}
	if err := e.runs.SubmitCourseReviews(ctx, reviewers[1], []runs.LessonReview{{Key: "vacuum", Verdict: "fail", Issues: []string{"Wrong claim"}}}); err != nil {
		t.Fatal(err)
	}
	e.finishCourseRuns(t, reviewers)
	wantCourseState(t, e.courseState(t, id), "authoring")

	// The one rewrite covers only what failed, and sees why.
	rewrite := e.courseRuns(t, id, "author")
	if len(rewrite) != 1 {
		t.Fatalf("rewrite: %v", rewrite)
	}
	var prompt string
	if err := e.pool.QueryRow(ctx, `SELECT input_message FROM run_turns WHERE run_id = $1`, rewrite[0]).Scan(&prompt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "Wrong claim") {
		t.Fatalf("rewrite prompt: %s", prompt)
	}
	if err := e.runs.SubmitCourseLesson(ctx, rewrite[0], courseLesson("tuples", "Find a tuple")); err == nil {
		t.Fatal("a verified lesson can't be rewritten")
	}
	if err := e.runs.SubmitCourseLesson(ctx, rewrite[0], courseLesson("vacuum", "Say what vacuum does")); err != nil {
		t.Fatal(err)
	}
	e.finishCourseRuns(t, rewrite)
	second := e.courseRuns(t, id, "review")
	if err := e.runs.SubmitCourseReviews(ctx, second[0], []runs.LessonReview{{Key: "vacuum", Verdict: "fail", Issues: []string{"Still wrong"}}}); err != nil {
		t.Fatal(err)
	}
	e.finishCourseRuns(t, second)

	// Ready: the lesson that failed twice stays, flagged with why.
	c := e.courseState(t, id)
	wantCourseState(t, c, "ready")
	raw, _ := json.Marshal(c)
	if strings.Contains(string(raw), "secretexplanation") {
		t.Fatal("the explanation must not be sent before an attempt")
	}
	lessons := map[string]map[string]any{}
	for _, l := range c["lessons"].([]any) {
		m := l.(map[string]any)
		lessons[m["key"].(string)] = m
	}
	if lessons["known-one"]["status"] != "known" || lessons["tuples"]["status"] != "verified" || lessons["vacuum"]["status"] != "flagged" {
		t.Fatalf("statuses: %v", lessons)
	}
	if issues := lessons["vacuum"]["issues"].([]any); len(issues) != 1 || issues[0] != "Still wrong" {
		t.Fatalf("issues: %v", issues)
	}
	summary := c["summary"].(map[string]any)
	if summary["lessons"].(float64) != 2 || summary["flagged"].(float64) != 1 {
		t.Fatalf("summary: %v", summary)
	}
	// Vacuum depends on tuples, which isn't done yet.
	if shaky := lessons["vacuum"]["shaky_prereqs"].([]any); len(shaky) != 1 || shaky[0] != "Tuples" {
		t.Fatalf("shaky: %v", shaky)
	}

	check := lessons["tuples"]["checks"].([]any)[0].(map[string]any)["id"].(string)
	attempt := func(answer string) map[string]any {
		code, out := e.do("POST", "/learn/courses/"+id+"/checks/"+check+"/attempts", map[string]any{"response": []string{answer}, "confidence": 2})
		if code != http.StatusCreated {
			t.Fatalf("attempt: %d %v", code, out)
		}
		return out
	}
	if out := attempt("wrong one"); out["correct"] != false || out["reveal"] != nil {
		t.Fatalf("first miss: %v", out)
	}
	if out := attempt("secretright"); out["correct"] != true || out["reveal"] == nil {
		t.Fatalf("right: %v", out)
	}
	c = e.courseState(t, id)
	for _, l := range c["lessons"].([]any) {
		if m := l.(map[string]any); m["key"] == "tuples" && m["progress"] != "complete" {
			t.Fatalf("progress: %v", m["progress"])
		}
	}

	// The tutor gets the lesson it's being asked about.
	if code, res = e.do("POST", "/learn/courses/"+id+"/tutor", map[string]any{"body": "why?", "lesson": "nope"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown lesson: %d %v", code, res)
	}
	if code, res = e.do("POST", "/learn/courses/"+id+"/tutor", map[string]any{"body": "why?", "lesson": "tuples"}); code != http.StatusAccepted {
		t.Fatalf("tutor: %d %v", code, res)
	}
	session, err := e.q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat, WorkspaceID: e.me.WorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	if can, err := runs.ChatCanDrawSVGs(ctx, e.q, session); err != nil || !can {
		t.Fatalf("a tutor should draw whichever persona it has: %v, %v", can, err)
	}
	note, err := runs.LearningTutorNote(ctx, e.q, session)
	if err != nil || !strings.Contains(note, "A claim about tuples") || !strings.Contains(note, "got this check wrong") {
		t.Fatalf("note: %q %v", note, err)
	}

	if code, _ = e.do("DELETE", "/learn/courses/"+id, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code, _ = e.do("GET", "/learn/courses/"+id, nil); code != http.StatusNotFound {
		t.Fatalf("after delete: %d", code)
	}
}

// Stopping a build ends every run it has going.
func TestCourseStop(t *testing.T) {
	e := newChatEnv(t)
	code, created := e.do("POST", "/learn/courses", map[string]any{"project": "PAY", "topic": "T", "level": "new", "minutes_budget": 60, "harness": "claude-code"})
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, created)
	}
	id := created["summary"].(map[string]any)["id"].(string)
	if code, res := e.do("POST", "/learn/courses/"+id+"/stop", nil); code != http.StatusOK {
		t.Fatalf("stop: %d %v", code, res)
	}
	if left := e.courseRuns(t, id, "scope"); len(left) != 0 {
		t.Fatalf("still live: %v", left)
	}
	if code, _ := e.do("POST", "/learn/courses/"+id+"/stop", nil); code != http.StatusConflict {
		t.Fatalf("stopping twice: %d", code)
	}
}

// One person can't see another's module.
func TestCourseMissing(t *testing.T) {
	e := newChatEnv(t)
	if code, _ := e.do("GET", "/learn/courses/"+uuid.NewString(), nil); code != http.StatusNotFound {
		t.Fatalf("got %d", code)
	}
	if code, _ := e.do("POST", "/learn/courses", map[string]any{"project": "PAY", "topic": " ", "level": "new", "minutes_budget": 60}); code != http.StatusUnprocessableEntity {
		t.Fatalf("blank topic: %d", code)
	}
}
