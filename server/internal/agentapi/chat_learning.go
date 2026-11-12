package agentapi

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// The person's Guided Learning modules from a chat. create_learning_module
// calls the persona their tutor, so it can also see where they are: which
// lessons they've finished and which checks they keep missing.

// maxMissedChecks bounds how many still-missed checks get_learning_progress lists.
const maxMissedChecks = 10

type learningModule struct {
	ID            uuid.UUID `json:"module_id"`
	Topic         string    `json:"topic"`
	Goal          string    `json:"goal,omitempty"`
	Level         string    `json:"level"`
	State         string    `json:"state"`
	CurrentLesson string    `json:"current_lesson,omitempty"`
	StartedAt     string    `json:"started_at"`
	Href          string    `json:"href"`
}

func toLearningModule(c store.LearningCourse) learningModule {
	return learningModule{ID: c.ID, Topic: c.Topic, Goal: c.Goal, Level: c.Level, State: c.State, CurrentLesson: c.CurrentLesson,
		StartedAt: c.CreatedAt.Format(time.RFC3339), Href: "/learn/" + c.ID.String()}
}

func listLearningModules(ctx context.Context, d Deps, c agents.Caller, _ Empty) (any, error) {
	s, err := personChatScope(ctx, d, c, "list_learning_modules")
	if err != nil {
		return nil, err
	}
	rows, err := d.Q.ListLearningCourses(ctx, store.ListLearningCoursesParams{WorkspaceID: c.Agent.WorkspaceID, UserID: s.UserID})
	if err != nil {
		return nil, err
	}
	out := make([]learningModule, len(rows))
	for i, r := range rows {
		out[i] = toLearningModule(r)
	}
	return map[string]any{"modules": out, "href": "/learn"}, nil
}

type LearningProgressIn struct {
	ModuleID uuid.UUID `json:"module_id" jsonschema:"Module id from list_learning_modules"`
}

type lessonProgress struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Progress string `json:"progress,omitempty"`
	Current  bool   `json:"current,omitempty"`
}

// getLearningProgress reads a module's lessons with the person's progress, as
// the Learn page shows it: a lesson is complete once every check has been
// right, solid if each was right the first time.
func getLearningProgress(ctx context.Context, d Deps, c agents.Caller, in LearningProgressIn) (any, error) {
	s, err := personChatScope(ctx, d, c, "get_learning_progress")
	if err != nil {
		return nil, err
	}
	course, err := d.Q.GetLearningCourse(ctx, store.GetLearningCourseParams{ID: in.ModuleID, WorkspaceID: c.Agent.WorkspaceID, UserID: s.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.Invalid("module_id", "no learning module %s", in.ModuleID)
	} else if err != nil {
		return nil, err
	}
	lessons, err := d.Q.ListLearningLessons(ctx, course.ID)
	if err != nil {
		return nil, err
	}
	checkRows, err := d.Q.ListLearningChecks(ctx, course.ID)
	if err != nil {
		return nil, err
	}
	attemptRows, err := d.Q.ListLearningAttempts(ctx, store.ListLearningAttemptsParams{CourseID: course.ID, UserID: course.UserID})
	if err != nil {
		return nil, err
	}
	checks := map[uuid.UUID][]store.LearningCheck{}
	for _, k := range checkRows {
		checks[k.LessonID] = append(checks[k.LessonID], k)
	}
	attempts := map[uuid.UUID][]store.LearningAttempt{}
	for _, a := range attemptRows {
		attempts[a.CheckID] = append(attempts[a.CheckID], a)
	}
	out := make([]lessonProgress, len(lessons))
	counts := map[string]int{}
	studyable := 0
	for i, l := range lessons {
		out[i] = lessonProgress{Key: l.Key, Title: l.Title, Status: l.Status, Current: l.Key == course.CurrentLesson}
		// As on the Learn page, lessons they already know and ones without
		// checks aren't counted.
		if l.Status == "known" || len(checks[l.ID]) == 0 {
			continue
		}
		studyable++
		out[i].Progress = runs.LessonProgress(checks[l.ID], attempts)
		counts[out[i].Progress]++
	}
	// Checks they've answered wrong and not yet got right: what to work on.
	missed := []string{}
	for _, k := range checkRows {
		wrong, right := false, false
		for _, a := range attempts[k.ID] {
			if a.Correct != nil {
				wrong, right = wrong || !*a.Correct, right || *a.Correct
			}
		}
		if wrong && !right && len(missed) < maxMissedChecks {
			missed = append(missed, k.Prompt)
		}
	}
	return map[string]any{"module": toLearningModule(course), "lessons": out, "studyable": studyable,
		"complete": counts["complete"] + counts["solid"], "solid": counts["solid"], "still_missed": missed}, nil
}
