-- +goose Up
-- Events can share a timestamp (one transaction, or seeded history), and
-- UUIDv7 is random within a millisecond. seq gives a total insertion order.
ALTER TABLE activity_events ADD COLUMN seq bigint GENERATED ALWAYS AS IDENTITY;
DROP INDEX activity_ticket_idx;
CREATE INDEX activity_ticket_idx ON activity_events (ticket_id, created_at, seq);

-- +goose Down
DROP INDEX activity_ticket_idx;
CREATE INDEX activity_ticket_idx ON activity_events (ticket_id, created_at);
ALTER TABLE activity_events DROP COLUMN seq;
