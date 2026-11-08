-- +goose Up
-- Baseline: the schema produced by migrations 00001–00040, squashed on
-- 2026-11-07 with pg_dump --schema-only (River's own objects excluded; River
-- migrates itself). Databases already at version 40 skip this file. A database
-- at an older version must first be upgraded with a pre-squash build.
-- +goose StatementBegin
DO $guard$
BEGIN
  IF to_regclass('public.workspaces') IS NOT NULL THEN
    RAISE EXCEPTION 'vesper: this database predates the 00040 baseline; upgrade it with a build from before the migration squash first';
  END IF;
END
$guard$;

--
-- Name: pg_trgm; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;

--
-- Name: memory_search_vector(text, text, text, text[]); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.memory_search_vector(title text, summary text, body text, tags text[]) RETURNS tsvector
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$
  SELECT setweight(to_tsvector('english'::regconfig, title), 'A')
      || setweight(to_tsvector('english'::regconfig, array_to_string(tags, ' ')), 'A')
      || setweight(to_tsvector('english'::regconfig, summary), 'B')
      || setweight(to_tsvector('english'::regconfig, body), 'C')
$$;

--
-- Name: notify_change(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.notify_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 PERFORM pg_notify('lazyagents_changes', json_build_object('type', TG_ARGV[0], 'ids', ARRAY[]::text[])::text);
 RETURN NULL;
END;
$$;

--
-- Name: notify_chat_change(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.notify_chat_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE ids text[];
BEGIN
 IF TG_OP = 'DELETE' THEN
  SELECT coalesce(array_agg(i), ARRAY[]::text[]) INTO ids FROM (SELECT DISTINCT to_jsonb(r)->>TG_ARGV[1] AS i FROM old_rows r LIMIT 50) s;
 ELSE
  SELECT coalesce(array_agg(i), ARRAY[]::text[]) INTO ids FROM (SELECT DISTINCT to_jsonb(r)->>TG_ARGV[1] AS i FROM new_rows r LIMIT 50) s;
 END IF;
 PERFORM pg_notify('lazyagents_changes', json_build_object('type', TG_ARGV[0], 'ids', ids)::text);
 RETURN NULL;
END;
$$;

--
-- Name: notify_entity_change(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.notify_entity_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE ids text[];
DECLARE cols text[] := TG_ARGV[1:TG_NARGS - 1];
BEGIN
 IF TG_OP = 'DELETE' THEN
  SELECT array_agg(i) INTO ids FROM (SELECT DISTINCT to_jsonb(r)->>c AS i FROM old_rows r CROSS JOIN unnest(cols) c WHERE to_jsonb(r)->>c IS NOT NULL LIMIT 51) s;
 ELSE
  SELECT array_agg(i) INTO ids FROM (SELECT DISTINCT to_jsonb(r)->>c AS i FROM new_rows r CROSS JOIN unnest(cols) c WHERE to_jsonb(r)->>c IS NOT NULL LIMIT 51) s;
 END IF;
 IF ids IS NULL THEN
  RETURN NULL;
 END IF;
 IF cardinality(ids) > 50 THEN
  ids := ARRAY[]::text[];
 END IF;
 PERFORM pg_notify('lazyagents_changes', json_build_object('type', TG_ARGV[0], 'ids', ids)::text);
 RETURN NULL;
END;
$$;

--
-- Name: notify_ticket_epic_change(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.notify_ticket_epic_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE ids text[];
BEGIN
 IF TG_OP = 'INSERT' THEN
  SELECT array_agg(i) INTO ids FROM (SELECT DISTINCT epic_id::text AS i FROM new_rows WHERE epic_id IS NOT NULL LIMIT 51) s;
 ELSIF TG_OP = 'DELETE' THEN
  SELECT array_agg(i) INTO ids FROM (SELECT DISTINCT epic_id::text AS i FROM old_rows WHERE epic_id IS NOT NULL LIMIT 51) s;
 ELSE
  SELECT array_agg(i) INTO ids FROM (
   SELECT DISTINCT e::text AS i
   FROM old_rows o JOIN new_rows n ON n.id = o.id
   CROSS JOIN LATERAL (VALUES (o.epic_id), (n.epic_id)) v(e)
   WHERE e IS NOT NULL
     AND (o.epic_id IS DISTINCT FROM n.epic_id
       OR o.status IS DISTINCT FROM n.status
       OR o.estimate_points IS DISTINCT FROM n.estimate_points)
   LIMIT 51) s;
 END IF;
 IF ids IS NULL THEN
  RETURN NULL;
 END IF;
 IF cardinality(ids) > 50 THEN
  ids := ARRAY[]::text[];
 END IF;
 PERFORM pg_notify('lazyagents_changes', json_build_object('type', 'epic.updated', 'ids', ids)::text);
 RETURN NULL;
END;
$$;

--
-- Name: touch_updated_at(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.touch_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END $$;

--
-- Name: uuid_v7(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.uuid_v7() RETURNS uuid
    LANGUAGE plpgsql
    AS $$
DECLARE
  ms bigint := floor(extract(epoch FROM clock_timestamp()) * 1000);
  b  bytea  := uuid_send(gen_random_uuid()); -- 16 random bytes, no pgcrypto needed
BEGIN
  b := set_byte(b, 0, ((ms >> 40) & 255)::int);
  b := set_byte(b, 1, ((ms >> 32) & 255)::int);
  b := set_byte(b, 2, ((ms >> 24) & 255)::int);
  b := set_byte(b, 3, ((ms >> 16) & 255)::int);
  b := set_byte(b, 4, ((ms >> 8) & 255)::int);
  b := set_byte(b, 5, (ms & 255)::int);
  b := set_byte(b, 6, (get_byte(b, 6) & 15) | 112); -- version 7
  b := set_byte(b, 8, (get_byte(b, 8) & 63) | 128); -- RFC 4122 variant
  RETURN encode(b, 'hex')::uuid;
END $$;

--
-- Name: acceptance_criteria; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.acceptance_criteria (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    ticket_id uuid NOT NULL,
    "position" integer NOT NULL,
    text text NOT NULL,
    done boolean DEFAULT false NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT acceptance_criteria_text_check CHECK (((length(text) >= 1) AND (length(text) <= 1000)))
);

--
-- Name: activity_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.activity_events (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    workspace_id uuid NOT NULL,
    ticket_id uuid,
    epic_id uuid,
    actor_type text NOT NULL,
    actor_id uuid,
    verb text NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    seq bigint NOT NULL,
    CONSTRAINT activity_events_actor_type_check CHECK ((actor_type = ANY (ARRAY['user'::text, 'agent'::text, 'system'::text])))
);

--
-- Name: activity_events_seq_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.activity_events ALTER COLUMN seq ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.activity_events_seq_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

--
-- Name: agent_definition_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agent_definition_versions (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    agent_id uuid NOT NULL,
    version integer NOT NULL,
    spec jsonb NOT NULL,
    note text DEFAULT ''::text NOT NULL,
    created_by_user_id uuid,
    created_at timestamptz DEFAULT now() NOT NULL
);

--
-- Name: agent_project_overrides; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agent_project_overrides (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    agent_id uuid NOT NULL,
    project_id uuid NOT NULL,
    spec jsonb NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    updated_by_user_id uuid,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL
);

--
-- Name: agent_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agent_tokens (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    agent_id uuid NOT NULL,
    token_hash bytea NOT NULL,
    prefix text NOT NULL,
    kind text NOT NULL,
    label text DEFAULT ''::text NOT NULL,
    run_id uuid,
    created_by_user_id uuid,
    created_at timestamptz DEFAULT now() NOT NULL,
    expires_at timestamptz,
    last_used_at timestamptz,
    revoked_at timestamptz,
    CONSTRAINT agent_tokens_kind_check CHECK ((kind = ANY (ARRAY['run'::text, 'unmanaged'::text])))
);

--
-- Name: agents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agents (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    workspace_id uuid NOT NULL,
    name text NOT NULL,
    handle text NOT NULL,
    color_index integer DEFAULT 0 NOT NULL,
    role text DEFAULT ''::text NOT NULL,
    specialty text DEFAULT ''::text NOT NULL,
    model text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'idle'::text NOT NULL,
    max_concurrency integer DEFAULT 1 NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    current_definition_version_id uuid,
    kind text DEFAULT 'main'::text NOT NULL,
    archived_at timestamptz,
    CONSTRAINT agents_handle_check CHECK ((handle ~ '^[a-z][a-z0-9-]{1,31}$'::text)),
    CONSTRAINT agents_kind_check CHECK ((kind = ANY (ARRAY['main'::text, 'sub'::text, 'assistant'::text]))),
    CONSTRAINT agents_status_check CHECK ((status = ANY (ARRAY['running'::text, 'waiting'::text, 'blocked'::text, 'idle'::text])))
);

--
-- Name: approval_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.approval_requests (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    workspace_id uuid NOT NULL,
    run_id uuid NOT NULL,
    ticket_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    scope text NOT NULL,
    action_summary text NOT NULL,
    action_detail jsonb DEFAULT '{}'::jsonb NOT NULL,
    risk_summary text DEFAULT ''::text NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    decided_by_user_id uuid,
    decided_at timestamptz,
    decision_note text DEFAULT ''::text NOT NULL,
    executed_at timestamptz,
    result jsonb,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT approval_requests_action_summary_check CHECK (((length(action_summary) >= 1) AND (length(action_summary) <= 300))),
    CONSTRAINT approval_requests_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'approved'::text, 'denied'::text, 'expired'::text])))
);

--
-- Name: artifact_aliases; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.artifact_aliases (
    workspace_id uuid NOT NULL,
    artifact_id uuid NOT NULL,
    path text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL
);

--
-- Name: artifact_reads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.artifact_reads (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    version_id uuid NOT NULL,
    section_id uuid,
    agent_id uuid NOT NULL,
    run_id uuid,
    read_at timestamptz DEFAULT now() NOT NULL
);

--
-- Name: artifact_sections; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.artifact_sections (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    version_id uuid NOT NULL,
    "position" integer NOT NULL,
    level integer NOT NULL,
    anchor text NOT NULL,
    heading text NOT NULL,
    CONSTRAINT artifact_sections_level_check CHECK (((level >= 1) AND (level <= 6)))
);

--
-- Name: artifact_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.artifact_versions (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    artifact_id uuid NOT NULL,
    seq integer NOT NULL,
    label text NOT NULL,
    body_md text,
    blob_key text,
    filename text,
    content_type text DEFAULT 'text/markdown'::text NOT NULL,
    size_bytes bigint DEFAULT 0 NOT NULL,
    note text DEFAULT ''::text NOT NULL,
    created_by_type text NOT NULL,
    created_by_id uuid NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    state text DEFAULT 'published'::text NOT NULL,
    based_on_version_id uuid,
    decided_by_id uuid,
    decided_at timestamptz,
    review_state text,
    reviewed_by_user_id uuid,
    reviewed_at timestamptz,
    review_note text DEFAULT ''::text NOT NULL,
    review_pins jsonb DEFAULT '[]'::jsonb NOT NULL,
    CONSTRAINT artifact_versions_check CHECK (((body_md IS NULL) <> (blob_key IS NULL))),
    CONSTRAINT artifact_versions_created_by_type_check CHECK ((created_by_type = ANY (ARRAY['user'::text, 'agent'::text]))),
    CONSTRAINT artifact_versions_review_state_check CHECK ((review_state = ANY (ARRAY['pending'::text, 'approved'::text, 'changes_requested'::text]))),
    CONSTRAINT artifact_versions_state_check CHECK ((state = ANY (ARRAY['published'::text, 'suggested'::text, 'rejected'::text])))
);

--
-- Name: artifacts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.artifacts (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    workspace_id uuid NOT NULL,
    project_id uuid,
    epic_id uuid,
    type text NOT NULL,
    title text NOT NULL,
    number integer,
    summary text DEFAULT ''::text NOT NULL,
    author_type text NOT NULL,
    author_id uuid NOT NULL,
    current_version_id uuid,
    search_text text DEFAULT ''::text NOT NULL,
    search tsvector GENERATED ALWAYS AS (((setweight(to_tsvector('english'::regconfig, title), 'A'::"char") || setweight(to_tsvector('english'::regconfig, summary), 'B'::"char")) || setweight(to_tsvector('english'::regconfig, search_text), 'C'::"char"))) STORED,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    path text,
    CONSTRAINT artifacts_author_type_check CHECK ((author_type = ANY (ARRAY['user'::text, 'agent'::text]))),
    CONSTRAINT artifacts_path_only_notes CHECK (((type = 'note'::text) = (path IS NOT NULL))),
    CONSTRAINT artifacts_project_required CHECK (((type = 'note'::text) OR (project_id IS NOT NULL))),
    CONSTRAINT artifacts_title_check CHECK (((length(title) >= 1) AND (length(title) <= 300))),
    CONSTRAINT artifacts_type_check CHECK ((type = ANY (ARRAY['prd'::text, 'spec'::text, 'adr'::text, 'report'::text, 'design'::text, 'note'::text, 'mockup'::text])))
);

--
-- Name: chat_attachments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.chat_attachments (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    workspace_id uuid NOT NULL,
    user_id uuid NOT NULL,
    message_id uuid,
    filename text NOT NULL,
    content_type text NOT NULL,
    size_bytes bigint NOT NULL,
    blob_key text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    source text DEFAULT 'upload'::text NOT NULL,
    CONSTRAINT chat_attachments_filename_check CHECK (((length(filename) >= 1) AND (length(filename) <= 255))),
    CONSTRAINT chat_attachments_size_bytes_check CHECK ((size_bytes > 0)),
    CONSTRAINT chat_attachments_source_check CHECK ((source = ANY (ARRAY['upload'::text, 'generated'::text])))
);

--
-- Name: chat_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.chat_messages (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    session_id uuid NOT NULL,
    seq bigint NOT NULL,
    role text NOT NULL,
    body_md text NOT NULL,
    harness text,
    model text,
    run_id uuid,
    run_turn_id uuid,
    context_refs jsonb DEFAULT '[]'::jsonb NOT NULL,
    citations jsonb DEFAULT '[]'::jsonb NOT NULL,
    usage jsonb,
    created_at timestamptz DEFAULT now() NOT NULL,
    code_changes jsonb DEFAULT '[]'::jsonb NOT NULL,
    CONSTRAINT chat_messages_role_check CHECK ((role = ANY (ARRAY['user'::text, 'assistant'::text, 'system'::text])))
);

--
-- Name: chat_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.chat_sessions (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    workspace_id uuid NOT NULL,
    project_id uuid NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    created_by uuid NOT NULL,
    agent_id uuid NOT NULL,
    harness text NOT NULL,
    model text NOT NULL,
    current_run_id uuid,
    state text DEFAULT 'idle'::text NOT NULL,
    archived boolean DEFAULT false NOT NULL,
    revision integer DEFAULT 1 NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    last_message_at timestamptz,
    persona_id uuid,
    repo_url text,
    work_branch text,
    CONSTRAINT chat_sessions_repo_branch CHECK (((repo_url IS NULL) = (work_branch IS NULL))),
    CONSTRAINT chat_sessions_state_check CHECK ((state = ANY (ARRAY['idle'::text, 'running'::text, 'failed'::text])))
);

--
-- Name: comments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comments (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    workspace_id uuid NOT NULL,
    ticket_id uuid NOT NULL,
    parent_comment_id uuid,
    author_type text NOT NULL,
    author_id uuid NOT NULL,
    body_md text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT comments_author_type_check CHECK ((author_type = ANY (ARRAY['user'::text, 'agent'::text]))),
    CONSTRAINT comments_body_md_check CHECK (((length(body_md) >= 1) AND (length(body_md) <= 20000)))
);

--
-- Name: epic_criteria; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.epic_criteria (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    epic_id uuid NOT NULL,
    "position" integer NOT NULL,
    text text NOT NULL,
    done boolean DEFAULT false NOT NULL,
    checked_by_type text,
    checked_by_id uuid,
    checked_at timestamptz,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT epic_criteria_checked_by_type_check CHECK ((checked_by_type = ANY (ARRAY['user'::text, 'agent'::text]))),
    CONSTRAINT epic_criteria_position_check CHECK (("position" >= 1)),
    CONSTRAINT epic_criteria_text_check CHECK (((length(text) >= 1) AND (length(text) <= 1000)))
);

--
-- Name: epics; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.epics (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    workspace_id uuid NOT NULL,
    project_id uuid NOT NULL,
    name text NOT NULL,
    objective text DEFAULT ''::text NOT NULL,
    owner_user_id uuid,
    start_date date,
    target_date date,
    health text DEFAULT 'on_track'::text NOT NULL,
    icon text,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    requires_mockup boolean DEFAULT false NOT NULL,
    status text DEFAULT 'open'::text NOT NULL,
    CONSTRAINT epics_health_check CHECK ((health = ANY (ARRAY['on_track'::text, 'at_risk'::text, 'off_track'::text]))),
    CONSTRAINT epics_status_check CHECK ((status = ANY (ARRAY['open'::text, 'done'::text])))
);

--
-- Name: inbox_reads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.inbox_reads (
    user_id uuid NOT NULL,
    item_id text NOT NULL,
    read_at timestamptz DEFAULT now() NOT NULL
);

--
-- Name: knowledge_links; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.knowledge_links (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    workspace_id uuid NOT NULL,
    source_kind text NOT NULL,
    source_id uuid NOT NULL,
    source_version_id uuid,
    source_ticket_id uuid,
    target_kind text NOT NULL,
    target_id uuid,
    target_anchor text DEFAULT ''::text NOT NULL,
    target_text text NOT NULL,
    raw_text text NOT NULL,
    snippet text DEFAULT ''::text NOT NULL,
    "position" integer NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT knowledge_links_check CHECK (((target_kind = 'unresolved'::text) = (target_id IS NULL))),
    CONSTRAINT knowledge_links_source_kind_check CHECK ((source_kind = ANY (ARRAY['artifact'::text, 'ticket'::text, 'comment'::text]))),
    CONSTRAINT knowledge_links_target_kind_check CHECK ((target_kind = ANY (ARRAY['artifact'::text, 'section'::text, 'ticket'::text, 'epic'::text, 'agent'::text, 'unresolved'::text])))
);

--
-- Name: labels; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.labels (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    workspace_id uuid NOT NULL,
    name text NOT NULL,
    color text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL
);

--
-- Name: memories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.memories (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    workspace_id uuid NOT NULL,
    user_id uuid NOT NULL,
    title text NOT NULL,
    summary text DEFAULT ''::text NOT NULL,
    body text DEFAULT ''::text NOT NULL,
    tags text[] DEFAULT '{}'::text[] NOT NULL,
    type text DEFAULT 'other'::text NOT NULL,
    source text DEFAULT ''::text NOT NULL,
    source_chat_id uuid,
    source_run_id uuid,
    superseded_by uuid,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    retired_reason text DEFAULT ''::text NOT NULL,
    retired_at timestamptz,
    consolidation_exempt boolean DEFAULT false NOT NULL,
    CONSTRAINT memories_body_check CHECK ((length(body) <= 20000)),
    CONSTRAINT memories_check CHECK ((superseded_by IS DISTINCT FROM id)),
    CONSTRAINT memories_retired_reason_check CHECK ((length(retired_reason) <= 300)),
    CONSTRAINT memories_summary_check CHECK ((length(summary) <= 300)),
    CONSTRAINT memories_title_check CHECK (((length(btrim(title)) >= 1) AND (length(btrim(title)) <= 200))),
    CONSTRAINT memories_type_check CHECK ((type = ANY (ARRAY['preference'::text, 'project'::text, 'person'::text, 'life'::text, 'work'::text, 'fact'::text, 'other'::text])))
);

--
-- Name: metric_snapshots; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.metric_snapshots (
    workspace_id uuid NOT NULL,
    range text NOT NULL,
    payload jsonb NOT NULL,
    generated_at timestamptz NOT NULL,
    CONSTRAINT metric_snapshots_range_check CHECK ((range = ANY (ARRAY['24h'::text, '7d'::text, '30d'::text])))
);

--
-- Name: personas; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.personas (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    workspace_id uuid NOT NULL,
    user_id uuid NOT NULL,
    name text NOT NULL,
    tone text DEFAULT 'neutral'::text NOT NULL,
    color text DEFAULT '#64748b'::text NOT NULL,
    avatar_key text,
    avatar_content_type text,
    custom_instructions text DEFAULT ''::text NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    last_used_at timestamptz,
    revision integer DEFAULT 1 NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    can_code boolean DEFAULT false NOT NULL,
    can_generate_images boolean DEFAULT false NOT NULL,
    repos text[] DEFAULT '{}'::text[] NOT NULL,
    CONSTRAINT personas_check CHECK (((avatar_key IS NULL) = (avatar_content_type IS NULL))),
    CONSTRAINT personas_color_check CHECK ((color ~ '^#[0-9a-f]{6}$'::text)),
    CONSTRAINT personas_custom_instructions_check CHECK ((length(custom_instructions) <= 2000)),
    CONSTRAINT personas_name_check CHECK (((length(btrim(name)) >= 1) AND (length(btrim(name)) <= 60))),
    CONSTRAINT personas_repos_check CHECK ((cardinality(repos) <= 10)),
    CONSTRAINT personas_tone_check CHECK ((tone = ANY (ARRAY['neutral'::text, 'friendly'::text, 'concise'::text, 'formal'::text, 'playful'::text])))
);

--
-- Name: plan_steps; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.plan_steps (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    run_id uuid NOT NULL,
    "position" integer NOT NULL,
    text text NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    CONSTRAINT plan_steps_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'current'::text, 'done'::text, 'needs_human'::text, 'failed'::text]))),
    CONSTRAINT plan_steps_text_check CHECK (((length(text) >= 1) AND (length(text) <= 500)))
);

