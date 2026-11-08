package runs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Guided Learning: a person gives a topic, one run drafts an outline for them
// to approve, then Vesper runs research, authoring and an independent review
// per unit. Vesper does the coordinating, as for deep research, so limits hold
// and a build can always be stopped.

const (
	PurposeLearning = "learning"

	learningRoleScope, learningRoleAuthor, learningRoleReview = "scope", "author", "review"

	learningMaxUnits, learningMaxLessons = 5, 20
	learningMaxObjectives                = 4
	learningMaxGlossary                  = 60
)

var learningKeyRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// CourseOutline is the module's shape: what gets approved before research.
type CourseOutline struct {
	Summary  string       `json:"summary" jsonschema:"What the module covers and who it's for, in one or two sentences"`
	Glossary []CourseTerm `json:"glossary" jsonschema:"The module's key terms, so every unit uses them the same way"`
	Units    []CourseUnit `json:"units" jsonschema:"At most 5 units, in teaching order"`
}

// CourseTerm is one glossary entry.
type CourseTerm struct {
	Term       string `json:"term"`
	Definition string `json:"definition"`
}

// CourseUnit is a group of lessons and the question researched for them.
type CourseUnit struct {
	Title    string         `json:"title" jsonschema:"Short unit title, unique in the module"`
	Question string         `json:"question" jsonschema:"What a web researcher should find out to ground this unit"`
	Lessons  []CourseLesson `json:"lessons"`
}

// CourseLesson is one concept in the prerequisite graph.
type CourseLesson struct {
	Key        string   `json:"key" jsonschema:"Lowercase slug, unique in the module, e.g. row-versions"`
	Title      string   `json:"title"`
	Objectives []string `json:"objectives" jsonschema:"1 to 4 things the learner will be able to do, each checkable by a question"`
	Prereqs    []string `json:"prereqs" jsonschema:"Keys of lessons this one directly depends on; each must come earlier"`
	Minutes    int      `json:"minutes" jsonschema:"Estimated minutes to study it"`
	Known      bool     `json:"known,omitempty" jsonschema:"Leave unset: the person marks lessons they already know"`
}

// Validate is the soundness check for an outline, from a model or a person's
// edit. Requiring each prerequisite to come earlier also rules out cycles.
func (o *CourseOutline) Validate() error {
	o.Summary = strings.TrimSpace(o.Summary)
	if o.Summary == "" {
		return apperr.Invalid("summary", "required")
	}
	if len(o.Glossary) > learningMaxGlossary {
		return apperr.Invalid("glossary", "at most %d terms", learningMaxGlossary)
	}
	if len(o.Units) == 0 || len(o.Units) > learningMaxUnits {
		return apperr.Invalid("units", "give 1 to %d units", learningMaxUnits)
	}
	seen, titles, total := map[string]bool{}, map[string]bool{}, 0
	for i := range o.Units {
		u := &o.Units[i]
		u.Title, u.Question = strings.TrimSpace(u.Title), strings.TrimSpace(u.Question)
		if u.Title == "" || utf8.RuneCountInString(u.Title) > 80 || titles[u.Title] {
			return apperr.Invalid("units", "each unit needs a unique title of up to 80 characters: %q", u.Title)
		}
		titles[u.Title] = true
		if u.Question == "" || utf8.RuneCountInString(u.Question) > 2000 {
			return apperr.Invalid("units", "unit %q needs a research question of up to 2000 characters", u.Title)
		}
		if len(u.Lessons) == 0 {
			return apperr.Invalid("units", "unit %q has no lessons", u.Title)
		}
		for j := range u.Lessons {
			l := &u.Lessons[j]
			l.Title = strings.TrimSpace(l.Title)
			if !learningKeyRe.MatchString(l.Key) {
				return apperr.Invalid("lessons", "%q isn't a valid key: use a lowercase slug", l.Key)
			}
			if seen[l.Key] {
				return apperr.Invalid("lessons", "the key %q is used twice", l.Key)
			}
			if l.Title == "" || utf8.RuneCountInString(l.Title) > 120 {
				return apperr.Invalid("lessons", "lesson %q needs a title of up to 120 characters", l.Key)
			}
			if len(l.Objectives) == 0 || len(l.Objectives) > learningMaxObjectives {
				return apperr.Invalid("lessons", "lesson %q needs 1 to %d objectives", l.Key, learningMaxObjectives)
			}
			for k := range l.Objectives {
				l.Objectives[k] = strings.TrimSpace(l.Objectives[k])
				if l.Objectives[k] == "" {
					return apperr.Invalid("lessons", "lesson %q has an empty objective", l.Key)
				}
			}
			if l.Prereqs == nil {
				l.Prereqs = []string{}
			}
			if l.Minutes <= 0 {
				l.Minutes = 10
			}
			for _, p := range l.Prereqs {
				if p == l.Key {
					return apperr.Invalid("lessons", "lesson %q can't depend on itself", l.Key)
				}
				if !seen[p] {
					if o.hasLesson(p) {
						return apperr.Invalid("order", "%q can't come before %q: it depends on it", l.Title, o.lessonTitle(p))
					}
					return apperr.Invalid("lessons", "lesson %q depends on %q, which isn't in the outline", l.Key, p)
				}
			}
			seen[l.Key] = true
			total++
		}
	}
	if total > learningMaxLessons {
		return apperr.Invalid("lessons", "at most %d lessons, not %d", learningMaxLessons, total)
	}
	return nil
}

