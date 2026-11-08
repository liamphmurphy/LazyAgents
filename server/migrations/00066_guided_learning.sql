-- +goose Up
-- Guided Learning: a module built from a topic by a pipeline Vesper coordinates
-- (scope, approve, research, author, review), then studied with checks and a tutor.
ALTER TABLE runs DROP CONSTRAINT runs_purpose_check;
ALTER TABLE runs ADD CONSTRAINT runs_purpose_check CHECK (purpose IN ('ticket', 'planning', 'chat', 'research', 'workflow', 'learning'));

-- A course job stops at 'ready': the course takes the findings, not a persona.
ALTER TABLE research_jobs ADD COLUMN kind text NOT NULL DEFAULT 'chat' CHECK (kind IN ('chat', 'course'));

CREATE TABLE learning_courses (
  id              uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id    uuid NOT NULL REFERENCES workspaces(id),
  user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  -- The hidden tutor chat; it also holds the course's project and model.
  chat_id         uuid NOT NULL UNIQUE REFERENCES chat_sessions(id) ON DELETE CASCADE,
  research_job_id uuid REFERENCES research_jobs(id) ON DELETE SET NULL,
  topic           text NOT NULL CHECK (length(topic) BETWEEN 1 AND 300),
  goal            text NOT NULL DEFAULT '',
  level           text NOT NULL CHECK (level IN ('new', 'some', 'experienced')),
  minutes_budget  integer NOT NULL,
  state           text NOT NULL DEFAULT 'scoping' CHECK (state IN ('scoping', 'outline_ready', 'researching', 'authoring', 'verifying', 'ready', 'stopped', 'failed')),
  error           text NOT NULL DEFAULT '',
  -- 0 is the first pass; 1 is the one rewrite of what review failed.
  round           integer NOT NULL DEFAULT 0,
  max_sources     integer NOT NULL DEFAULT 40,
  max_minutes     integer NOT NULL DEFAULT 30,
  -- NULL is unlimited, as for research.
  max_cost_micros bigint,
  outline         jsonb,
  current_lesson  text NOT NULL DEFAULT '',
  created_at      timestamptz NOT NULL DEFAULT now(),
  ready_at        timestamptz
);
CREATE INDEX learning_courses_user ON learning_courses (workspace_id, user_id, created_at DESC);
CREATE INDEX learning_courses_active ON learning_courses (state) WHERE state IN ('scoping', 'researching', 'authoring', 'verifying');

CREATE TABLE learning_runs (
  run_id    uuid PRIMARY KEY REFERENCES runs(id) ON DELETE CASCADE,
  course_id uuid NOT NULL REFERENCES learning_courses(id) ON DELETE CASCADE,
  role      text NOT NULL CHECK (role IN ('scope', 'author', 'review')),
  round     integer NOT NULL DEFAULT 0,
  unit_seq  integer NOT NULL DEFAULT 0
);
CREATE INDEX learning_runs_course ON learning_runs (course_id, role, round);

CREATE TABLE learning_lessons (
  id         uuid PRIMARY KEY DEFAULT uuid_v7(),
  course_id  uuid NOT NULL REFERENCES learning_courses(id) ON DELETE CASCADE,
  seq        integer NOT NULL,
  unit_seq   integer NOT NULL,
  key        text NOT NULL,
  title      text NOT NULL,
  objectives text[] NOT NULL,
  prereqs    text[] NOT NULL,
  minutes    integer NOT NULL DEFAULT 0,
  status     text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'written', 'verified', 'flagged', 'known')),
  body       jsonb NOT NULL DEFAULT '[]',
  issues     text[] NOT NULL DEFAULT '{}',
  UNIQUE (course_id, key),
  UNIQUE (course_id, seq)
);

CREATE TABLE learning_checks (
  id          uuid PRIMARY KEY DEFAULT uuid_v7(),
  lesson_id   uuid NOT NULL REFERENCES learning_lessons(id) ON DELETE CASCADE,
  seq         integer NOT NULL,
  objective   text NOT NULL,
  kind        text NOT NULL CHECK (kind IN ('mcq', 'order', 'short')),
  prompt      text NOT NULL,
  -- mcq: the choices; order: the items in their correct order; short: empty.
  options     text[] NOT NULL DEFAULT '{}',
  -- mcq: the right choice; short: a model answer; order: empty.
  answer      text NOT NULL DEFAULT '',
  rubric      text NOT NULL DEFAULT '',
  explanation text NOT NULL,
  sources     integer[] NOT NULL DEFAULT '{}',
  UNIQUE (lesson_id, seq)
);

CREATE TABLE learning_attempts (
  id         uuid PRIMARY KEY DEFAULT uuid_v7(),
  check_id   uuid NOT NULL REFERENCES learning_checks(id) ON DELETE CASCADE,
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  response   text[] NOT NULL,
  -- NULL: a short answer the person hasn't marked yet.
  correct    boolean,
  confidence smallint NOT NULL DEFAULT 0 CHECK (confidence BETWEEN 0 AND 3),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX learning_attempts_check ON learning_attempts (check_id, user_id, id);

CREATE TABLE learning_sources (
  course_id uuid NOT NULL REFERENCES learning_courses(id) ON DELETE CASCADE,
  n         integer NOT NULL,
  title     text NOT NULL,
  url       text NOT NULL,
  PRIMARY KEY (course_id, n),
  UNIQUE (course_id, url)
);

-- +goose Down
DROP TABLE learning_sources;
DROP TABLE learning_attempts;
DROP TABLE learning_checks;
DROP TABLE learning_lessons;
DROP TABLE learning_runs;
DROP TABLE learning_courses;
ALTER TABLE research_jobs DROP COLUMN kind;
DELETE FROM runs WHERE purpose = 'learning';
ALTER TABLE runs DROP CONSTRAINT runs_purpose_check;
ALTER TABLE runs ADD CONSTRAINT runs_purpose_check CHECK (purpose IN ('ticket', 'planning', 'chat', 'research', 'workflow'));