--
-- Name: planning_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.planning_sessions (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    workspace_id uuid NOT NULL,
    project_id uuid NOT NULL,
    epic_id uuid,
    artifact_id uuid,
    source_version_id uuid,
    goal text NOT NULL,
    run_id uuid NOT NULL,
    state text DEFAULT 'planning'::text NOT NULL,
    proposals jsonb DEFAULT '[]'::jsonb NOT NULL,
    ticket_keys text[] DEFAULT '{}'::text[] NOT NULL,
    revision integer DEFAULT 1 NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    context_refs jsonb DEFAULT '[]'::jsonb NOT NULL,
    proposal_count integer DEFAULT 0 NOT NULL,
    CONSTRAINT planning_sessions_state_check CHECK ((state = ANY (ARRAY['planning'::text, 'review'::text, 'accepted'::text, 'dismissed'::text])))
);

--
-- Name: projects; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.projects (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    workspace_id uuid NOT NULL,
    key text NOT NULL,
    name text NOT NULL,
    color text NOT NULL,
    next_ticket_seq integer DEFAULT 1 NOT NULL,
    repo_url text,
    default_branch text DEFAULT 'main'::text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    archived boolean DEFAULT false NOT NULL,
    revision integer DEFAULT 1 NOT NULL,
    CONSTRAINT projects_key_check CHECK ((key ~ '^[A-Z][A-Z0-9]{1,9}$'::text))
);

