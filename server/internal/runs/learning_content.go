package runs

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

const (
	learningMaxBlocks, learningBodyRunes         = 60, 40000
	learningMaxChecks                            = 6
	learningMaxVisuals                           = 6
	learningMaxAltRunes, learningMaxCaptionRunes = 300, 500
	// learningRevealAfter is how many wrong tries it takes to show the answer.
	learningRevealAfter = 2
)

// LessonBlock is one piece of a lesson body.
type LessonBlock struct {
	Kind       string   `json:"kind" jsonschema:"heading, text (Markdown), code, diagram (SVG) or chart (JSON spec)"`
	Text       string   `json:"text" jsonschema:"The content: for a diagram its SVG, for a chart its JSON spec"`
	Lang       string   `json:"lang,omitempty" jsonschema:"For code: its language"`
	Provenance string   `json:"provenance,omitempty" jsonschema:"For text, diagrams and charts: sourced or model_knowledge"`
	Sources    []int    `json:"sources,omitempty" jsonschema:"For sourced blocks: the source numbers that support it"`
	Alt        string   `json:"alt,omitempty" jsonschema:"For diagrams and charts: what it shows, for someone who can't see it"`
	Caption    string   `json:"caption,omitempty" jsonschema:"For diagrams and charts: one line on what to notice (Markdown)"`
	Steps      []string `json:"steps,omitempty" jsonschema:"For a step-through diagram: one caption per step, 2 to 12"`
}

// LessonCheckInput is one knowledge check.
type LessonCheckInput struct {
	Objective   string   `json:"objective" jsonschema:"The objective it tests, exactly as written in the outline"`
	Kind        string   `json:"kind" jsonschema:"mcq, order or short"`
	Prompt      string   `json:"prompt"`
	Options     []string `json:"options,omitempty" jsonschema:"mcq: at least 3 choices. order: at least 3 items, in their correct order"`
	Answer      string   `json:"answer,omitempty" jsonschema:"mcq: the text of the one right choice. short: a model answer"`
	Rubric      string   `json:"rubric,omitempty" jsonschema:"short: what a good answer must contain"`
	Explanation string   `json:"explanation" jsonschema:"Why the answer is right"`
	Sources     []int    `json:"sources,omitempty"`
}

// LessonSubmission is one finished lesson.
type LessonSubmission struct {
	Key     string             `json:"key" jsonschema:"The lesson's key from the outline"`
	Minutes int                `json:"minutes,omitempty"`
	Blocks  []LessonBlock      `json:"blocks"`
	Checks  []LessonCheckInput `json:"checks" jsonschema:"At least one per objective, at most 6"`
}

