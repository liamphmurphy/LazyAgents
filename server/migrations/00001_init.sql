-- +goose Up
-- Phase 0 baseline. Domain tables arrive in later migrations (PLAN.md §4).

-- UUIDv7 primary keys: time-ordered so btree inserts stay append-mostly.
-- Postgres 18 ships uuidv7(); this is a compatible implementation for 17.
-- +goose StatementBegin
CREATE FUNCTION uuid_v7() RETURNS uuid
LANGUAGE plpgsql VOLATILE AS $$
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
-- +goose StatementEnd

-- Keeps updated_at current on every table that opts in.
-- +goose StatementBegin
CREATE FUNCTION touch_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS touch_updated_at();
DROP FUNCTION IF EXISTS uuid_v7();