--
-- Name: provider_limits; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.provider_limits (
    workspace_id uuid NOT NULL,
    harness text NOT NULL,
    until timestamptz NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL
);

--
-- Name: run_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.run_events (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    run_id uuid NOT NULL,
    seq bigint NOT NULL,
    type text NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    at timestamptz DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT run_events_type_check CHECK ((type = ANY (ARRAY['message'::text, 'tool_call'::text, 'tool_result'::text, 'usage'::text, 'error'::text, 'lifecycle'::text])))
);

--
-- Name: run_events_seq_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.run_events ALTER COLUMN seq ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.run_events_seq_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

--
-- Name: run_turns; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.run_turns (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    run_id uuid NOT NULL,
    seq integer NOT NULL,
    trigger text NOT NULL,
    input_message text DEFAULT ''::text NOT NULL,
    started_at timestamptz DEFAULT now() NOT NULL,
    ended_at timestamptz,
    exit_code integer,
    status text DEFAULT 'done'::text NOT NULL,
    process_started_at timestamptz,
    log_key text,
    usage jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT run_turns_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'running'::text, 'done'::text]))),
    CONSTRAINT run_turns_trigger_check CHECK ((trigger = ANY (ARRAY['start'::text, 'approval'::text, 'comment'::text, 'resume'::text, 'delegation'::text])))
);

--
-- Name: runners; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.runners (
    id text NOT NULL,
    backend text NOT NULL,
    max_concurrent integer NOT NULL,
    running integer DEFAULT 0 NOT NULL,
    version text DEFAULT ''::text NOT NULL,
    started_at timestamptz DEFAULT now() NOT NULL,
    heartbeat_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT runners_backend_check CHECK ((backend = ANY (ARRAY['docker'::text, 'local'::text])))
);

--
-- Name: runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.runs (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    workspace_id uuid NOT NULL,
    ticket_id uuid,
    agent_id uuid NOT NULL,
    mode text NOT NULL,
    state text NOT NULL,
    wait_reason text,
    resolved_spec jsonb NOT NULL,
    definition_version_id uuid,
    harness text NOT NULL,
    model text NOT NULL,
    harness_session_id text,
    outcome text,
    tool_calls integer DEFAULT 0 NOT NULL,
    input_tokens bigint DEFAULT 0 NOT NULL,
    output_tokens bigint DEFAULT 0 NOT NULL,
    cost_usd_micros bigint DEFAULT 0 NOT NULL,
    cost_source text,
    started_at timestamptz DEFAULT now() NOT NULL,
    ended_at timestamptz,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    dispatched_by_user_id uuid,
    runner_id text,
    workspace_volume text,
    idle_turns integer DEFAULT 0 NOT NULL,
    parent_run_id uuid,
    purpose text NOT NULL,
    contract jsonb,
    checks jsonb,
    setup_ms bigint,
    harness_ms bigint,
    checks_ms bigint,
    CONSTRAINT runs_cost_source_check CHECK ((cost_source = ANY (ARRAY['harness'::text, 'computed'::text, 'reported'::text, 'unknown'::text, 'partial'::text]))),
    CONSTRAINT runs_mode_check CHECK ((mode = ANY (ARRAY['managed'::text, 'unmanaged'::text]))),
    CONSTRAINT runs_purpose_check CHECK ((purpose = ANY (ARRAY['ticket'::text, 'planning'::text, 'chat'::text]))),
    CONSTRAINT runs_purpose_ticket_check CHECK (((purpose = 'ticket'::text) = (ticket_id IS NOT NULL))),
    CONSTRAINT runs_state_check CHECK ((state = ANY (ARRAY['queued'::text, 'starting'::text, 'running'::text, 'waiting'::text, 'paused'::text, 'blocked'::text, 'completed'::text, 'failed'::text, 'cancelled'::text]))),
    CONSTRAINT runs_wait_reason_check CHECK ((wait_reason = ANY (ARRAY['approval'::text, 'human_reply'::text, 'pause'::text, 'delegation'::text, 'mockup_review'::text])))
);

--
-- Name: saved_views; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.saved_views (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    workspace_id uuid NOT NULL,
    user_id uuid NOT NULL,
    name text NOT NULL,
    query text DEFAULT ''::text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT saved_views_name_check CHECK (((length(btrim(name)) >= 1) AND (length(btrim(name)) <= 80))),
    CONSTRAINT saved_views_query_check CHECK ((length(query) <= 2000))
);