func (in *LessonSubmission) validate(objectives []string, sources map[int]bool) error {
	if len(in.Blocks) == 0 || len(in.Blocks) > learningMaxBlocks {
		return apperr.Invalid("blocks", "give 1 to %d blocks", learningMaxBlocks)
	}
	runes := 0
	citedKnown := func(field string, ns []int) error {
		for _, n := range ns {
			if !sources[n] {
				return apperr.Invalid(field, "there's no source %d", n)
			}
		}
		return nil
	}
	checkProvenance := func(i int, b *LessonBlock) error {
		switch b.Provenance {
		case "sourced":
			if len(b.Sources) == 0 {
				return apperr.Invalid("blocks", "sourced block %d cites no sources: cite them, or mark it model_knowledge", i)
			}
			return citedKnown("blocks", b.Sources)
		case "model_knowledge":
			b.Sources = nil
			return nil
		}
		return apperr.Invalid("blocks", "%s block %d needs a provenance of sourced or model_knowledge", b.Kind, i)
	}
	visuals := 0
	for i := range in.Blocks {
		b := &in.Blocks[i]
		if strings.TrimSpace(b.Text) == "" {
			return apperr.Invalid("blocks", "block %d is empty", i)
		}
		switch b.Kind {
		case "heading", "code":
			b.Provenance, b.Sources = "", nil
			b.Alt, b.Caption, b.Steps = "", "", nil
			runes += utf8.RuneCountInString(b.Text)
		case "text":
			if err := checkProvenance(i, b); err != nil {
				return err
			}
			b.Alt, b.Caption, b.Steps = "", "", nil
			runes += utf8.RuneCountInString(b.Text)
		case "diagram", "chart":
			if visuals++; visuals > learningMaxVisuals {
				return apperr.Invalid("blocks", "at most %d diagrams and charts in a lesson", learningMaxVisuals)
			}
			if err := checkProvenance(i, b); err != nil {
				return err
			}
			if err := checkVisualBlock(i, b); err != nil {
				return err
			}
		default:
			return apperr.Invalid("blocks", "block %d: kind must be heading, text, code, diagram or chart", i)
		}
	}
	if runes > learningBodyRunes {
		return apperr.Invalid("blocks", "the lesson is too long: at most %d characters", learningBodyRunes)
	}
	if len(in.Checks) == 0 || len(in.Checks) > learningMaxChecks {
		return apperr.Invalid("checks", "give 1 to %d checks", learningMaxChecks)
	}
	covered := map[string]bool{}
	for i := range in.Checks {
		k := &in.Checks[i]
		k.Prompt, k.Answer, k.Explanation = strings.TrimSpace(k.Prompt), strings.TrimSpace(k.Answer), strings.TrimSpace(k.Explanation)
		if k.Prompt == "" || k.Explanation == "" {
			return apperr.Invalid("checks", "check %d needs a prompt and an explanation", i)
		}
		if !slices.Contains(objectives, k.Objective) {
			return apperr.Invalid("checks", "check %d's objective isn't one of this lesson's, word for word: %q", i, k.Objective)
		}
		covered[k.Objective] = true
		if err := citedKnown("checks", k.Sources); err != nil {
			return err
		}
		seen := map[string]bool{}
		for j := range k.Options {
			k.Options[j] = strings.TrimSpace(k.Options[j])
			if k.Options[j] == "" || seen[k.Options[j]] {
				return apperr.Invalid("checks", "check %d has an empty or repeated option", i)
			}
			seen[k.Options[j]] = true
		}
		switch k.Kind {
		case "mcq":
			if len(k.Options) < 3 || !seen[k.Answer] {
				return apperr.Invalid("checks", "multiple-choice check %d needs at least 3 options, and answer must be exactly one of them", i)
			}
		case "order":
			if len(k.Options) < 3 {
				return apperr.Invalid("checks", "ordering check %d needs at least 3 items", i)
			}
			k.Answer = ""
		case "short":
			k.Options = []string{}
			if k.Answer == "" || strings.TrimSpace(k.Rubric) == "" {
				return apperr.Invalid("checks", "short-answer check %d needs a model answer and a rubric", i)
			}
		default:
			return apperr.Invalid("checks", "check %d: kind must be mcq, order or short", i)
		}
	}
	for _, o := range objectives {
		if !covered[o] {
			return apperr.Invalid("checks", "no check tests the objective %q", o)
		}
	}
	return nil
}

// checkVisualBlock validates a diagram or chart. It has its own size limit, so
// a lesson's visuals don't eat into its text.
func checkVisualBlock(i int, b *LessonBlock) error {
	b.Text, b.Alt, b.Caption = strings.TrimSpace(b.Text), strings.TrimSpace(b.Alt), strings.TrimSpace(b.Caption)
	if b.Alt == "" {
		return apperr.Invalid("blocks", "%s block %d needs alt text saying what it shows", b.Kind, i)
	}
	if utf8.RuneCountInString(b.Alt) > learningMaxAltRunes || utf8.RuneCountInString(b.Caption) > learningMaxCaptionRunes {
		return apperr.Invalid("blocks", "%s block %d: alt is at most %d characters and caption %d", b.Kind, i, learningMaxAltRunes, learningMaxCaptionRunes)
	}
	var err error
	if b.Kind == "chart" {
		b.Steps = nil
		err = CheckLessonChart(b.Text)
	} else {
		if n := len(b.Steps); n == 1 || n > visualMaxSteps {
			return apperr.Invalid("blocks", "diagram block %d: a step-through has 2 to %d steps", i, visualMaxSteps)
		}
		for j := range b.Steps {
			if b.Steps[j] = strings.TrimSpace(b.Steps[j]); b.Steps[j] == "" || utf8.RuneCountInString(b.Steps[j]) > learningMaxAltRunes {
				return apperr.Invalid("blocks", "diagram block %d: step %d needs a caption of at most %d characters", i, j+1, learningMaxAltRunes)
			}
		}
		err = CheckLessonDiagram(b.Text, len(b.Steps))
	}
	if err != nil {
		return apperr.Invalid("blocks", "%s block %d: %s", b.Kind, i, err)
	}
	return nil
}

