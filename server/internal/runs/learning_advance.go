package runs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

// AdvanceLearningCourses moves every building course forward. The runner's
// sweep calls it, and so does the end of a course's or researcher's run, so
// nothing depends on any one signal.
func AdvanceLearningCourses(ctx context.Context, db interface {
	beginner
	store.DBTX
}) error {
	ids, err := store.New(db).ActiveLearningCourses(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		if err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error { return advanceLearningCourse(ctx, store.New(tx), id) }); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func advanceLearningCourse(ctx context.Context, q *store.Queries, id uuid.UUID) error {
	c, err := q.LockLearningCourse(ctx, id)
	if err != nil {
		return err
	}
	rows, err := q.ListLearningRuns(ctx, id)
	if err != nil {
		return err
	}
	live := slices.ContainsFunc(rows, func(r store.ListLearningRunsRow) bool { return !slices.Contains(Terminal, r.RunState) })
	switch c.State {
	case "scoping":
		if live {
			return nil
		}
		if !hasPartOutline(c) {
			if c.Part > 0 {
				return failCourse(ctx, q, c, "Its outline couldn't be drafted. Try again.")
			}
			return failCourse(ctx, q, c, "The outline couldn't be drafted. Delete this module and try again.")
		}
		_, err := q.SetLearningCourseState(ctx, store.SetLearningCourseStateParams{ID: c.ID, State: "outline_ready", Error: "", Round: 0})
		return err
	case "researching":
		return advanceCourseResearch(ctx, q, c)
	case "authoring", "verifying":
		over := false
		if c.MaxCostMicros != nil {
			spent, err := q.LearningCourseSpend(ctx, c.ID)
			if err != nil {
				return err
			}
			over = spent >= *c.MaxCostMicros
		}
		if over && live {
			if err := cancelLearningRuns(ctx, q, c, "The module reached its spend limit."); err != nil {
				return err
			}
			live = false
		}
		if live {
			return nil
		}
		return advanceCourseRound(ctx, q, c, over)
	}
	return nil
}

// failCourse ends a build that can't go on. A later part is dropped instead,
// so what was built stays to study.
func failCourse(ctx context.Context, q *store.Queries, c store.LearningCourse, msg string) error {
	if c.Part > 0 {
		_, err := dropPart(ctx, q, c, strings.TrimSpace(fmt.Sprintf("Part %d couldn't be built. %s", c.Part+1, msg)))
		return err
	}
	_, err := q.SetLearningCourseState(ctx, store.SetLearningCourseStateParams{ID: c.ID, State: "failed", Error: msg, Round: c.Round})
	return err
}

// hasPartOutline reports whether scoping produced units for the part being built.
func hasPartOutline(c store.LearningCourse) bool {
	if len(c.Outline) == 0 {
		return false
	}
	o, err := parseOutline(c)
	return err == nil && o.firstUnit(int(c.Part)) < len(o.Units)
}

func advanceCourseResearch(ctx context.Context, q *store.Queries, c store.LearningCourse) error {
	if c.ResearchJobID == nil {
		return failCourse(ctx, q, c, "The module's research was lost.")
	}
	job, err := q.LockResearchJob(ctx, *c.ResearchJobID)
	if err != nil {
		return err
	}
	switch job.State {
	case "ready":
	case "failed":
		return failCourse(ctx, q, c, "Research found nothing to build on. "+job.Error)
	case "stopped":
		if c.Part > 0 {
			_, err := dropPart(ctx, q, c, "")
			return err
		}
		_, err := q.SetLearningCourseState(ctx, store.SetLearningCourseStateParams{ID: c.ID, State: "stopped", Error: "", Round: c.Round})
		return err
	default:
		return nil
	}
	tasks, err := q.ListResearchTasks(ctx, job.ID)
	if err != nil {
		return err
	}
	if err := numberCourseSources(ctx, q, c.ID, tasks); err != nil {
		return err
	}
	if _, err := q.SetResearchJobState(ctx, store.SetResearchJobStateParams{ID: job.ID, State: "done", StopReason: job.StopReason}); err != nil {
		return err
	}
	c, err = q.SetLearningCourseState(ctx, store.SetLearningCourseStateParams{ID: c.ID, State: "authoring", Error: "", Round: 0})
	if err != nil {
		return err
	}
	return queueCourseAuthors(ctx, q, c)
}

// numberCourseSources gives each page the researchers cited one number, which
// lessons then cite. A later part's pages carry on from the earlier parts'.
func numberCourseSources(ctx context.Context, q *store.Queries, course uuid.UUID, tasks []store.ListResearchTasksRow) error {
	seen, n := map[string]bool{}, int32(0)
	have, err := q.ListLearningSources(ctx, course)
	if err != nil {
		return err
	}
	for _, s := range have {
		seen[s.Url] = true
		n = max(n, s.N)
	}
	for _, t := range tasks {
		var found []ResearchFinding
		if err := json.Unmarshal(t.Findings, &found); err != nil {
			return err
		}
		for _, f := range found {
			for _, src := range f.Sources {
				if seen[src.URL] {
					continue
				}
				seen[src.URL] = true
				n++
				title := strings.TrimSpace(src.Title)
				if title == "" {
					title = src.URL
				}
				if err := q.CreateLearningSource(ctx, store.CreateLearningSourceParams{CourseID: course, N: n, Title: title, Url: src.URL}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// advanceCourseRound runs when a pass's runs have all ended: review what was
// written, rewrite what failed once, then finish.
func advanceCourseRound(ctx context.Context, q *store.Queries, c store.LearningCourse, over bool) error {
	lessons, err := q.ListLearningLessons(ctx, c.ID)
	if err != nil {
		return err
	}
	count := func(status string) int {
		n := 0
		for _, l := range lessons {
			if l.Status == status {
				n++
			}
		}
		return n
	}
	pending, written := count("pending"), count("written")
	if over {
		return finishCourse(ctx, q, c, lessons, "The module reached its spend limit first.")
	}
	if c.State == "authoring" {
		if written > 0 {
			if c, err = q.SetLearningCourseState(ctx, store.SetLearningCourseStateParams{ID: c.ID, State: "verifying", Error: "", Round: c.Round}); err != nil {
				return err
			}
			return queueCourseReviewers(ctx, q, c)
		}
		if c.Round == 0 {
			if c, err = q.SetLearningCourseState(ctx, store.SetLearningCourseStateParams{ID: c.ID, State: "authoring", Error: "", Round: 1}); err != nil {
				return err
			}
			return queueCourseAuthors(ctx, q, c)
		}
		return finishCourse(ctx, q, c, lessons, "")
	}
	// Review has finished. One more pass covers what it failed or didn't reach.
	if c.Round == 0 && pending > 0 {
		if c, err = q.SetLearningCourseState(ctx, store.SetLearningCourseStateParams{ID: c.ID, State: "authoring", Error: "", Round: 1}); err != nil {
			return err
		}
		return queueCourseAuthors(ctx, q, c)
	}
	if c.Round == 0 && written > 0 {
		if c, err = q.SetLearningCourseState(ctx, store.SetLearningCourseStateParams{ID: c.ID, State: "verifying", Error: "", Round: 1}); err != nil {
			return err
		}
		return queueCourseReviewers(ctx, q, c)
	}
	return finishCourse(ctx, q, c, lessons, "")
}

// finishCourse keeps what exists: a lesson that didn't make it is flagged, not
// hidden, unless nothing at all was written.
func finishCourse(ctx context.Context, q *store.Queries, c store.LearningCourse, lessons []store.LearningLesson, why string) error {
	o, err := parseOutline(c)
	if err != nil {
		return err
	}
	from := int32(o.firstUnit(int(c.Part)))
	usable := 0
	for _, l := range lessons {
		if l.UnitSeq < from {
			continue // an earlier part's
		}
		issues := l.Issues
		switch l.Status {
		case "verified":
			usable++
			continue
		case "flagged":
			if string(l.Body) != "[]" {
				usable++
			}
			continue
		case "written":
			usable++
			issues = append(slices.Clone(issues), strings.TrimSpace("The reviewer didn't finish this lesson, so treat it with care. "+why))
		case "pending":
			if string(l.Body) != "[]" {
				usable++ // a failed rewrite keeps its reviewer's issues
				issues = append(slices.Clone(issues), strings.TrimSpace("It couldn't be rewritten. "+why))
			} else {
				issues = []string{strings.TrimSpace("This lesson couldn't be written. " + why)}
			}
		default:
			continue
		}
		if err := q.SetLearningLessonStatus(ctx, store.SetLearningLessonStatusParams{ID: l.ID, Status: "flagged", Issues: issues}); err != nil {
			return err
		}
	}
	if usable == 0 {
		return failCourse(ctx, q, c, strings.TrimSpace("No lesson could be written. "+why))
	}
	_, err = q.SetLearningCourseState(ctx, store.SetLearningCourseStateParams{ID: c.ID, State: "ready", Error: "", Round: c.Round})
	return err
}

type courseBuild struct {
	env      learningEnv
	outline  CourseOutline
	active   []int
	tasks    []store.ListResearchTasksRow
	sources  []store.LearningSource
	numbered map[string]int32
	lessons  []store.LearningLesson
}

func loadCourseBuild(ctx context.Context, q *store.Queries, c store.LearningCourse) (b courseBuild, err error) {
	if b.env, err = loadLearningEnv(ctx, q, c); err != nil {
		return b, err
	}
	if b.outline, err = parseOutline(c); err != nil {
		return b, err
	}
	b.active = b.outline.activeUnits(int(c.Part))
	if c.ResearchJobID != nil {
		if b.tasks, err = q.ListResearchTasks(ctx, *c.ResearchJobID); err != nil {
			return b, err
		}
	}
	if b.sources, err = q.ListLearningSources(ctx, c.ID); err != nil {
		return b, err
	}
	b.numbered = make(map[string]int32, len(b.sources))
	for _, s := range b.sources {
		b.numbered[s.Url] = s.N
	}
	b.lessons, err = q.ListLearningLessons(ctx, c.ID)
	return b, err
}

// unitFindings renders a unit's research with the course's source numbers.
func (b courseBuild) unitFindings(unit int32) string {
	i := slices.Index(b.active, int(unit))
	if i < 0 || i >= len(b.tasks) {
		return "(No research for this unit.)"
	}
	var found []ResearchFinding
	if err := json.Unmarshal(b.tasks[i].Findings, &found); err != nil {
		return "(No research for this unit.)"
	}
	var out strings.Builder
	for _, f := range found {
		nums := make([]string, 0, len(f.Sources))
		for _, s := range f.Sources {
			nums = append(nums, fmt.Sprintf("[%d]", b.numbered[s.URL]))
		}
		line := "- " + f.Claim
		if f.Detail != "" {
			line += " (" + f.Detail + ")"
		}
		line += " " + strings.Join(nums, "") + "\n"
		if utf8.RuneCountInString(out.String())+utf8.RuneCountInString(line) > researchFindingsRunes {
			out.WriteString("(Remaining findings were left out for length.)\n")
			break
		}
		out.WriteString(line)
	}
	if out.Len() == 0 {
		return "(The researcher for this unit found nothing.)"
	}
	return out.String()
}

func (b courseBuild) unitsWith(status string) []int32 {
	out := []int32{}
	for _, l := range b.lessons {
		if l.Status == status && !slices.Contains(out, l.UnitSeq) {
			out = append(out, l.UnitSeq)
		}
	}
	return out
}

func queueCourseAuthors(ctx context.Context, q *store.Queries, c store.LearningCourse) error {
	b, err := loadCourseBuild(ctx, q, c)
	if err != nil {
		return err
	}
	for _, unit := range b.unitsWith("pending") {
		if err := b.env.queue(ctx, q, c, learningRoleAuthor, c.Round, unit, learningAuthorPrompt(c, b, unit)); err != nil {
			return err
		}
	}
	return nil
}

func queueCourseReviewers(ctx context.Context, q *store.Queries, c store.LearningCourse) error {
	b, err := loadCourseBuild(ctx, q, c)
	if err != nil {
		return err
	}
	checks, err := q.ListLearningChecks(ctx, c.ID)
	if err != nil {
		return err
	}
	for _, unit := range b.unitsWith("written") {
		if err := b.env.queue(ctx, q, c, learningRoleReview, c.Round, unit, learningReviewPrompt(c, b, checks, unit)); err != nil {
			return err
		}
	}
	return nil
}
