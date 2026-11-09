package runs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// A module longer than learningPartMinutes is built in parts. Its first
// scoping run plans every part and outlines the first; each later part is
// outlined when the learner gets near the end of what's built (or asks for it),
// from what they've done so far, then approved, researched, written and
// reviewed like the first. The learner keeps studying the parts already built
// meanwhile, and a part that fails or is stopped is dropped, leaving the module
// as it was.

// partTitle names a part's research job: the topic, and the part when there are several.
func partTitle(topic string, o CourseOutline, part int) string {
	if part >= len(o.Parts) {
		return topic
	}
	return truncate(fmt.Sprintf("%s: part %d, %s", topic, part+1, o.Parts[part].Title), 200)
}

// dropPart abandons the part being built: its lessons and units go, and the
// module is ready again with the parts it had. Callers hold the course's lock
// and have stopped its runs.
func dropPart(ctx context.Context, q *store.Queries, c store.LearningCourse, msg string) (store.LearningCourse, error) {
	if c.Part == 0 {
		return c, fmt.Errorf("dropPart: the first part can't be dropped")
	}
	if len(c.Outline) > 0 {
		o, err := parseOutline(c)
		if err != nil {
			return c, err
		}
		from := o.firstUnit(int(c.Part))
		if err := q.DeleteLearningLessonsFrom(ctx, store.DeleteLearningLessonsFromParams{CourseID: c.ID, UnitSeq: int32(from)}); err != nil {
			return c, err
		}
		o.Units = o.Units[:from]
		raw, err := json.Marshal(o)
		if err != nil {
			return c, err
		}
		if c, err = q.SetLearningCourseOutline(ctx, store.SetLearningCourseOutlineParams{ID: c.ID, Outline: raw}); err != nil {
			return c, err
		}
	}
	c, err := q.SetLearningCoursePart(ctx, store.SetLearningCoursePartParams{ID: c.ID, Part: c.Part - 1})
	if err != nil {
		return c, err
	}
	return q.SetLearningCourseState(ctx, store.SetLearningCourseStateParams{ID: c.ID, State: "ready", Error: msg, Round: 0})
}

// StartNextPart starts outlining a module's next part.
func (s *Service) StartNextPart(ctx context.Context, ws, user, id uuid.UUID) (store.LearningCourse, error) {
	var c store.LearningCourse
	err := s.tx(ctx, func(q *store.Queries) error {
		var err error
		if c, err = lockOwnedCourse(ctx, q, ws, user, id); err != nil {
			return err
		}
		c, err = startNextPart(ctx, q, c)
		return err
	})
	return c, err
}

// HasNextPart reports whether a ready module has a part still to build.
func HasNextPart(c store.LearningCourse, o CourseOutline) bool {
	return c.State == "ready" && int(c.Part)+1 < len(o.Parts)
}

func startNextPart(ctx context.Context, q *store.Queries, c store.LearningCourse) (store.LearningCourse, error) {
	if c.State != "ready" {
		return c, apperr.Conflict("finish building this part first")
	}
	o, err := parseOutline(c)
	if err != nil {
		return c, err
	}
	if !HasNextPart(c, o) {
		return c, apperr.Conflict("this module has no more parts to build")
	}
	e, err := loadLearningEnv(ctx, q, c)
	if err != nil {
		return c, err
	}
	p, err := chatProject(ctx, q, e.chat)
	if err != nil {
		return c, err
	}
	if p.Archived {
		return c, apperr.Conflict("restore the archived project before building the next part")
	}
	prompt, err := learningPartScopePrompt(ctx, q, c, o)
	if err != nil {
		return c, err
	}
	if c, err = q.SetLearningCoursePart(ctx, store.SetLearningCoursePartParams{ID: c.ID, Part: c.Part + 1}); err != nil {
		return c, err
	}
	if c, err = q.SetLearningCourseState(ctx, store.SetLearningCourseStateParams{ID: c.ID, State: "scoping", Error: "", Round: 0}); err != nil {
		return c, err
	}
	return c, e.queue(ctx, q, c, learningRoleScope, 0, 0, prompt)
}