--
-- Name: secrets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.secrets (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    workspace_id uuid NOT NULL,
    project_id uuid,
    name text NOT NULL,
    grants_scope text,
    nonce bytea NOT NULL,
    ciphertext bytea NOT NULL,
    created_by_user_id uuid,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    last_used_at timestamptz,
    CONSTRAINT secrets_name_check CHECK ((name ~ '^[a-z][a-z0-9-]*:[a-z0-9][a-z0-9-]*$'::text))
);

--
-- Name: ticket_artifacts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ticket_artifacts (
    ticket_id uuid NOT NULL,
    artifact_id uuid NOT NULL,
    relation text DEFAULT 'context'::text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT ticket_artifacts_relation_check CHECK ((relation = ANY (ARRAY['context'::text, 'generated'::text])))
);

--
-- Name: ticket_dependencies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ticket_dependencies (
    ticket_id uuid NOT NULL,
    blocker_id uuid NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT ticket_dependencies_check CHECK ((ticket_id <> blocker_id))
);

--
-- Name: ticket_labels; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ticket_labels (
    ticket_id uuid NOT NULL,
    label_id uuid NOT NULL
);

--
-- Name: tickets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tickets (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    workspace_id uuid NOT NULL,
    project_id uuid NOT NULL,
    number integer NOT NULL,
    key text NOT NULL,
    title text NOT NULL,
    description_md text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'backlog'::text NOT NULL,
    priority text DEFAULT 'none'::text NOT NULL,
    epic_id uuid,
    assignee_agent_id uuid,
    reviewer_user_id uuid,
    created_by_user_id uuid,
    estimate text,
    due_date date,
    branch text,
    pr_number integer,
    pr_url text,
    pr_state text,
    pr_additions integer,
    pr_deletions integer,
    sort_order double precision DEFAULT 0 NOT NULL,
    search tsvector GENERATED ALWAYS AS (((setweight(to_tsvector('simple'::regconfig, key), 'A'::"char") || setweight(to_tsvector('english'::regconfig, title), 'A'::"char")) || setweight(to_tsvector('english'::regconfig, description_md), 'C'::"char"))) STORED,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    pr_synced_at timestamptz,
    pr_closed_unmerged boolean DEFAULT false NOT NULL,
    dependency_revision integer DEFAULT 1 NOT NULL,
    requires_mockup boolean DEFAULT false NOT NULL,
    revision integer DEFAULT 1 NOT NULL,
    estimate_points integer,
    estimate_rationale text,
    estimated_by_type text,
    estimated_by_id uuid,
    estimated_at timestamptz,
    epic_review boolean DEFAULT false NOT NULL,
    CONSTRAINT tickets_estimate_points_check CHECK ((estimate_points = ANY (ARRAY[1, 2, 3, 5, 8, 13]))),
    CONSTRAINT tickets_estimate_rationale_check CHECK ((length(estimate_rationale) <= 2000)),
    CONSTRAINT tickets_estimated_by_type_check CHECK ((estimated_by_type = ANY (ARRAY['user'::text, 'agent'::text]))),
    CONSTRAINT tickets_pr_state_check CHECK ((pr_state = ANY (ARRAY['draft'::text, 'open'::text, 'merged'::text, 'closed'::text]))),
    CONSTRAINT tickets_priority_check CHECK ((priority = ANY (ARRAY['none'::text, 'low'::text, 'medium'::text, 'high'::text, 'urgent'::text]))),
    CONSTRAINT tickets_status_check CHECK ((status = ANY (ARRAY['backlog'::text, 'todo'::text, 'in_progress'::text, 'in_review'::text, 'done'::text, 'canceled'::text]))),
    CONSTRAINT tickets_title_check CHECK (((length(title) >= 1) AND (length(title) <= 300)))
);

--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    workspace_id uuid NOT NULL,
    name text NOT NULL,
    email text NOT NULL,
    initials text NOT NULL,
    role_title text DEFAULT ''::text NOT NULL,
    avatar_color text,
    password_hash text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    avatar_key text,
    avatar_content_type text,
    CONSTRAINT users_avatar_pair CHECK (((avatar_key IS NULL) = (avatar_content_type IS NULL)))
);

--
-- Name: workspaces; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.workspaces (
    id uuid DEFAULT public.uuid_v7() NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL
);

--
-- Name: acceptance_criteria acceptance_criteria_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acceptance_criteria
    ADD CONSTRAINT acceptance_criteria_pkey PRIMARY KEY (id);

--
-- Name: activity_events activity_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activity_events
    ADD CONSTRAINT activity_events_pkey PRIMARY KEY (id);

--
-- Name: agent_definition_versions agent_definition_versions_agent_id_version_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_definition_versions
    ADD CONSTRAINT agent_definition_versions_agent_id_version_key UNIQUE (agent_id, version);

--
-- Name: agent_definition_versions agent_definition_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_definition_versions
    ADD CONSTRAINT agent_definition_versions_pkey PRIMARY KEY (id);

--
-- Name: agent_project_overrides agent_project_overrides_agent_id_project_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_project_overrides
    ADD CONSTRAINT agent_project_overrides_agent_id_project_id_key UNIQUE (agent_id, project_id);

--
-- Name: agent_project_overrides agent_project_overrides_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_project_overrides
    ADD CONSTRAINT agent_project_overrides_pkey PRIMARY KEY (id);

--
-- Name: agent_tokens agent_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_tokens
    ADD CONSTRAINT agent_tokens_pkey PRIMARY KEY (id);

--
-- Name: agent_tokens agent_tokens_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_tokens
    ADD CONSTRAINT agent_tokens_token_hash_key UNIQUE (token_hash);

--
-- Name: agents agents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_pkey PRIMARY KEY (id);

--
-- Name: approval_requests approval_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_requests
    ADD CONSTRAINT approval_requests_pkey PRIMARY KEY (id);

--
-- Name: artifact_aliases artifact_aliases_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifact_aliases
    ADD CONSTRAINT artifact_aliases_pkey PRIMARY KEY (workspace_id, path);

--
-- Name: artifact_reads artifact_reads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifact_reads
    ADD CONSTRAINT artifact_reads_pkey PRIMARY KEY (id);

--
-- Name: artifact_sections artifact_sections_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifact_sections
    ADD CONSTRAINT artifact_sections_pkey PRIMARY KEY (id);

--
-- Name: artifact_sections artifact_sections_version_id_anchor_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifact_sections
    ADD CONSTRAINT artifact_sections_version_id_anchor_key UNIQUE (version_id, anchor);

--
-- Name: artifact_sections artifact_sections_version_id_position_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifact_sections
    ADD CONSTRAINT artifact_sections_version_id_position_key UNIQUE (version_id, "position");

--
-- Name: artifact_versions artifact_versions_artifact_id_seq_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifact_versions
    ADD CONSTRAINT artifact_versions_artifact_id_seq_key UNIQUE (artifact_id, seq);

--
-- Name: artifact_versions artifact_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifact_versions
    ADD CONSTRAINT artifact_versions_pkey PRIMARY KEY (id);

--
-- Name: artifacts artifacts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifacts
    ADD CONSTRAINT artifacts_pkey PRIMARY KEY (id);

--
-- Name: artifacts artifacts_workspace_id_type_number_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifacts
    ADD CONSTRAINT artifacts_workspace_id_type_number_key UNIQUE (workspace_id, type, number);

--
-- Name: chat_attachments chat_attachments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_attachments
    ADD CONSTRAINT chat_attachments_pkey PRIMARY KEY (id);

--
-- Name: chat_messages chat_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_messages
    ADD CONSTRAINT chat_messages_pkey PRIMARY KEY (id);

--
-- Name: chat_messages chat_messages_session_id_seq_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_messages
    ADD CONSTRAINT chat_messages_session_id_seq_key UNIQUE (session_id, seq);

--
-- Name: chat_sessions chat_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_sessions
    ADD CONSTRAINT chat_sessions_pkey PRIMARY KEY (id);

--
-- Name: comments comments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments
    ADD CONSTRAINT comments_pkey PRIMARY KEY (id);

--
-- Name: epic_criteria epic_criteria_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.epic_criteria
    ADD CONSTRAINT epic_criteria_pkey PRIMARY KEY (id);

--
-- Name: epic_criteria epic_criteria_position_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.epic_criteria
    ADD CONSTRAINT epic_criteria_position_key UNIQUE (epic_id, "position") DEFERRABLE;

--
-- Name: epics epics_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.epics
    ADD CONSTRAINT epics_pkey PRIMARY KEY (id);

--
-- Name: inbox_reads inbox_reads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inbox_reads
    ADD CONSTRAINT inbox_reads_pkey PRIMARY KEY (user_id, item_id);

--
-- Name: knowledge_links knowledge_links_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_links
    ADD CONSTRAINT knowledge_links_pkey PRIMARY KEY (id);

--
-- Name: labels labels_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.labels
    ADD CONSTRAINT labels_pkey PRIMARY KEY (id);

--
-- Name: labels labels_workspace_id_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.labels
    ADD CONSTRAINT labels_workspace_id_name_key UNIQUE (workspace_id, name);

--
-- Name: memories memories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memories
    ADD CONSTRAINT memories_pkey PRIMARY KEY (id);

--
-- Name: metric_snapshots metric_snapshots_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.metric_snapshots
    ADD CONSTRAINT metric_snapshots_pkey PRIMARY KEY (workspace_id, range);

--
-- Name: personas personas_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.personas
    ADD CONSTRAINT personas_pkey PRIMARY KEY (id);

