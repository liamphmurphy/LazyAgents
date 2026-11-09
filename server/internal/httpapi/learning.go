package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// courseView is a module's lessons with the person's progress on them.
type courseView struct {
	lessons  []store.LearningLesson
	checks   map[uuid.UUID][]store.LearningCheck
	attempts map[uuid.UUID][]store.LearningAttempt
	progress map[string]string
}

func (s *Server) loadCourseView(ctx context.Context, c store.LearningCourse) (courseView, error) {
	v := courseView{checks: map[uuid.UUID][]store.LearningCheck{}, attempts: map[uuid.UUID][]store.LearningAttempt{}, progress: map[string]string{}}
	var err error
	if v.lessons, err = s.Q.ListLearningLessons(ctx, c.ID); err != nil {
		return v, err
	}
	checks, err := s.Q.ListLearningChecks(ctx, c.ID)
	if err != nil {
		return v, err
	}
	for _, k := range checks {
		v.checks[k.LessonID] = append(v.checks[k.LessonID], k)
	}
	attempts, err := s.Q.ListLearningAttempts(ctx, store.ListLearningAttemptsParams{CourseID: c.ID, UserID: c.UserID})
	if err != nil {
		return v, err
	}
	for _, a := range attempts {
		v.attempts[a.CheckID] = append(v.attempts[a.CheckID], a)
	}
	for _, l := range v.lessons {
		v.progress[l.Key] = runs.LessonProgress(v.checks[l.ID], v.attempts)
	}
	return v, nil
}

// studyable: known lessons and ones that couldn't be written aren't counted.
func (v courseView) studyable(l store.LearningLesson) bool {
	return l.Status != "known" && len(v.checks[l.ID]) > 0
}

func (s *Server) courseNote(ctx context.Context, c store.LearningCourse, v courseView) (string, error) {
	switch c.State {
	case "researching":
		if c.ResearchJobID == nil {
			return "", nil
		}
		tasks, err := s.Q.ListResearchTasks(ctx, *c.ResearchJobID)
		if err != nil {
			return "", err
		}
		done, read := 0, 0
		for _, t := range tasks {
			if slices.Contains(runs.Terminal, t.RunState) {
				done++
			}
			read += int(t.SourcesRead)
		}
		return fmt.Sprintf("%d of %d researchers done · %d sources", done, len(tasks), read), nil
	case "authoring", "verifying":
		role, verb := "author", "written"
		if c.State == "verifying" {
			role, verb = "review", "reviewed"
		}
		rows, err := s.Q.ListLearningRuns(ctx, c.ID)
		if err != nil {
			return "", err
		}
		total, done := 0, 0
		for _, r := range rows {
			if r.Role == role && r.Round == c.Round && r.Part == c.Part {
				total++
				if slices.Contains(runs.Terminal, r.RunState) {
					done++
				}
			}
		}
		note := fmt.Sprintf("%d of %d units %s", done, total, verb)
		if c.Round > 0 {
			note += " · second pass"
		}
		return note, nil
	}
	return "", nil
}

func (s *Server) toCourseSummary(ctx context.Context, c store.LearningCourse, v courseView) (api.CourseSummary, error) {
	out := api.CourseSummary{Id: c.ID, Topic: c.Topic, State: api.CourseState(c.State), Error: c.Error, CreatedAt: c.CreatedAt, ReadyAt: c.ReadyAt, Part: int(c.Part), Parts: 1}
	var err error
	if out.BuildNote, err = s.courseNote(ctx, c, v); err != nil {
		return out, err
	}
	study := 0
	if len(c.Outline) > 0 {
		var o runs.CourseOutline
		if err := json.Unmarshal(c.Outline, &o); err != nil {
			return out, err
		}
		out.Parts = max(1, len(o.Parts))
		study = runs.StudyableUnits(c, o)
		// The outline's figures are for the part being built or last built.
		total, minutes := 0, 0
		for _, u := range o.Units {
			if u.Part != int(c.Part) {
				continue
			}
			out.Units++
			for _, l := range u.Lessons {
				if !l.Known {
					total++
					minutes += l.Minutes
				}
			}
		}
		out.Lessons, out.Minutes = total, &minutes
	}
	out.Studyable = study > 0
	if !out.Studyable {
		return out, nil
	}
	out.Lessons = 0
	for _, l := range v.lessons {
		if int(l.UnitSeq) >= study {
			continue
		}
		if l.Status == "flagged" {
			out.Flagged++
		}
		if !v.studyable(l) {
			continue
		}
		out.Lessons++
		switch v.progress[l.Key] {
		case "solid":
			out.Solid++
			out.Complete++
		case "complete":
			out.Complete++
		default:
			if out.NextLesson == nil {
				title := l.Title
				out.NextLesson = &title
			}
		}
	}
	return out, nil
}