// SubmitCourseLesson stores an author's lesson and its checks, replacing what
// that lesson had.
func (s *Service) SubmitCourseLesson(ctx context.Context, runID uuid.UUID, in LessonSubmission) error {
	return s.tx(ctx, func(q *store.Queries) error {
		c, run, err := lockCourseForRun(ctx, q, runID, learningRoleAuthor)
		if err != nil {
			return err
		}
		if c.State != "authoring" {
			return apperr.Conflict("this module isn't being written")
		}
		l, err := q.GetLearningLessonByKey(ctx, store.GetLearningLessonByKeyParams{CourseID: c.ID, Key: in.Key})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && l.UnitSeq != run.UnitSeq) {
			return apperr.Invalid("key", "%q isn't one of the lessons you were asked to write", in.Key)
		} else if err != nil {
			return err
		}
		if l.Status != "pending" && l.Status != "written" {
			return apperr.Conflict("lesson %q doesn't need writing", in.Key)
		}
		rows, err := q.ListLearningSources(ctx, c.ID)
		if err != nil {
			return err
		}
		sources := make(map[int]bool, len(rows))
		for _, r := range rows {
			sources[int(r.N)] = true
		}
		if err := in.validate(l.Objectives, sources); err != nil {
			return err
		}
		body, err := json.Marshal(in.Blocks)
		if err != nil {
			return err
		}
		minutes := l.Minutes
		if in.Minutes > 0 {
			minutes = int32(in.Minutes)
		}
		if err := q.SetLearningLessonContent(ctx, store.SetLearningLessonContentParams{ID: l.ID, Body: body, Minutes: minutes}); err != nil {
			return err
		}
		if err := q.DeleteLearningChecks(ctx, l.ID); err != nil {
			return err
		}
		for i, k := range in.Checks {
			src := make([]int32, 0, len(k.Sources))
			for _, n := range k.Sources {
				src = append(src, int32(n))
			}
			opts := k.Options
			if opts == nil {
				opts = []string{}
			}
			if err := q.CreateLearningCheck(ctx, store.CreateLearningCheckParams{LessonID: l.ID, Seq: int32(i), Objective: k.Objective, Kind: k.Kind, Prompt: k.Prompt, Options: opts, Answer: k.Answer, Rubric: k.Rubric, Explanation: k.Explanation, Sources: src}); err != nil {
				return err
			}
		}
		return nil
	})
}

// LessonReview is a reviewer's verdict on one lesson.
type LessonReview struct {
	Key     string   `json:"key"`
	Verdict string   `json:"verdict" jsonschema:"pass or fail"`
	Issues  []string `json:"issues,omitempty" jsonschema:"For fail: exactly what's wrong and where"`
}

// SubmitCourseReviews applies verdicts. A failed lesson goes back for its one
// rewrite on the first pass and is flagged on the second.
func (s *Service) SubmitCourseReviews(ctx context.Context, runID uuid.UUID, reviews []LessonReview) error {
	return s.tx(ctx, func(q *store.Queries) error {
		c, run, err := lockCourseForRun(ctx, q, runID, learningRoleReview)
		if err != nil {
			return err
		}
		if c.State != "verifying" {
			return apperr.Conflict("this module isn't being reviewed")
		}
		for _, r := range reviews {
			l, err := q.GetLearningLessonByKey(ctx, store.GetLearningLessonByKeyParams{CourseID: c.ID, Key: r.Key})
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && (l.UnitSeq != run.UnitSeq || l.Status != "written")) {
				return apperr.Invalid("key", "%q isn't awaiting your review", r.Key)
			} else if err != nil {
				return err
			}
			status, issues := "", []string{}
			switch r.Verdict {
			case "pass":
				status = "verified"
			case "fail":
				for _, i := range r.Issues {
					if i = strings.TrimSpace(i); i != "" {
						issues = append(issues, i)
					}
				}
				if len(issues) == 0 {
					return apperr.Invalid("issues", "a failed lesson needs its issues: %q", r.Key)
				}
				status = "pending"
				if c.Round > 0 {
					status = "flagged"
				}
			default:
				return apperr.Invalid("verdict", "must be pass or fail")
			}
			if err := q.SetLearningLessonStatus(ctx, store.SetLearningLessonStatusParams{ID: l.ID, Status: status, Issues: issues}); err != nil {
				return err
			}
		}
		return nil
	})
}