--
-- Name: plan_steps plan_steps_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plan_steps
    ADD CONSTRAINT plan_steps_pkey PRIMARY KEY (id);

--
-- Name: plan_steps plan_steps_run_id_position_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plan_steps
    ADD CONSTRAINT plan_steps_run_id_position_key UNIQUE (run_id, "position");

--
-- Name: planning_sessions planning_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_sessions
    ADD CONSTRAINT planning_sessions_pkey PRIMARY KEY (id);

--
-- Name: planning_sessions planning_sessions_run_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_sessions
    ADD CONSTRAINT planning_sessions_run_id_key UNIQUE (run_id);

--
-- Name: projects projects_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_pkey PRIMARY KEY (id);

--
-- Name: projects projects_workspace_id_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_workspace_id_key_key UNIQUE (workspace_id, key);

--
-- Name: provider_limits provider_limits_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.provider_limits
    ADD CONSTRAINT provider_limits_pkey PRIMARY KEY (workspace_id, harness);

--
-- Name: run_events run_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.run_events
    ADD CONSTRAINT run_events_pkey PRIMARY KEY (id);

--
-- Name: run_turns run_turns_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.run_turns
    ADD CONSTRAINT run_turns_pkey PRIMARY KEY (id);

--
-- Name: run_turns run_turns_run_id_seq_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.run_turns
    ADD CONSTRAINT run_turns_run_id_seq_key UNIQUE (run_id, seq);

--
-- Name: runners runners_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runners
    ADD CONSTRAINT runners_pkey PRIMARY KEY (id);

--
-- Name: runs runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runs
    ADD CONSTRAINT runs_pkey PRIMARY KEY (id);

--
-- Name: saved_views saved_views_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.saved_views
    ADD CONSTRAINT saved_views_pkey PRIMARY KEY (id);

--
-- Name: secrets secrets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.secrets
    ADD CONSTRAINT secrets_pkey PRIMARY KEY (id);

--
-- Name: ticket_artifacts ticket_artifacts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_artifacts
    ADD CONSTRAINT ticket_artifacts_pkey PRIMARY KEY (ticket_id, artifact_id);

--
-- Name: ticket_dependencies ticket_dependencies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_dependencies
    ADD CONSTRAINT ticket_dependencies_pkey PRIMARY KEY (ticket_id, blocker_id);

--
-- Name: ticket_labels ticket_labels_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_labels
    ADD CONSTRAINT ticket_labels_pkey PRIMARY KEY (ticket_id, label_id);

--
-- Name: tickets tickets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_pkey PRIMARY KEY (id);

--
-- Name: tickets tickets_project_id_number_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_project_id_number_key UNIQUE (project_id, number);

--
-- Name: tickets tickets_workspace_id_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_workspace_id_key_key UNIQUE (workspace_id, key);

--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

--
-- Name: workspaces workspaces_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workspaces
    ADD CONSTRAINT workspaces_pkey PRIMARY KEY (id);

--
-- Name: workspaces workspaces_slug_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workspaces
    ADD CONSTRAINT workspaces_slug_key UNIQUE (slug);

--
-- Name: acceptance_criteria_ticket_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX acceptance_criteria_ticket_idx ON public.acceptance_criteria USING btree (ticket_id, "position");

--
-- Name: activity_completion_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX activity_completion_idx ON public.activity_events USING btree (workspace_id, created_at, ticket_id) WHERE ((verb = 'status_changed'::text) AND ((payload ->> 'to'::text) = 'done'::text));

--
-- Name: activity_ticket_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX activity_ticket_idx ON public.activity_events USING btree (ticket_id, created_at, seq);

--
-- Name: activity_workspace_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX activity_workspace_idx ON public.activity_events USING btree (workspace_id, created_at DESC);

--
-- Name: agent_tokens_agent_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agent_tokens_agent_idx ON public.agent_tokens USING btree (agent_id);

--
-- Name: agents_live_handle; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX agents_live_handle ON public.agents USING btree (workspace_id, handle) WHERE (archived_at IS NULL);

--
-- Name: agents_one_assistant; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX agents_one_assistant ON public.agents USING btree (workspace_id) WHERE (kind = 'assistant'::text);

--
-- Name: approval_requests_pending_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX approval_requests_pending_idx ON public.approval_requests USING btree (workspace_id, created_at) WHERE (state = 'pending'::text);

--
-- Name: approval_requests_ticket_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX approval_requests_ticket_idx ON public.approval_requests USING btree (ticket_id, created_at);

--
-- Name: artifact_aliases_lower_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX artifact_aliases_lower_idx ON public.artifact_aliases USING btree (workspace_id, lower(path));

--
-- Name: artifact_reads_section_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX artifact_reads_section_idx ON public.artifact_reads USING btree (section_id) WHERE (section_id IS NOT NULL);

--
-- Name: artifact_reads_version_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX artifact_reads_version_idx ON public.artifact_reads USING btree (version_id);

--
-- Name: artifact_versions_suggested_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX artifact_versions_suggested_idx ON public.artifact_versions USING btree (artifact_id) WHERE (state = 'suggested'::text);

--
-- Name: artifacts_note_path_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX artifacts_note_path_idx ON public.artifacts USING btree (workspace_id, lower(path)) WHERE (path IS NOT NULL);

--
-- Name: artifacts_note_path_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX artifacts_note_path_trgm ON public.artifacts USING gin (lower(path) public.gin_trgm_ops) WHERE (type = 'note'::text);

--
-- Name: artifacts_note_title_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX artifacts_note_title_idx ON public.artifacts USING btree (workspace_id, lower(title)) WHERE (type = 'note'::text);

--
-- Name: artifacts_note_title_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX artifacts_note_title_trgm ON public.artifacts USING gin (lower(title) public.gin_trgm_ops) WHERE (type = 'note'::text);

--
-- Name: artifacts_project_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX artifacts_project_idx ON public.artifacts USING btree (project_id);

--
-- Name: artifacts_search_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX artifacts_search_idx ON public.artifacts USING gin (search);

--
-- Name: chat_attachments_message; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX chat_attachments_message ON public.chat_attachments USING btree (message_id) WHERE (message_id IS NOT NULL);

--
-- Name: chat_attachments_owner; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX chat_attachments_owner ON public.chat_attachments USING btree (workspace_id, user_id);

--
-- Name: chat_attachments_unattached; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX chat_attachments_unattached ON public.chat_attachments USING btree (created_at) WHERE (message_id IS NULL);

--
-- Name: chat_sessions_list; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX chat_sessions_list ON public.chat_sessions USING btree (project_id, created_by, COALESCE(last_message_at, created_at) DESC, id DESC) WHERE (NOT archived);

--
-- Name: chat_sessions_persona; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX chat_sessions_persona ON public.chat_sessions USING btree (created_by, persona_id, COALESCE(last_message_at, created_at) DESC, id DESC) WHERE (NOT archived);

--
-- Name: comments_ticket_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX comments_ticket_idx ON public.comments USING btree (ticket_id, created_at);

--
-- Name: epics_project_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX epics_project_idx ON public.epics USING btree (project_id);

--
-- Name: knowledge_links_source_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX knowledge_links_source_idx ON public.knowledge_links USING btree (source_kind, source_id);

--
-- Name: knowledge_links_target_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX knowledge_links_target_idx ON public.knowledge_links USING btree (target_kind, target_id);

--
-- Name: knowledge_links_unresolved_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX knowledge_links_unresolved_idx ON public.knowledge_links USING btree (workspace_id, lower(target_text)) WHERE (target_kind = 'unresolved'::text);

--
-- Name: memories_owner; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX memories_owner ON public.memories USING btree (workspace_id, user_id, updated_at DESC, id DESC);

--
-- Name: memories_search; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX memories_search ON public.memories USING gin (public.memory_search_vector(title, summary, body, tags));

--
-- Name: personas_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX personas_name ON public.personas USING btree (workspace_id, user_id, lower(btrim(name)));

--
-- Name: personas_one_default; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX personas_one_default ON public.personas USING btree (workspace_id, user_id) WHERE is_default;

--
-- Name: planning_sessions_workspace; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX planning_sessions_workspace ON public.planning_sessions USING btree (workspace_id, created_at DESC);

--
-- Name: run_events_run_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX run_events_run_idx ON public.run_events USING btree (run_id, seq);

--
-- Name: run_turns_pending_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX run_turns_pending_idx ON public.run_turns USING btree (run_id) WHERE (status <> 'done'::text);

--
-- Name: runs_agent_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX runs_agent_idx ON public.runs USING btree (agent_id, state);

--
-- Name: runs_one_active_per_ticket; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX runs_one_active_per_ticket ON public.runs USING btree (ticket_id) WHERE ((parent_run_id IS NULL) AND (state <> ALL (ARRAY['completed'::text, 'failed'::text, 'cancelled'::text])));

--
-- Name: runs_parent_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX runs_parent_idx ON public.runs USING btree (parent_run_id) WHERE (parent_run_id IS NOT NULL);

--
-- Name: runs_queued_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX runs_queued_idx ON public.runs USING btree (started_at) WHERE (state = 'queued'::text);

--
-- Name: saved_views_user_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX saved_views_user_name ON public.saved_views USING btree (user_id, lower(name));

--
-- Name: secrets_name_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX secrets_name_key ON public.secrets USING btree (workspace_id, COALESCE(project_id, '00000000-0000-0000-0000-000000000000'::uuid), name);