func (o *CourseOutline) find(key string) *CourseLesson {
	for i := range o.Units {
		for j := range o.Units[i].Lessons {
			if o.Units[i].Lessons[j].Key == key {
				return &o.Units[i].Lessons[j]
			}
		}
	}
	return nil
}

func (o *CourseOutline) hasLesson(key string) bool { return o.find(key) != nil }

func (o *CourseOutline) lessonTitle(key string) string {
	if l := o.find(key); l != nil {
		return l.Title
	}
	return key
}

// activeUnits are the indexes of units with something left to teach; research
// task seq i belongs to activeUnits[i].
func (o *CourseOutline) activeUnits() []int {
	out := []int{}
	for i, u := range o.Units {
		if slices.ContainsFunc(u.Lessons, func(l CourseLesson) bool { return !l.Known }) {
			out = append(out, i)
		}
	}
	return out
}

// OutlineEdit is the person's arrangement of an outline: which lessons remain,
// in which units and order, and which they already know.
type OutlineEdit struct {
	Units []OutlineEditUnit
	Known []string
}

// OutlineEditUnit names an existing unit and its lessons' keys, in order.
type OutlineEditUnit struct {
	Title   string
	Lessons []string
}

// Apply rearranges o. Nothing new can come in through an edit: only units and
// lessons the outline already has.
func (o *CourseOutline) Apply(e OutlineEdit) (CourseOutline, error) {
	out := CourseOutline{Summary: o.Summary, Glossary: o.Glossary, Units: []CourseUnit{}}
	for _, eu := range e.Units {
		i := slices.IndexFunc(o.Units, func(u CourseUnit) bool { return u.Title == eu.Title })
		if i < 0 {
			return out, apperr.Invalid("units", "there's no unit called %q", eu.Title)
		}
		if len(eu.Lessons) == 0 {
			continue
		}
		u := CourseUnit{Title: o.Units[i].Title, Question: o.Units[i].Question, Lessons: []CourseLesson{}}
		for _, key := range eu.Lessons {
			l := o.find(key)
			if l == nil {
				return out, apperr.Invalid("units", "there's no lesson %q", key)
			}
			next := *l
			next.Known = slices.Contains(e.Known, key)
			u.Lessons = append(u.Lessons, next)
		}
		out.Units = append(out.Units, u)
	}
	for _, key := range e.Known {
		if !out.hasLesson(key) {
			return out, apperr.Invalid("known", "there's no lesson %q", key)
		}
	}
	return out, out.Validate()
}

