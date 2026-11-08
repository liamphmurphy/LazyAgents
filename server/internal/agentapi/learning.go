package agentapi

import (
	"context"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/runs"
)

func courseRun(c agents.Caller) error {
	if c.RunID == nil {
		return apperr.Conflict("this tool only works in a course run")
	}
	return nil
}

func submitOutline(ctx context.Context, d Deps, c agents.Caller, in runs.CourseOutline) (any, error) {
	if err := courseRun(c); err != nil {
		return nil, err
	}
	n, err := d.Runs.SubmitCourseOutline(ctx, *c.RunID, in)
	if err != nil {
		return nil, err
	}
	return map[string]any{"saved": true, "lessons": n}, nil
}

func submitLesson(ctx context.Context, d Deps, c agents.Caller, in runs.LessonSubmission) (any, error) {
	if err := courseRun(c); err != nil {
		return nil, err
	}
	if err := d.Runs.SubmitCourseLesson(ctx, *c.RunID, in); err != nil {
		return nil, err
	}
	return map[string]any{"saved": true}, nil
}

type SubmitReviewIn struct {
	Reviews []runs.LessonReview `json:"reviews" jsonschema:"One verdict per lesson you reviewed"`
}

func submitReview(ctx context.Context, d Deps, c agents.Caller, in SubmitReviewIn) (any, error) {
	if err := courseRun(c); err != nil {
		return nil, err
	}
	if err := d.Runs.SubmitCourseReviews(ctx, *c.RunID, in.Reviews); err != nil {
		return nil, err
	}
	return map[string]any{"saved": len(in.Reviews)}, nil
}
