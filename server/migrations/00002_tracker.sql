-- +goose Up
-- Phase 2: core tracker (PLAN.md §4). Enumerations are text + CHECK so new
-- values are a one-line migration. Every table carries workspace_id for later
-- multi-workspace support; v1 has exactly one workspace.

CREATE TABLE workspaces (
  id         uuid PRIMARY KEY DEFAULT uuid_v7(),
  name       text NOT NULL,
  slug       text NOT NULL UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
  id            uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id  uuid NOT NULL REFERENCES workspaces(id),
  name          text NOT NULL,
  email         text NOT NULL,
  initials      text NOT NULL,
  role_title    text NOT NULL DEFAULT '',
  avatar_color  text,                       -- optional tint; NULL = neutral token
  password_hash text NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_key ON users (lower(email));

CREATE TABLE sessions (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash   bytea NOT NULL UNIQUE,      -- sha256 of the cookie value
  expires_at   timestamptz NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_idx ON sessions (user_id);

CREATE TABLE projects (
  id              uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id    uuid NOT NULL REFERENCES workspaces(id),
  key             text NOT NULL CHECK (key ~ '^[A-Z][A-Z0-9]{1,9}$'),
  name            text NOT NULL,
  color           text NOT NULL,
  next_ticket_seq integer NOT NULL DEFAULT 1,
  repo_url        text,
  default_branch  text NOT NULL DEFAULT 'main',
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, key)
);

-- Plain records until Phase 4 adds definitions, tokens and runs.
CREATE TABLE agents (
  id              uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id    uuid NOT NULL REFERENCES workspaces(id),
  name            text NOT NULL,
  handle          text NOT NULL CHECK (handle ~ '^[a-z][a-z0-9-]{1,31}$'),
  color_index     integer NOT NULL DEFAULT 0,
  role            text NOT NULL DEFAULT '',
  specialty       text NOT NULL DEFAULT '',
  model           text NOT NULL DEFAULT '',
  status          text NOT NULL DEFAULT 'idle' CHECK (status IN ('running', 'waiting', 'blocked', 'idle')),
  max_concurrency integer NOT NULL DEFAULT 1,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, handle)
);

CREATE TABLE epics (
  id            uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id  uuid NOT NULL REFERENCES workspaces(id),
  project_id    uuid NOT NULL REFERENCES projects(id),
  name          text NOT NULL,
  objective     text NOT NULL DEFAULT '',
  owner_user_id uuid REFERENCES users(id),
  start_date    date,
  target_date   date,
  health        text NOT NULL DEFAULT 'on_track' CHECK (health IN ('on_track', 'at_risk', 'off_track')),
  icon          text,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX epics_project_idx ON epics (project_id);

CREATE TABLE labels (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  name         text NOT NULL,
  color        text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, name)
);

CREATE TABLE tickets (
  id                 uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id       uuid NOT NULL REFERENCES workspaces(id),
  project_id         uuid NOT NULL REFERENCES projects(id),
  number             integer NOT NULL,
  key                text NOT NULL,           -- "PAY-214", fixed at creation
  title              text NOT NULL CHECK (length(title) BETWEEN 1 AND 300),
  description_md     text NOT NULL DEFAULT '',
  status             text NOT NULL DEFAULT 'backlog'
                     CHECK (status IN ('backlog', 'todo', 'in_progress', 'in_review', 'done')),
  priority           text NOT NULL DEFAULT 'none'
                     CHECK (priority IN ('none', 'low', 'medium', 'high', 'urgent')),
  epic_id            uuid REFERENCES epics(id) ON DELETE SET NULL,
  assignee_agent_id  uuid REFERENCES agents(id) ON DELETE SET NULL,
  reviewer_user_id   uuid REFERENCES users(id) ON DELETE SET NULL,
  created_by_user_id uuid REFERENCES users(id),
  estimate           text,
  due_date           date,
  branch             text,
  pr_number          integer,
  pr_url             text,
  pr_state           text CHECK (pr_state IN ('draft', 'open', 'merged', 'closed')),
  pr_additions       integer,
  pr_deletions       integer,
  sort_order         double precision NOT NULL DEFAULT 0,
  search             tsvector GENERATED ALWAYS AS (
                       setweight(to_tsvector('simple', key), 'A') ||
                       setweight(to_tsvector('english', title), 'A') ||
                       setweight(to_tsvector('english', description_md), 'C')
                     ) STORED,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, number),
  UNIQUE (workspace_id, key)
);
CREATE INDEX tickets_project_status_idx ON tickets (project_id, status, sort_order);
CREATE INDEX tickets_assignee_idx ON tickets (assignee_agent_id) WHERE assignee_agent_id IS NOT NULL;
CREATE INDEX tickets_search_idx ON tickets USING gin (search);

CREATE TABLE ticket_labels (
  ticket_id uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
  label_id  uuid NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
  PRIMARY KEY (ticket_id, label_id)
);

CREATE TABLE acceptance_criteria (
  id         uuid PRIMARY KEY DEFAULT uuid_v7(),
  ticket_id  uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
  position   integer NOT NULL,
  text       text NOT NULL CHECK (length(text) BETWEEN 1 AND 1000),
  done       boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX acceptance_criteria_ticket_idx ON acceptance_criteria (ticket_id, position);

CREATE TABLE comments (
  id                uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id      uuid NOT NULL REFERENCES workspaces(id),
  ticket_id         uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
  parent_comment_id uuid REFERENCES comments(id) ON DELETE CASCADE,
  author_type       text NOT NULL CHECK (author_type IN ('user', 'agent')),
  author_id         uuid NOT NULL,
  body_md           text NOT NULL CHECK (length(body_md) BETWEEN 1 AND 20000),
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX comments_ticket_idx ON comments (ticket_id, created_at);

-- Append-only. The ticket timeline, dashboard attention list and Inbox are
-- queries over this table. Written in the same transaction as the change.
CREATE TABLE activity_events (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  ticket_id    uuid REFERENCES tickets(id) ON DELETE CASCADE,
  epic_id      uuid REFERENCES epics(id) ON DELETE SET NULL,
  actor_type   text NOT NULL CHECK (actor_type IN ('user', 'agent', 'system')),
  actor_id     uuid,
  verb         text NOT NULL,
  payload      jsonb NOT NULL DEFAULT '{}',
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX activity_ticket_idx ON activity_events (ticket_id, created_at);
CREATE INDEX activity_workspace_idx ON activity_events (workspace_id, created_at DESC);

CREATE TRIGGER workspaces_touch BEFORE UPDATE ON workspaces FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER users_touch BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER projects_touch BEFORE UPDATE ON projects FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER agents_touch BEFORE UPDATE ON agents FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER epics_touch BEFORE UPDATE ON epics FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER labels_touch BEFORE UPDATE ON labels FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER tickets_touch BEFORE UPDATE ON tickets FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER criteria_touch BEFORE UPDATE ON acceptance_criteria FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER comments_touch BEFORE UPDATE ON comments FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- +goose Down
DROP TABLE activity_events, comments, acceptance_criteria, ticket_labels, tickets,
  labels, epics, agents, projects, sessions, users, workspaces;