// LearningSpec freezes what a course run may do: nothing in the workspace, no
// memory, and no web except a Claude Code reviewer opening the course's own
// sources (CheckTool holds it to them; Codex can't be restricted that way).
func LearningSpec(base agentdef.Resolved, harness, model, effort, role string, minutes int) agentdef.Resolved {
	base.Harness, base.Model = harness, model
	r := ChatSpec(base, ChatCaps{})
	r.Fallbacks = []agentdef.Fallback{}
	r.Repository = nil
	r.Memory.Enabled = false
	r.MaxConcurrency = researchParallel
	r.Budget = agentdef.Budget{MaxRuntime: fmt.Sprintf("%dm", minutes), MaxTurns: 4}
	r.Instructions = learningInstructions[role]
	if !(role == learningRoleReview && harness == agentdef.HarnessClaudeCode) {
		delete(r.HarnessOptions, agentdef.OptionWebSearch)
	}
	if effort != "" {
		r.HarnessOptions["effort"] = effort
	}
	return r
}

// CourseInput starts a module.
type CourseInput struct {
	Project, Topic, Goal, Level string
	MinutesBudget               int
	PersonaID                   *uuid.UUID
	Harness, Model, Effort      string
}

func (in *CourseInput) validate() error {
	in.Topic, in.Goal = strings.TrimSpace(in.Topic), strings.TrimSpace(in.Goal)
	if in.Topic == "" || utf8.RuneCountInString(in.Topic) > 200 {
		return apperr.Invalid("topic", "give a topic of up to 200 characters")
	}
	if utf8.RuneCountInString(in.Goal) > 1000 {
		return apperr.Invalid("goal", "keep the goal to 1000 characters")
	}
	if !slices.Contains([]string{"new", "some", "experienced"}, in.Level) {
		return apperr.Invalid("level", "must be new, some or experienced")
	}
	if in.MinutesBudget < 15 || in.MinutesBudget > 1200 {
		return apperr.Invalid("minutes_budget", "must be between 15 and 1200")
	}
	return nil
}

// learningEnv is what queuing a course's runs needs.
type learningEnv struct {
	chat  store.ChatSession
	agent store.Agent
	base  agentdef.Resolved
}

func loadLearningEnv(ctx context.Context, q *store.Queries, c store.LearningCourse) (learningEnv, error) {
	var e learningEnv
	var err error
	if e.chat, err = q.GetChatSession(ctx, store.GetChatSessionParams{ID: c.ChatID, WorkspaceID: c.WorkspaceID}); err != nil {
		return e, err
	}
	p, err := q.GetProject(ctx, store.GetProjectParams{ID: e.chat.ProjectID, WorkspaceID: c.WorkspaceID})
	if err != nil {
		return e, err
	}
	e.agent, e.base, err = assistantSpec(ctx, q, c.WorkspaceID, p.Key)
	return e, err
}

