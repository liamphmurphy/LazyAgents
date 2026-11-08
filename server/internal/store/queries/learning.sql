-- name: CreateLearningCourse :one
INSERT INTO learning_courses (workspace_id, user_id, chat_id, topic, goal, level, minutes_budget)
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: GetLearningCourse :one
SELECT * FROM learning_courses WHERE id = $1 AND workspace_id = $2 AND user_id = $3;

-- name: LockLearningCourse :one
SELECT * FROM learning_courses WHERE id = $1 FOR UPDATE;

-- name: ListLearningCourses :many
SELECT * FROM learning_courses WHERE workspace_id = $1 AND user_id = $2 ORDER BY created_at DESC, id DESC LIMIT 100;

-- name: ActiveLearningCourses :many
SELECT id FROM learning_courses WHERE state IN ('scoping', 'researching', 'authoring', 'verifying') ORDER BY created_at LIMIT 50;

-- name: LearningCourseForChat :one
SELECT * FROM learning_courses WHERE chat_id = $1;

-- name: SetLearningCourseOutline :one
UPDATE learning_courses SET outline = $2 WHERE id = $1 RETURNING *;

-- name: SetLearningCourseLimits :exec
UPDATE learning_courses SET max_sources = $2, max_minutes = $3, max_cost_micros = $4, research_job_id = $5 WHERE id = $1;

-- name: SetLearningCourseState :one
UPDATE learning_courses SET state = $2, error = $3, round = $4,
  ready_at = CASE WHEN $2 = 'ready' THEN now() ELSE ready_at END
WHERE id = $1 RETURNING *;

-- name: SetLearningCurrentLesson :exec
UPDATE learning_courses SET current_lesson = $2 WHERE id = $1;

-- name: CreateLearningRun :exec
INSERT INTO learning_runs (run_id, course_id, role, round, unit_seq) VALUES ($1, $2, $3, $4, $5);

-- name: GetLearningRun :one
SELECT * FROM learning_runs WHERE run_id = $1;

-- name: ListLearningRuns :many
SELECT l.*, r.state AS run_state FROM learning_runs l JOIN runs r ON r.id = l.run_id WHERE l.course_id = $1 ORDER BY r.created_at, l.run_id;

-- name: LearningCourseForRun :one
SELECT c.* FROM learning_courses c JOIN learning_runs l ON l.course_id = c.id WHERE l.run_id = $1;

-- name: OwnsLearningRun :one
SELECT EXISTS (SELECT 1 FROM learning_courses c JOIN learning_runs l ON l.course_id = c.id WHERE l.run_id = $1 AND c.user_id = $2)::boolean;

-- name: LearningCourseSpend :one
-- What the course's own runs and its researchers have cost so far.
SELECT coalesce(sum(r.cost_usd_micros), 0)::bigint FROM runs r WHERE r.id IN (
  SELECT l.run_id FROM learning_runs l WHERE l.course_id = $1
  UNION SELECT t.run_id FROM research_tasks t JOIN learning_courses c ON c.research_job_id = t.job_id WHERE c.id = $1);

-- name: CreateLearningLesson :exec
INSERT INTO learning_lessons (course_id, seq, unit_seq, key, title, objectives, prereqs, minutes, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ListLearningLessons :many
SELECT * FROM learning_lessons WHERE course_id = $1 ORDER BY seq;

-- name: GetLearningLessonByKey :one
SELECT * FROM learning_lessons WHERE course_id = $1 AND key = $2;

-- name: SetLearningLessonContent :exec
UPDATE learning_lessons SET body = $2, minutes = $3, status = 'written' WHERE id = $1;

-- name: SetLearningLessonStatus :exec
UPDATE learning_lessons SET status = $2, issues = $3 WHERE id = $1;

-- name: DeleteLearningChecks :exec
DELETE FROM learning_checks WHERE lesson_id = $1;

-- name: CreateLearningCheck :exec
INSERT INTO learning_checks (lesson_id, seq, objective, kind, prompt, options, answer, rubric, explanation, sources)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: ListLearningChecks :many
SELECT k.* FROM learning_checks k JOIN learning_lessons l ON l.id = k.lesson_id WHERE l.course_id = $1 ORDER BY l.seq, k.seq;

-- name: GetLearningCheck :one
SELECT k.* FROM learning_checks k JOIN learning_lessons l ON l.id = k.lesson_id WHERE k.id = $1 AND l.course_id = $2;

-- name: CreateLearningAttempt :one
INSERT INTO learning_attempts (check_id, user_id, response, correct, confidence) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: LockLearningAttempt :one
SELECT a.* FROM learning_attempts a JOIN learning_checks k ON k.id = a.check_id JOIN learning_lessons l ON l.id = k.lesson_id
WHERE a.id = $1 AND a.user_id = $2 AND l.course_id = $3 FOR UPDATE OF a;

-- name: SetLearningAttemptCorrect :exec
UPDATE learning_attempts SET correct = $2 WHERE id = $1;

-- name: ListLearningAttempts :many
-- Oldest first, so the first with a verdict is what "right first try" reads.
SELECT a.* FROM learning_attempts a JOIN learning_checks k ON k.id = a.check_id JOIN learning_lessons l ON l.id = k.lesson_id
WHERE l.course_id = $1 AND a.user_id = $2 ORDER BY a.id;

-- name: CountLearningAttempts :one
SELECT count(*)::int FROM learning_attempts WHERE check_id = $1 AND user_id = $2;

-- name: CreateLearningSource :exec
INSERT INTO learning_sources (course_id, n, title, url) VALUES ($1, $2, $3, $4);

-- name: ListLearningSources :many
SELECT * FROM learning_sources WHERE course_id = $1 ORDER BY n;

-- name: LearningSourceForRun :one
-- Whether a reviewer may open this address: only the course's own sources.
SELECT EXISTS (SELECT 1 FROM learning_sources s JOIN learning_runs l ON l.course_id = s.course_id WHERE l.run_id = $1 AND s.url = $2)::boolean;
