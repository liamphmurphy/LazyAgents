-- +goose Up
-- An app space is one Vesper keeps for one of its apps, e.g. the Finance
-- app's Ask chats: at most one per app per person. It's still an ordinary
-- space, so its instructions and memory reach the chats in it.
ALTER TABLE chat_spaces ADD COLUMN app text CHECK (app IN ('finance'));
CREATE UNIQUE INDEX chat_spaces_app ON chat_spaces (workspace_id, user_id, app) WHERE app IS NOT NULL;

-- Finance data belongs to the person, not to the space: deleting the Finance
-- space only loses its chats, and a new one is made the next time the app
-- opens.
CREATE TABLE finance_accounts (
  id             uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id   uuid NOT NULL REFERENCES workspaces(id),
  user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name           text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
  kind           text NOT NULL CHECK (kind IN ('checking', 'savings', 'credit', 'brokerage', 'other')),
  -- The last digits the bank shows, e.g. 4417.
  mask           text NOT NULL DEFAULT '' CHECK (length(mask) <= 8),
  -- The account id inside OFX files, which matches later imports to this account.
  ofx_account_id text,
  created_at     timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, user_id, name)
);
CREATE UNIQUE INDEX finance_accounts_ofx ON finance_accounts (workspace_id, user_id, ofx_account_id) WHERE ofx_account_id IS NOT NULL;

-- Balances as a statement or the person reported them. Net worth rolls the
-- newest one per account backwards and forwards with the transactions.
CREATE TABLE finance_balances (
  account_id    uuid NOT NULL REFERENCES finance_accounts(id) ON DELETE CASCADE,
  as_of         date NOT NULL,
  -- Negative for debt, e.g. a credit card's balance owed.
  balance_cents bigint NOT NULL,
  PRIMARY KEY (account_id, as_of)
);

CREATE TABLE finance_imports (
  id          uuid PRIMARY KEY DEFAULT uuid_v7(),
  account_id  uuid NOT NULL REFERENCES finance_accounts(id) ON DELETE CASCADE,
  filename    text NOT NULL,
  format      text NOT NULL CHECK (format IN ('csv', 'ofx')),
  parsed      integer NOT NULL,
  inserted    integer NOT NULL,
  duplicates  integer NOT NULL,
  transfers   integer NOT NULL,
  categorized integer NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX finance_imports_account ON finance_imports (account_id, created_at);

CREATE TABLE finance_transactions (
  id              uuid PRIMARY KEY DEFAULT uuid_v7(),
  account_id      uuid NOT NULL REFERENCES finance_accounts(id) ON DELETE CASCADE,
  import_id       uuid REFERENCES finance_imports(id) ON DELETE SET NULL,
  posted_on       date NOT NULL,
  -- Negative is money out of the account.
  amount_cents    bigint NOT NULL CHECK (amount_cents <> 0),
  description     text NOT NULL,
  -- The description normalized for matching rules: upper case, no digits or punctuation.
  merchant        text NOT NULL,
  -- NULL until categorized: the "Needs review" queue. 'Transfer' is money
  -- moving between the person's own accounts, left out of income and spend.
  category        text,
  -- Who set the category: a built-in keyword, the person's rule, the person
  -- (or the assistant for them) on this transaction, or transfer matching.
  -- Rules never overwrite 'person' or 'match'.
  category_source text CHECK (category_source IN ('default', 'rule', 'person', 'match')),
  -- The other side of a matched transfer.
  transfer_id     uuid REFERENCES finance_transactions(id) ON DELETE SET NULL,
  -- OFX's FITID, else a hash of the row and its occurrence in the file:
  -- re-importing an overlapping export adds nothing twice.
  dedupe_key      text NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (account_id, dedupe_key),
  CHECK ((category IS NULL) = (category_source IS NULL))
);
CREATE INDEX finance_transactions_posted ON finance_transactions (account_id, posted_on);
CREATE INDEX finance_transactions_review ON finance_transactions (account_id, posted_on DESC) WHERE category IS NULL;

-- The person's categorization rules: a transaction whose merchant starts with
-- pattern gets category.
CREATE TABLE finance_rules (
  id           uuid PRIMARY KEY DEFAULT uuid_v7(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  pattern      text NOT NULL CHECK (length(pattern) BETWEEN 1 AND 80),
  category     text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, user_id, pattern)
);

-- +goose Down
DROP TABLE finance_rules;
DROP TABLE finance_transactions;
DROP TABLE finance_imports;
DROP TABLE finance_balances;
DROP TABLE finance_accounts;
DROP INDEX chat_spaces_app;
ALTER TABLE chat_spaces DROP COLUMN app;