func (e learningEnv) queue(ctx context.Context, q *store.Queries, c store.LearningCourse, role string, round, unit int32, prompt string) error {
	spec := LearningSpec(e.base, e.chat.Harness, e.chat.Model, e.chat.Effort, role, int(c.MaxMinutes))
	if err := spec.Validate(); err != nil {
		return apperr.Invalid("harness", "%s", err.Error())
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	run, err := q.CreateRun(ctx, store.CreateRunParams{WorkspaceID: c.WorkspaceID, AgentID: e.agent.ID, Mode: "managed", State: "queued", ResolvedSpec: raw,
		DefinitionVersionID: e.agent.CurrentDefinitionVersionID, Harness: spec.Harness, Model: spec.Model, Purpose: PurposeLearning})
	if err != nil {
		return err
	}
	if err := q.SetRunDispatcher(ctx, store.SetRunDispatcherParams{ID: run.ID, DispatchedByUserID: &c.UserID}); err != nil {
		return err
	}
	if _, err := q.StartTurn(ctx, store.StartTurnParams{RunID: run.ID, Trigger: "start", InputMessage: prompt}); err != nil {
		return err
	}
	if err := q.CreateLearningRun(ctx, store.CreateLearningRunParams{RunID: run.ID, CourseID: c.ID, Role: role, Round: round, UnitSeq: unit}); err != nil {
		return err
	}
	return event(ctx, q, run.ID, "lifecycle", map[string]any{"state": "queued", "purpose": PurposeLearning, "course_id": c.ID})
}

// StartCourse creates the module, its hidden tutor chat and the scoping run.
func (s *Service) StartCourse(ctx context.Context, ws, user uuid.UUID, in CourseInput) (store.LearningCourse, error) {
	var c store.LearningCourse
	if err := in.validate(); err != nil {
		return c, err
	}
	err := s.tx(ctx, func(q *store.Queries) error {
		chat, err := s.startChat(ctx, q, ws, user, ChatInput{Project: in.Project, Title: "Tutor: " + firstLine(in.Topic, 80), Harness: in.Harness, Model: in.Model, Effort: in.Effort, PersonaID: in.PersonaID, Hidden: true, NoRepo: true})
		if err != nil {
			return err
		}
		if chat.Harness == agentdef.HarnessOpenCode {
			return apperr.Conflict("a module needs web research, which OpenCode doesn't have: choose Claude Code or Codex")
		}
		c, err = q.CreateLearningCourse(ctx, store.CreateLearningCourseParams{WorkspaceID: ws, UserID: user, ChatID: chat.ID, Topic: in.Topic, Goal: in.Goal, Level: in.Level, MinutesBudget: int32(in.MinutesBudget)})
		if err != nil {
			return err
		}
		e, err := loadLearningEnv(ctx, q, c)
		if err != nil {
			return err
		}
		return e.queue(ctx, q, c, learningRoleScope, 0, 0, learningScopePrompt(c))
	})
	return c, err
}

// lockOwnedCourse takes the chat then the course, the order every path uses.
func lockOwnedCourse(ctx context.Context, q *store.Queries, ws, user, id uuid.UUID) (store.LearningCourse, error) {
	c, err := q.GetLearningCourse(ctx, store.GetLearningCourseParams{ID: id, WorkspaceID: ws, UserID: user})
	if errors.Is(err, pgx.ErrNoRows) {
		return c, apperr.ErrNotFound
	} else if err != nil {
		return c, err
	}
	if _, err := lockChat(ctx, q, ws, user, c.ChatID); err != nil {
		return c, err
	}
	return q.LockLearningCourse(ctx, id)
}

func parseOutline(c store.LearningCourse) (CourseOutline, error) {
	var o CourseOutline
	if len(c.Outline) == 0 {
		return o, apperr.Conflict("this module has no outline yet")
	}
	return o, json.Unmarshal(c.Outline, &o)
}

// SubmitCourseOutline stores a scoping run's outline. The run may call it again
// after a refusal; the advance moves on once the run ends.
func (s *Service) SubmitCourseOutline(ctx context.Context, runID uuid.UUID, o CourseOutline) (int, error) {
	if err := o.Validate(); err != nil {
		return 0, err
	}
	total := 0
	for i := range o.Units {
		for j := range o.Units[i].Lessons {
			o.Units[i].Lessons[j].Known = false
			total++
		}
	}
	raw, err := json.Marshal(o)
	if err != nil {
		return 0, err
	}
	return total, s.tx(ctx, func(q *store.Queries) error {
		c, _, err := lockCourseForRun(ctx, q, runID, learningRoleScope)
		if err != nil {
			return err
		}
		if c.State != "scoping" {
			return apperr.Conflict("this module is past scoping")
		}
		_, err = q.SetLearningCourseOutline(ctx, store.SetLearningCourseOutlineParams{ID: c.ID, Outline: raw})
		return err
	})
}

func lockCourseForRun(ctx context.Context, q *store.Queries, runID uuid.UUID, role string) (store.LearningCourse, store.LearningRun, error) {
	var c store.LearningCourse
	r, err := q.GetLearningRun(ctx, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, r, apperr.Conflict("this tool only works in a course run")
	} else if err != nil {
		return c, r, err
	}
	if r.Role != role {
		return c, r, apperr.Conflict("this tool isn't for a %s run", r.Role)
	}
	c, err = q.LockLearningCourse(ctx, r.CourseID)
	return c, r, err
}

// EditCourseOutline applies the person's arrangement of an outline awaiting approval.
func (s *Service) EditCourseOutline(ctx context.Context, ws, user, id uuid.UUID, e OutlineEdit) (store.LearningCourse, error) {
	var c store.LearningCourse
	err := s.tx(ctx, func(q *store.Queries) error {
		var err error
		if c, err = lockOwnedCourse(ctx, q, ws, user, id); err != nil {
			return err
		}
		if c.State != "outline_ready" {
			return apperr.Conflict("the outline can only change before it's approved")
		}
		o, err := parseOutline(c)
		if err != nil {
			return err
		}
		next, err := o.Apply(e)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(next)
		if err != nil {
			return err
		}
		c, err = q.SetLearningCourseOutline(ctx, store.SetLearningCourseOutlineParams{ID: c.ID, Outline: raw})
		return err
	})
	return c, err
}

// ApproveCourse freezes the outline into lessons and starts its researchers.
func (s *Service) ApproveCourse(ctx context.Context, ws, user, id uuid.UUID, maxSources, maxMinutes int, maxCostUSD *float64) (store.LearningCourse, error) {
	var c store.LearningCourse
	err := s.tx(ctx, func(q *store.Queries) error {
		var err error
		if c, err = lockOwnedCourse(ctx, q, ws, user, id); err != nil {
			return err
		}
		if c.State != "outline_ready" {
			return apperr.Conflict("this module isn't waiting for approval")
		}
		o, err := parseOutline(c)
		if err != nil {
			return err
		}
		active := o.activeUnits()
		if len(active) == 0 {
			return apperr.Invalid("known", "you've marked every lesson as known: leave at least one to learn")
		}
		in := ResearchInput{Title: c.Topic, MaxSources: maxSources, MaxMinutes: maxMinutes, MaxCostUSD: maxCostUSD}
		for _, i := range active {
			in.Questions = append(in.Questions, ResearchQuestion{Label: o.Units[i].Title, Question: o.Units[i].Question})
		}
		if err := in.validate(); err != nil {
			return err
		}
		seq := int32(0)
		for i, u := range o.Units {
			for _, l := range u.Lessons {
				status := "pending"
				if l.Known {
					status = "known"
				}
				if err := q.CreateLearningLesson(ctx, store.CreateLearningLessonParams{CourseID: c.ID, Seq: seq, UnitSeq: int32(i), Key: l.Key, Title: l.Title, Objectives: l.Objectives, Prereqs: l.Prereqs, Minutes: int32(l.Minutes), Status: status}); err != nil {
					return err
				}
				seq++
			}
		}
		chat, err := q.GetChatSession(ctx, store.GetChatSessionParams{ID: c.ChatID, WorkspaceID: ws})
		if err != nil {
			return err
		}
		p, err := q.GetProject(ctx, store.GetProjectParams{ID: chat.ProjectID, WorkspaceID: ws})
		if err != nil {
			return err
		}
		if p.Archived {
			return apperr.Conflict("restore the archived project before building this module")
		}
		job, err := createResearchRuns(ctx, q, ws, user, chat, p.Key, nil, researchKindCourse, in)
		if err != nil {
			return err
		}
		if err := q.SetLearningCourseLimits(ctx, store.SetLearningCourseLimitsParams{ID: c.ID, MaxSources: int32(maxSources), MaxMinutes: int32(maxMinutes), MaxCostMicros: job.MaxCostMicros, ResearchJobID: &job.ID}); err != nil {
			return err
		}
		c, err = q.SetLearningCourseState(ctx, store.SetLearningCourseStateParams{ID: c.ID, State: "researching", Error: "", Round: 0})
		return err
	})
	return c, err
}

func cancelLearningRuns(ctx context.Context, q *store.Queries, c store.LearningCourse, reason string) error {
	rows, err := q.ListLearningRuns(ctx, c.ID)
	if err != nil {
		return err
	}
	tasks := make([]store.ListResearchTasksRow, 0, len(rows))
	for _, r := range rows {
		tasks = append(tasks, store.ListResearchTasksRow{RunID: r.RunID, RunState: r.RunState})
	}
	return cancelResearchRuns(ctx, q, tasks, reason)
}

// haltCourse cancels everything a course has running. Callers hold its lock.
func haltCourse(ctx context.Context, q *store.Queries, c store.LearningCourse, reason string) error {
	if err := cancelLearningRuns(ctx, q, c, reason); err != nil {
		return err
	}
	if c.ResearchJobID == nil {
		return nil
	}
	job, err := q.LockResearchJob(ctx, *c.ResearchJobID)
	if err != nil {
		return err
	}
	tasks, err := q.ListResearchTasks(ctx, job.ID)
	if err != nil {
		return err
	}
	if err := cancelResearchRuns(ctx, q, tasks, reason); err != nil {
		return err
	}
	if slices.Contains([]string{"running", "finishing", "ready", "writing"}, job.State) {
		_, err = q.SetResearchJobState(ctx, store.SetResearchJobStateParams{ID: job.ID, State: "stopped", StopReason: "person"})
	}
	return err
}

var courseBuilding = []string{"scoping", "outline_ready", "researching", "authoring", "verifying"}

// StopCourse ends a build. What was written stays out of reach: a partial
// module has no order worth studying, so it's only there to delete.
func (s *Service) StopCourse(ctx context.Context, ws, user, id uuid.UUID) (store.LearningCourse, error) {
	var c store.LearningCourse
	err := s.tx(ctx, func(q *store.Queries) error {
		var err error
		if c, err = lockOwnedCourse(ctx, q, ws, user, id); err != nil {
			return err
		}
		if !slices.Contains(courseBuilding, c.State) {
			return apperr.Conflict("this module isn't being built")
		}
		if err := haltCourse(ctx, q, c, "The module was stopped."); err != nil {
			return err
		}
		c, err = q.SetLearningCourseState(ctx, store.SetLearningCourseStateParams{ID: c.ID, State: "stopped", Error: "", Round: c.Round})
		return err
	})
	return c, err
}

// DeleteCourse stops a build if there is one and removes the module with its
// tutor chat.
func (s *Service) DeleteCourse(ctx context.Context, ws, user, id uuid.UUID) error {
	var chat uuid.UUID
	err := s.tx(ctx, func(q *store.Queries) error {
		c, err := lockOwnedCourse(ctx, q, ws, user, id)
		if err != nil {
			return err
		}
		chat = c.ChatID
		if slices.Contains(courseBuilding, c.State) {
			if err := haltCourse(ctx, q, c, "The module was deleted."); err != nil {
				return err
			}
			_, err = q.SetLearningCourseState(ctx, store.SetLearningCourseStateParams{ID: c.ID, State: "stopped", Error: "", Round: c.Round})
		}
		return err
	})
	if err != nil {
		return err
	}
	return s.DeleteChat(ctx, ws, user, chat) // the course goes with it
}
