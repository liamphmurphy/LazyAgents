-- +goose Up
-- The dock originally created ordinary chats. Match its generated first
-- messages rather than titles (which people and personas can rename), and
-- record only the visibility flags changed here so rollback is exact.
CREATE TABLE knowledge_hidden_chat_backfill (
 chat_id uuid PRIMARY KEY REFERENCES chat_sessions(id) ON DELETE CASCADE
);
INSERT INTO knowledge_hidden_chat_backfill(chat_id)
SELECT c.id FROM chat_sessions c
JOIN chat_messages m ON m.session_id = c.id AND m.seq = 1 AND m.role = 'user'
WHERE NOT c.hidden AND c.space_id IS NULL AND (
 m.body_md = 'I''m in Knowledge, my notes and documents. Help me find, connect and write pages: search_knowledge before answering.'
 OR m.body_md LIKE 'I''m working on the Knowledge page [[%]]. Read it with read!_page and help me with it.' ESCAPE '!'
);
UPDATE chat_sessions SET hidden = true
WHERE id IN (SELECT chat_id FROM knowledge_hidden_chat_backfill);

-- +goose Down
UPDATE chat_sessions SET hidden = false
WHERE id IN (SELECT chat_id FROM knowledge_hidden_chat_backfill);
DROP TABLE knowledge_hidden_chat_backfill;