--
-- Name: ticket_artifacts_artifact_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ticket_artifacts_artifact_idx ON public.ticket_artifacts USING btree (artifact_id);

--
-- Name: ticket_dependencies_blocker; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ticket_dependencies_blocker ON public.ticket_dependencies USING btree (blocker_id);

--
-- Name: tickets_assignee_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX tickets_assignee_idx ON public.tickets USING btree (assignee_agent_id) WHERE (assignee_agent_id IS NOT NULL);

--
-- Name: tickets_epic_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX tickets_epic_idx ON public.tickets USING btree (epic_id) WHERE (epic_id IS NOT NULL);

--
-- Name: tickets_epic_review_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX tickets_epic_review_idx ON public.tickets USING btree (epic_id) WHERE epic_review;

--
-- Name: tickets_project_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX tickets_project_status_idx ON public.tickets USING btree (project_id, status, sort_order);

--
-- Name: tickets_search_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX tickets_search_idx ON public.tickets USING gin (search);

--
-- Name: users_email_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX users_email_key ON public.users USING btree (lower(email));

--
-- Name: agent_project_overrides agent_project_overrides_touch; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER agent_project_overrides_touch BEFORE UPDATE ON public.agent_project_overrides FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

--
-- Name: agents agents_touch; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER agents_touch BEFORE UPDATE ON public.agents FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

--
-- Name: artifacts artifacts_touch; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER artifacts_touch BEFORE UPDATE ON public.artifacts FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

--
-- Name: comments comments_touch; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER comments_touch BEFORE UPDATE ON public.comments FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

--
-- Name: acceptance_criteria criteria_touch; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER criteria_touch BEFORE UPDATE ON public.acceptance_criteria FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

--
-- Name: epic_criteria epic_criteria_touch; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER epic_criteria_touch BEFORE UPDATE ON public.epic_criteria FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

--
-- Name: epics epics_touch; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER epics_touch BEFORE UPDATE ON public.epics FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

--
-- Name: labels labels_touch; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER labels_touch BEFORE UPDATE ON public.labels FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

--
-- Name: projects projects_touch; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER projects_touch BEFORE UPDATE ON public.projects FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

--
-- Name: activity_events realtime_change; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change AFTER INSERT OR DELETE OR UPDATE ON public.activity_events FOR EACH STATEMENT EXECUTE FUNCTION public.notify_change('activity.created');

--
-- Name: agents realtime_change; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change AFTER INSERT OR DELETE OR UPDATE ON public.agents FOR EACH STATEMENT EXECUTE FUNCTION public.notify_change('agent.updated');

--
-- Name: approval_requests realtime_change; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change AFTER INSERT OR DELETE OR UPDATE ON public.approval_requests FOR EACH STATEMENT EXECUTE FUNCTION public.notify_change('approval.updated');

--
-- Name: artifact_reads realtime_change; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change AFTER INSERT OR DELETE OR UPDATE ON public.artifact_reads FOR EACH STATEMENT EXECUTE FUNCTION public.notify_change('artifact.updated');

--
-- Name: artifact_versions realtime_change; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change AFTER INSERT OR DELETE OR UPDATE ON public.artifact_versions FOR EACH STATEMENT EXECUTE FUNCTION public.notify_change('artifact.updated');

--
-- Name: artifacts realtime_change; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change AFTER INSERT OR DELETE OR UPDATE ON public.artifacts FOR EACH STATEMENT EXECUTE FUNCTION public.notify_change('artifact.updated');

--
-- Name: inbox_reads realtime_change; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change AFTER INSERT OR DELETE OR UPDATE ON public.inbox_reads FOR EACH STATEMENT EXECUTE FUNCTION public.notify_change('inbox.updated');

--
-- Name: knowledge_links realtime_change; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change AFTER INSERT OR DELETE OR UPDATE ON public.knowledge_links FOR EACH STATEMENT EXECUTE FUNCTION public.notify_change('knowledge.updated');

--
-- Name: metric_snapshots realtime_change; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change AFTER INSERT OR DELETE OR UPDATE ON public.metric_snapshots FOR EACH STATEMENT EXECUTE FUNCTION public.notify_change('metrics.updated');

--
-- Name: planning_sessions realtime_change; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change AFTER INSERT OR DELETE OR UPDATE ON public.planning_sessions FOR EACH STATEMENT EXECUTE FUNCTION public.notify_change('planning.updated');

--
-- Name: projects realtime_change; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change AFTER INSERT OR DELETE OR UPDATE ON public.projects FOR EACH STATEMENT EXECUTE FUNCTION public.notify_change('project.updated');

--
-- Name: run_events realtime_change; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change AFTER INSERT OR DELETE OR UPDATE ON public.run_events FOR EACH STATEMENT EXECUTE FUNCTION public.notify_change('run.events');

--
-- Name: acceptance_criteria realtime_change_del; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_del AFTER DELETE ON public.acceptance_criteria REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('ticket.updated', 'ticket_id');

--
-- Name: chat_messages realtime_change_del; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_del AFTER DELETE ON public.chat_messages REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_chat_change('chat.updated', 'session_id');

--
-- Name: chat_sessions realtime_change_del; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_del AFTER DELETE ON public.chat_sessions REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_chat_change('chat.updated', 'id');

--
-- Name: comments realtime_change_del; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_del AFTER DELETE ON public.comments REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('ticket.updated', 'ticket_id');

--
-- Name: epic_criteria realtime_change_del; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_del AFTER DELETE ON public.epic_criteria REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('epic.updated', 'epic_id');

--
-- Name: epics realtime_change_del; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_del AFTER DELETE ON public.epics REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('epic.updated', 'id');

--
-- Name: personas realtime_change_del; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_del AFTER DELETE ON public.personas REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_chat_change('persona.updated', 'id');

--
-- Name: plan_steps realtime_change_del; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_del AFTER DELETE ON public.plan_steps REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('run.updated', 'run_id');

--
-- Name: run_turns realtime_change_del; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_del AFTER DELETE ON public.run_turns REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('run.updated', 'run_id');

--
-- Name: runs realtime_change_del; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_del AFTER DELETE ON public.runs REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('run.updated', 'id');

--
-- Name: ticket_artifacts realtime_change_del; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_del AFTER DELETE ON public.ticket_artifacts REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('ticket.updated', 'ticket_id');

--
-- Name: ticket_dependencies realtime_change_del; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_del AFTER DELETE ON public.ticket_dependencies REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('ticket.updated', 'ticket_id', 'blocker_id');

--
-- Name: tickets realtime_change_del; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_del AFTER DELETE ON public.tickets REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('ticket.updated', 'id');

--
-- Name: acceptance_criteria realtime_change_ins; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_ins AFTER INSERT ON public.acceptance_criteria REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('ticket.updated', 'ticket_id');

--
-- Name: chat_messages realtime_change_ins; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_ins AFTER INSERT ON public.chat_messages REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_chat_change('chat.updated', 'session_id');

--
-- Name: chat_sessions realtime_change_ins; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_ins AFTER INSERT ON public.chat_sessions REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_chat_change('chat.updated', 'id');

--
-- Name: comments realtime_change_ins; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_ins AFTER INSERT ON public.comments REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('ticket.updated', 'ticket_id');

--
-- Name: epic_criteria realtime_change_ins; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_ins AFTER INSERT ON public.epic_criteria REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('epic.updated', 'epic_id');

--
-- Name: epics realtime_change_ins; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_ins AFTER INSERT ON public.epics REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('epic.updated', 'id');

--
-- Name: personas realtime_change_ins; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_ins AFTER INSERT ON public.personas REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_chat_change('persona.updated', 'id');

--
-- Name: plan_steps realtime_change_ins; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_ins AFTER INSERT ON public.plan_steps REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('run.updated', 'run_id');

--
-- Name: run_turns realtime_change_ins; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_ins AFTER INSERT ON public.run_turns REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('run.updated', 'run_id');

--
-- Name: runs realtime_change_ins; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_ins AFTER INSERT ON public.runs REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('run.updated', 'id');

--
-- Name: ticket_artifacts realtime_change_ins; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_ins AFTER INSERT ON public.ticket_artifacts REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('ticket.updated', 'ticket_id');

--
-- Name: ticket_dependencies realtime_change_ins; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_ins AFTER INSERT ON public.ticket_dependencies REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('ticket.updated', 'ticket_id', 'blocker_id');

--
-- Name: tickets realtime_change_ins; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_ins AFTER INSERT ON public.tickets REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('ticket.updated', 'id');

--
-- Name: acceptance_criteria realtime_change_upd; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_upd AFTER UPDATE ON public.acceptance_criteria REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('ticket.updated', 'ticket_id');

--
-- Name: chat_messages realtime_change_upd; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_upd AFTER UPDATE ON public.chat_messages REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_chat_change('chat.updated', 'session_id');

--
-- Name: chat_sessions realtime_change_upd; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_upd AFTER UPDATE ON public.chat_sessions REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_chat_change('chat.updated', 'id');

--
-- Name: comments realtime_change_upd; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_upd AFTER UPDATE ON public.comments REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('ticket.updated', 'ticket_id');

--
-- Name: epic_criteria realtime_change_upd; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_upd AFTER UPDATE ON public.epic_criteria REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('epic.updated', 'epic_id');

