-- +goose Up
-- Full-text search over a person's chats. The body is capped because a tsvector
-- can't exceed 1 MB, and a very long reply mustn't fail its insert.
CREATE INDEX chat_messages_search ON chat_messages USING gin (to_tsvector('english', left(body_md, 200000)));
-- The sweep that retires old link preview images.
CREATE INDEX link_previews_fetched ON link_previews (fetched_at);

-- +goose Down
DROP INDEX link_previews_fetched;
DROP INDEX chat_messages_search;
