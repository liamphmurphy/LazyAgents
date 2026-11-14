-- +goose Up
-- Native Google Antigravity sign-in (agy acp --login -> harness:google) and
-- runner-side `agy models` listings.
ALTER TABLE sign_ins DROP CONSTRAINT IF EXISTS sign_ins_provider_check;
ALTER TABLE sign_ins ADD CONSTRAINT sign_ins_provider_check
  CHECK (provider IN ('claude', 'codex', 'github', 'antigravity'));

ALTER TABLE harness_model_listings DROP CONSTRAINT IF EXISTS harness_model_listings_harness_check;
ALTER TABLE harness_model_listings ADD CONSTRAINT harness_model_listings_harness_check
  CHECK (harness IN ('claude-code', 'codex', 'antigravity'));

-- +goose Down
DELETE FROM sign_ins WHERE provider = 'antigravity';
ALTER TABLE sign_ins DROP CONSTRAINT IF EXISTS sign_ins_provider_check;
ALTER TABLE sign_ins ADD CONSTRAINT sign_ins_provider_check
  CHECK (provider IN ('claude', 'codex', 'github'));

DELETE FROM harness_model_listings WHERE harness = 'antigravity';
ALTER TABLE harness_model_listings DROP CONSTRAINT IF EXISTS harness_model_listings_harness_check;
ALTER TABLE harness_model_listings ADD CONSTRAINT harness_model_listings_harness_check
  CHECK (harness IN ('claude-code', 'codex'));