--
-- Name: epics realtime_change_upd; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_upd AFTER UPDATE ON public.epics REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('epic.updated', 'id');

--
-- Name: personas realtime_change_upd; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_upd AFTER UPDATE ON public.personas REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_chat_change('persona.updated', 'id');

--
-- Name: plan_steps realtime_change_upd; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_upd AFTER UPDATE ON public.plan_steps REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('run.updated', 'run_id');

--
-- Name: run_turns realtime_change_upd; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_upd AFTER UPDATE ON public.run_turns REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('run.updated', 'run_id');

--
-- Name: runs realtime_change_upd; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_upd AFTER UPDATE ON public.runs REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('run.updated', 'id');

--
-- Name: ticket_artifacts realtime_change_upd; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_upd AFTER UPDATE ON public.ticket_artifacts REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('ticket.updated', 'ticket_id');

--
-- Name: ticket_dependencies realtime_change_upd; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_upd AFTER UPDATE ON public.ticket_dependencies REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('ticket.updated', 'ticket_id', 'blocker_id');

--
-- Name: tickets realtime_change_upd; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_change_upd AFTER UPDATE ON public.tickets REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_entity_change('ticket.updated', 'id');

--
-- Name: tickets realtime_epic_del; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_epic_del AFTER DELETE ON public.tickets REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_ticket_epic_change();

--
-- Name: tickets realtime_epic_ins; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_epic_ins AFTER INSERT ON public.tickets REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_ticket_epic_change();

--
-- Name: tickets realtime_epic_upd; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER realtime_epic_upd AFTER UPDATE ON public.tickets REFERENCING OLD TABLE AS old_rows NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION public.notify_ticket_epic_change();

--
-- Name: runs runs_touch; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER runs_touch BEFORE UPDATE ON public.runs FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

--
-- Name: secrets secrets_touch; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER secrets_touch BEFORE UPDATE ON public.secrets FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

--
-- Name: tickets tickets_touch; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER tickets_touch BEFORE UPDATE ON public.tickets FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

--
-- Name: users users_touch; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER users_touch BEFORE UPDATE ON public.users FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

--
-- Name: workspaces workspaces_touch; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER workspaces_touch BEFORE UPDATE ON public.workspaces FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

--
-- Name: acceptance_criteria acceptance_criteria_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acceptance_criteria
    ADD CONSTRAINT acceptance_criteria_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id) ON DELETE CASCADE;

--
-- Name: activity_events activity_events_epic_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activity_events
    ADD CONSTRAINT activity_events_epic_id_fkey FOREIGN KEY (epic_id) REFERENCES public.epics(id) ON DELETE SET NULL;

--
-- Name: activity_events activity_events_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activity_events
    ADD CONSTRAINT activity_events_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id) ON DELETE CASCADE;

--
-- Name: activity_events activity_events_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activity_events
    ADD CONSTRAINT activity_events_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

--
-- Name: agent_definition_versions agent_definition_versions_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_definition_versions
    ADD CONSTRAINT agent_definition_versions_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: agent_definition_versions agent_definition_versions_created_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_definition_versions
    ADD CONSTRAINT agent_definition_versions_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id);

--
-- Name: agent_project_overrides agent_project_overrides_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_project_overrides
    ADD CONSTRAINT agent_project_overrides_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: agent_project_overrides agent_project_overrides_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_project_overrides
    ADD CONSTRAINT agent_project_overrides_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;

--
-- Name: agent_project_overrides agent_project_overrides_updated_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_project_overrides
    ADD CONSTRAINT agent_project_overrides_updated_by_user_id_fkey FOREIGN KEY (updated_by_user_id) REFERENCES public.users(id);

--
-- Name: agent_tokens agent_tokens_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_tokens
    ADD CONSTRAINT agent_tokens_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: agent_tokens agent_tokens_created_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_tokens
    ADD CONSTRAINT agent_tokens_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id);

--
-- Name: agent_tokens agent_tokens_run_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_tokens
    ADD CONSTRAINT agent_tokens_run_fk FOREIGN KEY (run_id) REFERENCES public.runs(id) ON DELETE CASCADE;

--
-- Name: agents agents_current_definition_version_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_current_definition_version_id_fkey FOREIGN KEY (current_definition_version_id) REFERENCES public.agent_definition_versions(id);

--
-- Name: agents agents_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

--
-- Name: approval_requests approval_requests_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_requests
    ADD CONSTRAINT approval_requests_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id);

--
-- Name: approval_requests approval_requests_decided_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_requests
    ADD CONSTRAINT approval_requests_decided_by_user_id_fkey FOREIGN KEY (decided_by_user_id) REFERENCES public.users(id);

--
-- Name: approval_requests approval_requests_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_requests
    ADD CONSTRAINT approval_requests_run_id_fkey FOREIGN KEY (run_id) REFERENCES public.runs(id) ON DELETE CASCADE;

--
-- Name: approval_requests approval_requests_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_requests
    ADD CONSTRAINT approval_requests_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id) ON DELETE CASCADE;

--
-- Name: approval_requests approval_requests_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_requests
    ADD CONSTRAINT approval_requests_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

--
-- Name: artifact_aliases artifact_aliases_artifact_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifact_aliases
    ADD CONSTRAINT artifact_aliases_artifact_id_fkey FOREIGN KEY (artifact_id) REFERENCES public.artifacts(id) ON DELETE CASCADE;

--
-- Name: artifact_aliases artifact_aliases_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifact_aliases
    ADD CONSTRAINT artifact_aliases_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

--
-- Name: artifact_reads artifact_reads_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifact_reads
    ADD CONSTRAINT artifact_reads_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: artifact_reads artifact_reads_run_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifact_reads
    ADD CONSTRAINT artifact_reads_run_fk FOREIGN KEY (run_id) REFERENCES public.runs(id) ON DELETE SET NULL;

--
-- Name: artifact_reads artifact_reads_section_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifact_reads
    ADD CONSTRAINT artifact_reads_section_id_fkey FOREIGN KEY (section_id) REFERENCES public.artifact_sections(id) ON DELETE SET NULL;

--
-- Name: artifact_reads artifact_reads_version_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifact_reads
    ADD CONSTRAINT artifact_reads_version_id_fkey FOREIGN KEY (version_id) REFERENCES public.artifact_versions(id) ON DELETE CASCADE;

--
-- Name: artifact_sections artifact_sections_version_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifact_sections
    ADD CONSTRAINT artifact_sections_version_id_fkey FOREIGN KEY (version_id) REFERENCES public.artifact_versions(id) ON DELETE CASCADE;

--
-- Name: artifact_versions artifact_versions_artifact_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifact_versions
    ADD CONSTRAINT artifact_versions_artifact_id_fkey FOREIGN KEY (artifact_id) REFERENCES public.artifacts(id) ON DELETE CASCADE;

--
-- Name: artifact_versions artifact_versions_based_on_version_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifact_versions
    ADD CONSTRAINT artifact_versions_based_on_version_id_fkey FOREIGN KEY (based_on_version_id) REFERENCES public.artifact_versions(id) ON DELETE SET NULL;

--
-- Name: artifact_versions artifact_versions_decided_by_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifact_versions
    ADD CONSTRAINT artifact_versions_decided_by_id_fkey FOREIGN KEY (decided_by_id) REFERENCES public.users(id);

--
-- Name: artifact_versions artifact_versions_reviewed_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifact_versions
    ADD CONSTRAINT artifact_versions_reviewed_by_user_id_fkey FOREIGN KEY (reviewed_by_user_id) REFERENCES public.users(id);

--
-- Name: artifacts artifacts_current_version_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifacts
    ADD CONSTRAINT artifacts_current_version_fk FOREIGN KEY (current_version_id) REFERENCES public.artifact_versions(id) ON DELETE SET NULL;

--
-- Name: artifacts artifacts_epic_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifacts
    ADD CONSTRAINT artifacts_epic_id_fkey FOREIGN KEY (epic_id) REFERENCES public.epics(id) ON DELETE SET NULL;

--
-- Name: artifacts artifacts_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifacts
    ADD CONSTRAINT artifacts_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id);

--
-- Name: artifacts artifacts_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifacts
    ADD CONSTRAINT artifacts_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

--
-- Name: chat_attachments chat_attachments_message_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_attachments
    ADD CONSTRAINT chat_attachments_message_id_fkey FOREIGN KEY (message_id) REFERENCES public.chat_messages(id) ON DELETE SET NULL;

--
-- Name: chat_attachments chat_attachments_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_attachments
    ADD CONSTRAINT chat_attachments_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

--
-- Name: chat_attachments chat_attachments_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_attachments
    ADD CONSTRAINT chat_attachments_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

--
-- Name: chat_messages chat_messages_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_messages
    ADD CONSTRAINT chat_messages_run_id_fkey FOREIGN KEY (run_id) REFERENCES public.runs(id) ON DELETE SET NULL;

--
-- Name: chat_messages chat_messages_run_turn_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_messages
    ADD CONSTRAINT chat_messages_run_turn_id_fkey FOREIGN KEY (run_turn_id) REFERENCES public.run_turns(id) ON DELETE SET NULL;

--
-- Name: chat_messages chat_messages_session_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_messages
    ADD CONSTRAINT chat_messages_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.chat_sessions(id) ON DELETE CASCADE;