// planNextPartNear starts outlining the next part once the learner works in
// the last unit built, so it's ready to approve by the time they need it. It's
// best effort: the person can always start it themselves.
func (s *Service) planNextPartNear(ctx context.Context, ws, user, course uuid.UUID, unit int32) {
	_ = s.tx(ctx, func(q *store.Queries) error {
		c, err := lockOwnedCourse(ctx, q, ws, user, course)
		if err != nil {
			return err
		}
		o, err := parseOutline(c)
		if err != nil || !HasNextPart(c, o) || int(unit) != len(o.Units)-1 {
			return err
		}
		_, err = startNextPart(ctx, q, c)
		return err
	})
}

// learningPartScopePrompt asks for the next part's outline, with what the
// learner has done so far so it can build on it.
func learningPartScopePrompt(ctx context.Context, q *store.Queries, c store.LearningCourse, o CourseOutline) (string, error) {
	next := int(c.Part) + 1
	lessons, err := q.ListLearningLessons(ctx, c.ID)
	if err != nil {
		return "", err
	}
	checks, err := q.ListLearningChecks(ctx, c.ID)
	if err != nil {
		return "", err
	}
	attempts, err := q.ListLearningAttempts(ctx, store.ListLearningAttemptsParams{CourseID: c.ID, UserID: c.UserID})
	if err != nil {
		return "", err
	}
	byLesson := map[uuid.UUID][]store.LearningCheck{}
	for _, k := range checks {
		byLesson[k.LessonID] = append(byLesson[k.LessonID], k)
	}
	byCheck := map[uuid.UUID][]store.LearningAttempt{}
	missed := map[uuid.UUID]bool{}
	for _, a := range attempts {
		byCheck[a.CheckID] = append(byCheck[a.CheckID], a)
		if a.Correct != nil && !*a.Correct {
			missed[a.CheckID] = true
		}
	}
	goal := c.Goal
	if goal == "" {
		goal = "(none given)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Design the next part of a learning module.\nTopic: %s\nThe learner's goal: %s\nLevel: %s\n\nThe module's parts:\n", c.Topic, goal, learningLevels[c.Level])
	for i, p := range o.Parts {
		mark := ""
		switch {
		case i < next:
			mark = " (built)"
		case i == next:
			mark = " <- outline this one"
		}
		fmt.Fprintf(&b, "%d. %s (about %d min): %s%s\n", i+1, p.Title, p.Minutes, p.Summary, mark)
	}
	b.WriteString("\nAlready taught, with how the learner did. Use these keys as prerequisites; don't teach them again, and don't reuse their keys or unit titles:\n")
	var shaky []string
	for i, l := range lessons {
		if i == 0 || l.UnitSeq != lessons[i-1].UnitSeq {
			if int(l.UnitSeq) < len(o.Units) {
				fmt.Fprintf(&b, "Unit: %s\n", o.Units[l.UnitSeq].Title)
			}
		}
		how := map[string]string{"new": "not done yet", "complete": "done", "solid": "solid"}[LessonProgress(byLesson[l.ID], byCheck)]
		if l.Status == "known" {
			how = "already knew it"
		}
		fmt.Fprintf(&b, "- %s: %s (%s)\n", l.Key, l.Title, how)
		for _, k := range byLesson[l.ID] {
			if missed[k.ID] {
				shaky = append(shaky, l.Title)
				break
			}
		}
	}
	if len(shaky) > 0 {
		fmt.Fprintf(&b, "\nThey got checks wrong in: %s. If this part builds on those, you may open with a short review lesson.\n", strings.Join(shaky, "; "))
	}
	fmt.Fprintf(&b, "\nOutline part %d only: about %d minutes, at most %d units and %d lessons. Leave parts out. Call submit_outline, then stop.", next+1, o.Parts[next].Minutes, learningMaxUnits, learningMaxLessons)
	return b.String(), nil
}