func toCourseCheck(k store.LearningCheck, attempts []store.LearningAttempt) api.CourseCheck {
	out := api.CourseCheck{Id: k.ID, Objective: k.Objective, Kind: api.CourseCheckKind(k.Kind), Prompt: k.Prompt, Options: runs.CheckOptions(k), Attempts: len(attempts)}
	for _, a := range attempts {
		if a.Correct == nil {
			id := a.ID
			out.UngradedAttemptId = &id
			continue
		}
		out.UngradedAttemptId = nil
		correct := *a.Correct
		out.LastCorrect = &correct
		out.Solved = out.Solved || correct
	}
	out.Reveal = toCourseReveal(runs.RevealCheck(k, attempts))
	return out
}

func toCourseReveal(r *runs.CheckReveal) *api.CourseReveal {
	if r == nil {
		return nil
	}
	return &api.CourseReveal{Answer: r.Answer, Rubric: r.Rubric, Explanation: r.Explanation}
}

func (s *Server) toCourse(ctx context.Context, c store.LearningCourse) (api.Course, error) {
	out := api.Course{Goal: c.Goal, Level: api.CourseLevel(c.Level), MinutesBudget: int(c.MinutesBudget), ChatId: c.ChatID, MaxSources: int(c.MaxSources), MaxMinutes: int(c.MaxMinutes), Lessons: []api.CourseLesson{}, Sources: []api.CourseSource{}}
	v, err := s.loadCourseView(ctx, c)
	if err != nil {
		return out, err
	}
	if out.Summary, err = s.toCourseSummary(ctx, c, v); err != nil {
		return out, err
	}
	var outline runs.CourseOutline
	if len(c.Outline) > 0 {
		if err := json.Unmarshal(c.Outline, &outline); err != nil {
			return out, err
		}
		var o api.CourseOutline
		if err := json.Unmarshal(c.Outline, &o); err != nil {
			return out, err
		}
		out.Outline = &o
	}
	titles := map[string]string{}
	for _, l := range v.lessons {
		titles[l.Key] = l.Title
	}
	statusOf := map[string]string{}
	for _, l := range v.lessons {
		statusOf[l.Key] = l.Status
	}
	// Answers and bodies only leave the server once their part is finished.
	study := runs.StudyableUnits(c, outline)
	if study == 0 {
		return out, nil
	}
	for _, l := range v.lessons {
		if int(l.UnitSeq) >= study {
			continue
		}
		lesson := api.CourseLesson{Key: l.Key, Title: l.Title, Objectives: l.Objectives, Prereqs: l.Prereqs, Minutes: int(l.Minutes), Status: api.CourseLessonStatus(l.Status), Issues: l.Issues, Progress: api.CourseLessonProgress(v.progress[l.Key]), ShakyPrereqs: []string{}, Checks: []api.CourseCheck{}, Body: []api.CourseBlock{}}
		if int(l.UnitSeq) < len(outline.Units) {
			lesson.UnitTitle = outline.Units[l.UnitSeq].Title
		}
		if err := json.Unmarshal(l.Body, &lesson.Body); err != nil {
			return out, err
		}
		for _, p := range l.Prereqs {
			if statusOf[p] != "known" && v.progress[p] == "new" {
				lesson.ShakyPrereqs = append(lesson.ShakyPrereqs, titles[p])
			}
		}
		for _, k := range v.checks[l.ID] {
			lesson.Checks = append(lesson.Checks, toCourseCheck(k, v.attempts[k.ID]))
		}
		out.Lessons = append(out.Lessons, lesson)
	}
	sources, err := s.Q.ListLearningSources(ctx, c.ID)
	if err != nil {
		return out, err
	}
	for _, src := range sources {
		out.Sources = append(out.Sources, api.CourseSource{N: int(src.N), Title: src.Title, Url: src.Url})
	}
	return out, nil
}