// gradeCheck grades what can be graded without a person: nil for a short answer.
func gradeCheck(k store.LearningCheck, response []string) *bool {
	var ok bool
	switch k.Kind {
	case "mcq":
		ok = len(response) == 1 && response[0] == k.Answer
	case "order":
		ok = slices.Equal(response, k.Options)
	default:
		return nil
	}
	return &ok
}

// CheckOptions is what a learner sees: shuffled the same way every time, so
// neither position nor the stored order gives the answer away.
func CheckOptions(k store.LearningCheck) []string {
	out := slices.Clone(k.Options)
	key := func(o string) [32]byte { return sha256.Sum256([]byte(k.ID.String() + o)) }
	slices.SortFunc(out, func(a, b string) int {
		x, y := key(a), key(b)
		return slices.Compare(x[:], y[:])
	})
	return out
}

// CheckReveal is the answer side of a check, shown only once it's earned.
type CheckReveal struct {
	Answer      []string
	Rubric      string
	Explanation string
}

// RevealCheck returns the answer for this history of attempts, or nil.
func RevealCheck(k store.LearningCheck, attempts []store.LearningAttempt) *CheckReveal {
	solved := slices.ContainsFunc(attempts, func(a store.LearningAttempt) bool { return a.Correct != nil && *a.Correct })
	if !solved && len(attempts) < learningRevealAfter && !(k.Kind == "short" && len(attempts) > 0) {
		return nil
	}
	answer := []string{k.Answer}
	if k.Kind == "order" {
		answer = k.Options
	}
	return &CheckReveal{Answer: answer, Rubric: k.Rubric, Explanation: k.Explanation}
}

// CourseAttempt is a recorded attempt and everything that's now shown for it.
type CourseAttempt struct {
	Attempt store.LearningAttempt
	Reveal  *CheckReveal
}

// AttemptCheck records and grades one try at a check, on the server: the
// browser never has the key before this.
func (s *Service) AttemptCheck(ctx context.Context, ws, user, course, check uuid.UUID, response []string, confidence int) (CourseAttempt, error) {
	var out CourseAttempt
	if confidence < 0 || confidence > 3 {
		return out, apperr.Invalid("confidence", "must be between 0 and 3")
	}
	if len(response) == 0 || strings.TrimSpace(strings.Join(response, "")) == "" {
		return out, apperr.Invalid("response", "give an answer")
	}
	if utf8.RuneCountInString(strings.Join(response, "")) > 10000 {
		return out, apperr.Invalid("response", "that answer is too long")
	}
	var unit int32
	err := s.tx(ctx, func(q *store.Queries) error {
		c, err := q.GetLearningCourse(ctx, store.GetLearningCourseParams{ID: course, WorkspaceID: ws, UserID: user})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		} else if err != nil {
			return err
		}
		k, err := q.GetLearningCheck(ctx, store.GetLearningCheckParams{ID: check, CourseID: course})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		} else if err != nil {
			return err
		}
		if unit, err = q.LearningCheckUnit(ctx, store.LearningCheckUnitParams{ID: k.ID, CourseID: course}); err != nil {
			return err
		}
		if ok, err := studyable(c, unit); err != nil {
			return err
		} else if !ok {
			return apperr.Conflict("this lesson isn't ready yet")
		}
		out.Attempt, err = q.CreateLearningAttempt(ctx, store.CreateLearningAttemptParams{CheckID: k.ID, UserID: user, Response: response, Correct: gradeCheck(k, response), Confidence: int16(confidence)})
		if err != nil {
			return err
		}
		out.Reveal, err = revealFor(ctx, q, c, k, user)
		return err
	})
	if err == nil {
		s.planNextPartNear(ctx, ws, user, course, unit)
	}
	return out, err
}

