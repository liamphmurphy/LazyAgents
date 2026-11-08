-- +goose Up
-- OpenCode is gone. It only ran anthropic/ and openai/ models, so everything
-- that selected it moves to the first-party harness for the same provider. A
-- definition that was read-only stays read-only on its new harness. OpenCode
-- fallbacks are dropped: the chain's other targets still apply. Finished runs
-- keep their history as it happened.

-- +goose StatementBegin
CREATE FUNCTION pg_temp.opencode_target(model text) RETURNS text LANGUAGE sql IMMUTABLE AS $$
  SELECT CASE WHEN model LIKE 'openai/%' THEN 'codex' ELSE 'claude-code' END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION pg_temp.opencode_model(model text) RETURNS text LANGUAGE sql IMMUTABLE AS $$
  SELECT regexp_replace(model, '^(anthropic|openai)/([^#]*)(#.*)?$', '\2')
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION pg_temp.drop_opencode(spec jsonb) RETURNS jsonb LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE
  opts jsonb := coalesce(nullif(spec->'harness_options', 'null'::jsonb), '{}'::jsonb);
  readonly boolean := spec#>>'{harness_options,opencode,readonly}' = 'true';
  target text;
BEGIN
  opts := opts - 'opencode';
  IF spec->>'harness' = 'opencode' THEN
    target := pg_temp.opencode_target(spec->>'model');
    spec := jsonb_set(spec, '{harness}', to_jsonb(target));
    spec := jsonb_set(spec, '{model}', to_jsonb(pg_temp.opencode_model(spec->>'model')));
    IF readonly THEN
      opts := jsonb_set(opts, ARRAY[target],
        coalesce(opts->target, '{}'::jsonb) || '{"sandbox": "read-only", "readonly": "true"}'::jsonb);
    END IF;
  END IF;
  spec := jsonb_set(spec, '{harness_options}', opts);
  IF jsonb_typeof(spec->'fallbacks') = 'array' THEN
    -- A fallback the new primary now duplicates would fail validation.
    spec := jsonb_set(spec, '{fallbacks}', coalesce((
      SELECT jsonb_agg(f ORDER BY ord)
      FROM jsonb_array_elements(spec->'fallbacks') WITH ORDINALITY AS t(f, ord)
      WHERE f->>'harness' <> 'opencode'
        AND NOT (f->>'harness' = spec->>'harness' AND f->>'model' = spec->>'model')
    ), '[]'::jsonb));
  END IF;
  RETURN spec;
END
$$;
-- +goose StatementEnd

UPDATE agent_definition_versions SET spec = pg_temp.drop_opencode(spec)
WHERE spec->>'harness' = 'opencode'
   OR spec->'harness_options' ? 'opencode'
   OR spec->'fallbacks' @> '[{"harness": "opencode"}]'::jsonb;

-- agents.model is denormalized from the current definition.
UPDATE agents a SET model = v.spec->>'model'
FROM agent_definition_versions v
WHERE v.id = a.current_definition_version_id AND a.model ~ '^(anthropic|openai)/';

-- An override can name a provider-qualified model on its own, for an agent
-- whose definition set the harness; it follows that model's provider.
UPDATE agent_project_overrides
SET spec = jsonb_set(jsonb_set(spec, '{harness}', to_jsonb(pg_temp.opencode_target(spec->>'model'))),
                     '{model}', to_jsonb(pg_temp.opencode_model(spec->>'model')))
WHERE spec->>'model' ~ '^(anthropic|openai)/';
UPDATE agent_project_overrides SET spec = spec - 'harness' WHERE spec->>'harness' = 'opencode';

-- A chat's next message starts a fresh segment on its new harness.
UPDATE chat_sessions
SET harness = pg_temp.opencode_target(model), model = pg_temp.opencode_model(model)
WHERE harness = 'opencode';

DROP FUNCTION pg_temp.drop_opencode(jsonb);
DROP FUNCTION pg_temp.opencode_model(text);
DROP FUNCTION pg_temp.opencode_target(text);

-- +goose Down
-- One-way: the provider-qualified models and OpenCode targets aren't kept.