func (s *Server) ownedCourse(ctx context.Context, id uuid.UUID) (store.LearningCourse, error) {
	u, _, ws := me(ctx)
	c, err := s.Q.GetLearningCourse(ctx, store.GetLearningCourseParams{ID: id, WorkspaceID: ws, UserID: u.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return c, apperr.ErrNotFound
	}
	return c, err
}

func (s *Server) ListCourses(ctx context.Context, _ api.ListCoursesRequestObject) (api.ListCoursesResponseObject, error) {
	u, _, ws := me(ctx)
	rows, err := s.Q.ListLearningCourses(ctx, store.ListLearningCoursesParams{WorkspaceID: ws, UserID: u.ID})
	if err != nil {
		return nil, err
	}
	out := make(api.ListCourses200JSONResponse, 0, len(rows))
	for _, c := range rows {
		v, err := s.loadCourseView(ctx, c)
		if err != nil {
			return nil, err
		}
		sum, err := s.toCourseSummary(ctx, c, v)
		if err != nil {
			return nil, err
		}
		out = append(out, sum)
	}
	return out, nil
}

func (s *Server) CreateCourse(ctx context.Context, req api.CreateCourseRequestObject) (api.CreateCourseResponseObject, error) {
	u, _, ws := me(ctx)
	b := req.Body
	in := runs.CourseInput{Project: b.Project, Topic: b.Topic, Goal: deref(b.Goal), Level: string(b.Level), MinutesBudget: b.MinutesBudget, PersonaID: b.PersonaId, Harness: deref(b.Harness), Model: deref(b.Model)}
	if b.Effort != nil {
		in.Effort = string(*b.Effort)
	}
	c, err := s.Runs.StartCourse(ctx, ws, u.ID, in)
	if err != nil {
		return nil, err
	}
	out, err := s.toCourse(ctx, c)
	return api.CreateCourse201JSONResponse(out), err
}

func (s *Server) GetCourse(ctx context.Context, req api.GetCourseRequestObject) (api.GetCourseResponseObject, error) {
	c, err := s.ownedCourse(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	out, err := s.toCourse(ctx, c)
	return api.GetCourse200JSONResponse(out), err
}

func (s *Server) DeleteCourse(ctx context.Context, req api.DeleteCourseRequestObject) (api.DeleteCourseResponseObject, error) {
	u, _, ws := me(ctx)
	if err := s.Runs.DeleteCourse(ctx, ws, u.ID, req.Id); err != nil {
		return nil, err
	}
	return api.DeleteCourse204Response{}, nil
}

func (s *Server) EditCourseOutline(ctx context.Context, req api.EditCourseOutlineRequestObject) (api.EditCourseOutlineResponseObject, error) {
	u, _, ws := me(ctx)
	e := runs.OutlineEdit{Known: req.Body.Known}
	for _, unit := range req.Body.Units {
		e.Units = append(e.Units, runs.OutlineEditUnit{Title: unit.Title, Lessons: unit.Lessons})
	}
	c, err := s.Runs.EditCourseOutline(ctx, ws, u.ID, req.Id, e)
	if err != nil {
		return nil, err
	}
	out, err := s.toCourse(ctx, c)
	return api.EditCourseOutline200JSONResponse(out), err
}

func (s *Server) ApproveCourse(ctx context.Context, req api.ApproveCourseRequestObject) (api.ApproveCourseResponseObject, error) {
	u, _, ws := me(ctx)
	var limit *float64
	if req.Body.MaxCostUsd != nil {
		v := float64(*req.Body.MaxCostUsd)
		limit = &v
	}
	c, err := s.Runs.ApproveCourse(ctx, ws, u.ID, req.Id, req.Body.MaxSources, req.Body.MaxMinutes, limit)
	if err != nil {
		return nil, err
	}
	out, err := s.toCourse(ctx, c)
	return api.ApproveCourse200JSONResponse(out), err
}

func (s *Server) StartCourseNextPart(ctx context.Context, req api.StartCourseNextPartRequestObject) (api.StartCourseNextPartResponseObject, error) {
	u, _, ws := me(ctx)
	c, err := s.Runs.StartNextPart(ctx, ws, u.ID, req.Id)
	if err != nil {
		return nil, err
	}
	out, err := s.toCourse(ctx, c)
	return api.StartCourseNextPart200JSONResponse(out), err
}

func (s *Server) StopCourse(ctx context.Context, req api.StopCourseRequestObject) (api.StopCourseResponseObject, error) {
	u, _, ws := me(ctx)
	c, err := s.Runs.StopCourse(ctx, ws, u.ID, req.Id)
	if err != nil {
		return nil, err
	}
	out, err := s.toCourse(ctx, c)
	return api.StopCourse200JSONResponse(out), err
}

func (s *Server) AttemptCourseCheck(ctx context.Context, req api.AttemptCourseCheckRequestObject) (api.AttemptCourseCheckResponseObject, error) {
	u, _, ws := me(ctx)
	res, err := s.Runs.AttemptCheck(ctx, ws, u.ID, req.Id, req.CheckId, req.Body.Response, req.Body.Confidence)
	if err != nil {
		return nil, err
	}
	return api.AttemptCourseCheck201JSONResponse(api.CourseAttempt{Id: res.Attempt.ID, Correct: res.Attempt.Correct, Reveal: toCourseReveal(res.Reveal)}), nil
}

func (s *Server) GradeCourseAttempt(ctx context.Context, req api.GradeCourseAttemptRequestObject) (api.GradeCourseAttemptResponseObject, error) {
	u, _, ws := me(ctx)
	if err := s.Runs.GradeShortAnswer(ctx, ws, u.ID, req.Id, req.AttemptId, req.Body.Correct); err != nil {
		return nil, err
	}
	return api.GradeCourseAttempt204Response{}, nil
}

func (s *Server) PostCourseTutorMessage(ctx context.Context, req api.PostCourseTutorMessageRequestObject) (api.PostCourseTutorMessageResponseObject, error) {
	u, _, ws := me(ctx)
	if _, err := s.Runs.PostTutorMessage(ctx, ws, u.ID, req.Id, req.Body.Lesson, req.Body.Body); err != nil {
		return nil, err
	}
	return api.PostCourseTutorMessage202Response{}, nil
}
