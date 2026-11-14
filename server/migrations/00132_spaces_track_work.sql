-- +goose Up
-- Spaces and projects become one thing (LAZYAGENTS-213): a space. Every space has chats,
-- instructions, sources and memory; a space that tracks work also has a project: a key,
-- tickets, epics, runs and repositories. The project stays its own table (tickets, runs and
-- everything keyed on it are untouched) and belongs to exactly one space.
--
-- RESTRICT: a space that tracks work can't be deleted under its project; it stops tracking
-- work first, which unlinks the project.
ALTER TABLE projects ADD COLUMN space_id uuid UNIQUE REFERENCES chat_spaces(id) ON DELETE RESTRICT;

-- +goose StatementBegin
-- The space color nearest a project's hex color, for a project's new space. The hexes are
-- the space tiles' own (web/lib/spaces.ts). Nearest by direction (cosine), not distance: the
-- tiles are all one lightness, so a dark navy should find blue, not the tile it's closest to
-- in raw RGB. $1, not the parameter's name, which the tile table's hex column would shadow.
CREATE FUNCTION nearest_space_color(hex text) RETURNS text LANGUAGE sql IMMUTABLE AS $$
  SELECT c.name
    FROM (VALUES ('green', '#34D399'), ('amber', '#FBBF24'), ('red', '#F87171'), ('blue', '#60A5FA'),
                 ('violet', '#A78BFA'), ('pink', '#F472B6'), ('teal', '#2DD4BF'), ('orange', '#FB923C')) AS c(name, hex),
         LATERAL (SELECT ('x' || substr(c.hex, 2, 2))::bit(8)::int AS r, ('x' || substr(c.hex, 4, 2))::bit(8)::int AS g,
                         ('x' || substr(c.hex, 6, 2))::bit(8)::int AS b) cc,
         LATERAL (SELECT ('x' || substr($1, 2, 2))::bit(8)::int AS r, ('x' || substr($1, 4, 2))::bit(8)::int AS g,
                         ('x' || substr($1, 6, 2))::bit(8)::int AS b) pc
   ORDER BY (cc.r * pc.r + cc.g * pc.g + cc.b * pc.b)::float8
            / greatest(sqrt((cc.r * cc.r + cc.g * cc.g + cc.b * cc.b)::float8) * sqrt((pc.r * pc.r + pc.g * pc.g + pc.b * pc.b)::float8), 1) DESC,
            c.name
   LIMIT 1
$$;
-- +goose StatementEnd

-- Each project gets a space, owned by the workspace's first person (Vesper has one person per
-- install; projects were workspace-wide and spaces are per person). Space memory is off and the
-- instructions empty, so the chats moved in below get exactly the context they had before.
WITH owners AS (
  SELECT DISTINCT ON (workspace_id) workspace_id, id AS user_id FROM users ORDER BY workspace_id, created_at, id
), made AS (
  INSERT INTO chat_spaces (workspace_id, user_id, name, color, project_id, space_memory)
  SELECT p.workspace_id, o.user_id, left(btrim(p.name), 80),
         CASE WHEN p.color ~ '^#[0-9a-fA-F]{6}$' THEN nearest_space_color(p.color) ELSE 'green' END, p.id, false
    FROM projects p JOIN owners o ON o.workspace_id = p.workspace_id
  RETURNING id, project_id
)
UPDATE projects p SET space_id = made.id FROM made WHERE made.project_id = p.id;

-- A chat that had a project and no space is in that project's space now. Chats already in a
-- space stay there (their space keeps the project as where new tickets go); app chats (hidden)
-- keep no space.
UPDATE chat_sessions c SET space_id = p.space_id
  FROM projects p JOIN chat_spaces s ON s.id = p.space_id
 WHERE c.project_id = p.id AND c.space_id IS NULL AND NOT c.hidden AND c.created_by = s.user_id;

-- +goose Down
UPDATE chat_sessions SET space_id = NULL WHERE space_id IN (SELECT space_id FROM projects WHERE space_id IS NOT NULL);
-- +goose StatementBegin
DO $$
DECLARE ids uuid[];
BEGIN
  SELECT array_agg(space_id) INTO ids FROM projects WHERE space_id IS NOT NULL;
  ALTER TABLE projects DROP COLUMN space_id;
  DELETE FROM chat_spaces WHERE id = ANY(coalesce(ids, '{}'));
END $$;
-- +goose StatementEnd
DROP FUNCTION nearest_space_color(text);