// studyable reports whether a unit's lessons can be studied yet.
func studyable(c store.LearningCourse, unit int32) (bool, error) {
	if len(c.Outline) == 0 {
		return false, nil
	}
	o, err := parseOutline(c)
	if err != nil {
		return false, err
	}
	return int(unit) < StudyableUnits(c, o), nil
}

func revealFor(ctx context.Context, q *store.Queries, c store.LearningCourse, k store.LearningCheck, user uuid.UUID) (*CheckReveal, error) {
	all, err := q.ListLearningAttempts(ctx, store.ListLearningAttemptsParams{CourseID: c.ID, UserID: user})
	if err != nil {
		return nil, err
	}
	all = slices.DeleteFunc(all, func(a store.LearningAttempt) bool { return a.CheckID != k.ID })
	return RevealCheck(k, all), nil
}

// GradeShortAnswer records the person's own verdict against the rubric.
func (s *Service) GradeShortAnswer(ctx context.Context, ws, user, course, attempt uuid.UUID, correct bool) error {
	return s.tx(ctx, func(q *store.Queries) error {
		if _, err := q.GetLearningCourse(ctx, store.GetLearningCourseParams{ID: course, WorkspaceID: ws, UserID: user}); errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		} else if err != nil {
			return err
		}
		a, err := q.LockLearningAttempt(ctx, store.LockLearningAttemptParams{ID: attempt, UserID: user, CourseID: course})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		} else if err != nil {
			return err
		}
		if a.Correct != nil {
			return apperr.Conflict("this answer has already been marked")
		}
		return q.SetLearningAttemptCorrect(ctx, store.SetLearningAttemptCorrectParams{ID: a.ID, Correct: &correct})
	})
}

// PostTutorMessage notes which lesson the person is on and asks the tutor.
func (s *Service) PostTutorMessage(ctx context.Context, ws, user, course uuid.UUID, lesson, body string) (store.ChatMessage, error) {
	var out store.ChatMessage
	c, err := q(s).GetLearningCourse(ctx, store.GetLearningCourseParams{ID: course, WorkspaceID: ws, UserID: user})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperr.ErrNotFound
	} else if err != nil {
		return out, err
	}
	l, err := q(s).GetLearningLessonByKey(ctx, store.GetLearningLessonByKeyParams{CourseID: course, Key: lesson})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperr.Invalid("lesson", "there's no lesson %q", lesson)
	} else if err != nil {
		return out, err
	}
	if ok, err := studyable(c, l.UnitSeq); err != nil {
		return out, err
	} else if !ok {
		return out, apperr.Conflict("the tutor starts once this lesson is ready")
	}
	if err := q(s).SetLearningCurrentLesson(ctx, store.SetLearningCurrentLessonParams{ID: course, CurrentLesson: lesson}); err != nil {
		return out, err
	}
	return s.PostChatMessage(ctx, ws, user, c.ChatID, ChatMessageInput{Body: body})
}

func q(s *Service) *store.Queries { return store.New(s.Pool) }

// LessonProgress reads one lesson's state from its checks' attempts: "complete"
// once each has been right at some point, "solid" if each was right first try.
func LessonProgress(checks []store.LearningCheck, attempts map[uuid.UUID][]store.LearningAttempt) string {
	if len(checks) == 0 {
		return "new"
	}
	complete, solid := true, true
	for _, k := range checks {
		graded := slices.DeleteFunc(slices.Clone(attempts[k.ID]), func(a store.LearningAttempt) bool { return a.Correct == nil })
		if !slices.ContainsFunc(graded, func(a store.LearningAttempt) bool { return *a.Correct }) {
			complete = false
		}
		if len(graded) == 0 || !*graded[0].Correct {
			solid = false
		}
	}
	switch {
	case complete && solid:
		return "solid"
	case complete:
		return "complete"
	}
	return "new"
}
