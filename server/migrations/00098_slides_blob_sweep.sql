-- +goose Up
-- Slides image cleanup (ADR: Slides deck model and export, §10). Deck images
-- are content-addressed blobs that a deck and its copies share. Deleting a
-- deck, its chat or the Slides space cascades the deck_assets rows, so a
-- trigger queues each released blob key here and the runner's sweep deletes
-- the blob once no deck_assets row uses the key any more.
CREATE TABLE deck_blob_sweep (
  blob_key  text PRIMARY KEY CHECK (blob_key LIKE 'slides/%'),
  queued_at timestamptz NOT NULL DEFAULT now()
);

-- +goose StatementBegin
CREATE FUNCTION deck_assets_queue_blob() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO deck_blob_sweep (blob_key) VALUES (OLD.blob_key)
  ON CONFLICT (blob_key) DO UPDATE SET queued_at = now();
  RETURN NULL;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER deck_assets_queue_blob AFTER DELETE ON deck_assets
  FOR EACH ROW EXECUTE FUNCTION deck_assets_queue_blob();

-- +goose Down
-- Lossy: keys still queued are forgotten, so their blobs stay in storage.
DROP TRIGGER deck_assets_queue_blob ON deck_assets;
DROP FUNCTION deck_assets_queue_blob();
DROP TABLE deck_blob_sweep;