--
-- Name: chat_sessions chat_sessions_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_sessions
    ADD CONSTRAINT chat_sessions_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id);

--
-- Name: chat_sessions chat_sessions_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_sessions
    ADD CONSTRAINT chat_sessions_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);

--
-- Name: chat_sessions chat_sessions_current_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_sessions
    ADD CONSTRAINT chat_sessions_current_run_id_fkey FOREIGN KEY (current_run_id) REFERENCES public.runs(id) ON DELETE SET NULL;

--
-- Name: chat_sessions chat_sessions_persona_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_sessions
    ADD CONSTRAINT chat_sessions_persona_id_fkey FOREIGN KEY (persona_id) REFERENCES public.personas(id) ON DELETE SET NULL;

--
-- Name: chat_sessions chat_sessions_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_sessions
    ADD CONSTRAINT chat_sessions_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id);

--
-- Name: chat_sessions chat_sessions_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_sessions
    ADD CONSTRAINT chat_sessions_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

--
-- Name: comments comments_parent_comment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments
    ADD CONSTRAINT comments_parent_comment_id_fkey FOREIGN KEY (parent_comment_id) REFERENCES public.comments(id) ON DELETE CASCADE;

--
-- Name: comments comments_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments
    ADD CONSTRAINT comments_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id) ON DELETE CASCADE;

--
-- Name: comments comments_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments
    ADD CONSTRAINT comments_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

--
-- Name: epic_criteria epic_criteria_epic_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.epic_criteria
    ADD CONSTRAINT epic_criteria_epic_id_fkey FOREIGN KEY (epic_id) REFERENCES public.epics(id) ON DELETE CASCADE;

--
-- Name: epics epics_owner_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.epics
    ADD CONSTRAINT epics_owner_user_id_fkey FOREIGN KEY (owner_user_id) REFERENCES public.users(id);

--
-- Name: epics epics_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.epics
    ADD CONSTRAINT epics_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id);

--
-- Name: epics epics_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.epics
    ADD CONSTRAINT epics_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

--
-- Name: inbox_reads inbox_reads_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inbox_reads
    ADD CONSTRAINT inbox_reads_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

--
-- Name: knowledge_links knowledge_links_source_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_links
    ADD CONSTRAINT knowledge_links_source_ticket_id_fkey FOREIGN KEY (source_ticket_id) REFERENCES public.tickets(id) ON DELETE CASCADE;

--
-- Name: knowledge_links knowledge_links_source_version_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_links
    ADD CONSTRAINT knowledge_links_source_version_id_fkey FOREIGN KEY (source_version_id) REFERENCES public.artifact_versions(id) ON DELETE CASCADE;

--
-- Name: knowledge_links knowledge_links_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_links
    ADD CONSTRAINT knowledge_links_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

--
-- Name: labels labels_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.labels
    ADD CONSTRAINT labels_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

--
-- Name: memories memories_source_chat_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memories
    ADD CONSTRAINT memories_source_chat_id_fkey FOREIGN KEY (source_chat_id) REFERENCES public.chat_sessions(id) ON DELETE SET NULL;

--
-- Name: memories memories_source_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memories
    ADD CONSTRAINT memories_source_run_id_fkey FOREIGN KEY (source_run_id) REFERENCES public.runs(id) ON DELETE SET NULL;

--
-- Name: memories memories_superseded_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memories
    ADD CONSTRAINT memories_superseded_by_fkey FOREIGN KEY (superseded_by) REFERENCES public.memories(id) ON DELETE CASCADE;

--
-- Name: memories memories_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memories
    ADD CONSTRAINT memories_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

--
-- Name: memories memories_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memories
    ADD CONSTRAINT memories_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

--
-- Name: metric_snapshots metric_snapshots_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.metric_snapshots
    ADD CONSTRAINT metric_snapshots_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id) ON DELETE CASCADE;

--
-- Name: personas personas_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.personas
    ADD CONSTRAINT personas_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

--
-- Name: personas personas_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.personas
    ADD CONSTRAINT personas_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

--
-- Name: plan_steps plan_steps_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plan_steps
    ADD CONSTRAINT plan_steps_run_id_fkey FOREIGN KEY (run_id) REFERENCES public.runs(id) ON DELETE CASCADE;

--
-- Name: planning_sessions planning_sessions_artifact_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_sessions
    ADD CONSTRAINT planning_sessions_artifact_id_fkey FOREIGN KEY (artifact_id) REFERENCES public.artifacts(id);

--
-- Name: planning_sessions planning_sessions_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_sessions
    ADD CONSTRAINT planning_sessions_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);

--
-- Name: planning_sessions planning_sessions_epic_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_sessions
    ADD CONSTRAINT planning_sessions_epic_id_fkey FOREIGN KEY (epic_id) REFERENCES public.epics(id);

--
-- Name: planning_sessions planning_sessions_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_sessions
    ADD CONSTRAINT planning_sessions_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id);

--
-- Name: planning_sessions planning_sessions_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_sessions
    ADD CONSTRAINT planning_sessions_run_id_fkey FOREIGN KEY (run_id) REFERENCES public.runs(id);

--
-- Name: planning_sessions planning_sessions_source_version_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_sessions
    ADD CONSTRAINT planning_sessions_source_version_id_fkey FOREIGN KEY (source_version_id) REFERENCES public.artifact_versions(id);

--
-- Name: planning_sessions planning_sessions_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.planning_sessions
    ADD CONSTRAINT planning_sessions_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

--
-- Name: projects projects_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

--
-- Name: provider_limits provider_limits_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.provider_limits
    ADD CONSTRAINT provider_limits_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id) ON DELETE CASCADE;

--
-- Name: run_events run_events_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.run_events
    ADD CONSTRAINT run_events_run_id_fkey FOREIGN KEY (run_id) REFERENCES public.runs(id) ON DELETE CASCADE;

--
-- Name: run_turns run_turns_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.run_turns
    ADD CONSTRAINT run_turns_run_id_fkey FOREIGN KEY (run_id) REFERENCES public.runs(id) ON DELETE CASCADE;

--
-- Name: runs runs_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runs
    ADD CONSTRAINT runs_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id);

--
-- Name: runs runs_definition_version_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runs
    ADD CONSTRAINT runs_definition_version_id_fkey FOREIGN KEY (definition_version_id) REFERENCES public.agent_definition_versions(id);

--
-- Name: runs runs_dispatched_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runs
    ADD CONSTRAINT runs_dispatched_by_user_id_fkey FOREIGN KEY (dispatched_by_user_id) REFERENCES public.users(id);

--
-- Name: runs runs_parent_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runs
    ADD CONSTRAINT runs_parent_run_id_fkey FOREIGN KEY (parent_run_id) REFERENCES public.runs(id) ON DELETE CASCADE;

--
-- Name: runs runs_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runs
    ADD CONSTRAINT runs_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id) ON DELETE CASCADE;

--
-- Name: runs runs_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runs
    ADD CONSTRAINT runs_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

--
-- Name: saved_views saved_views_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.saved_views
    ADD CONSTRAINT saved_views_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

--
-- Name: saved_views saved_views_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.saved_views
    ADD CONSTRAINT saved_views_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

--
-- Name: secrets secrets_created_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.secrets
    ADD CONSTRAINT secrets_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id);

--
-- Name: secrets secrets_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.secrets
    ADD CONSTRAINT secrets_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;

--
-- Name: secrets secrets_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.secrets
    ADD CONSTRAINT secrets_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

--
-- Name: ticket_artifacts ticket_artifacts_artifact_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_artifacts
    ADD CONSTRAINT ticket_artifacts_artifact_id_fkey FOREIGN KEY (artifact_id) REFERENCES public.artifacts(id) ON DELETE CASCADE;

--
-- Name: ticket_artifacts ticket_artifacts_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_artifacts
    ADD CONSTRAINT ticket_artifacts_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id) ON DELETE CASCADE;

--
-- Name: ticket_dependencies ticket_dependencies_blocker_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_dependencies
    ADD CONSTRAINT ticket_dependencies_blocker_id_fkey FOREIGN KEY (blocker_id) REFERENCES public.tickets(id) ON DELETE CASCADE;

--
-- Name: ticket_dependencies ticket_dependencies_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_dependencies
    ADD CONSTRAINT ticket_dependencies_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id) ON DELETE CASCADE;

--
-- Name: ticket_labels ticket_labels_label_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_labels
    ADD CONSTRAINT ticket_labels_label_id_fkey FOREIGN KEY (label_id) REFERENCES public.labels(id) ON DELETE CASCADE;

--
-- Name: ticket_labels ticket_labels_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_labels
    ADD CONSTRAINT ticket_labels_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id) ON DELETE CASCADE;

--
-- Name: tickets tickets_assignee_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_assignee_agent_id_fkey FOREIGN KEY (assignee_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

--
-- Name: tickets tickets_created_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id);

--
-- Name: tickets tickets_epic_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_epic_id_fkey FOREIGN KEY (epic_id) REFERENCES public.epics(id) ON DELETE SET NULL;

--
-- Name: tickets tickets_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id);

--
-- Name: tickets tickets_reviewer_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_reviewer_user_id_fkey FOREIGN KEY (reviewer_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

--
-- Name: tickets tickets_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

--
-- Name: users users_workspace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id);

-- +goose StatementEnd
