-- Finance app. Every query is scoped to one person: (workspace_id, user_id),
-- directly or through finance_accounts.

-- name: GetAppSpace :one
SELECT * FROM chat_spaces WHERE workspace_id = @workspace_id AND user_id = @user_id AND app = @app_name::text;

-- name: CreateAppSpace :one
-- ON CONFLICT: two tabs opening the app at once get the same space.
INSERT INTO chat_spaces (workspace_id, user_id, name, description, color, instructions, connector_ids, space_memory, app)
VALUES (@workspace_id, @user_id, @name, @description, @color, @instructions, '{}', true, @app_name::text)
ON CONFLICT (workspace_id, user_id, app) WHERE app IS NOT NULL DO UPDATE SET updated_at = chat_spaces.updated_at
RETURNING *;

-- name: ListFinanceAccounts :many
-- The person's accounts in the order they were added, each with how many
-- transactions it has. LatestFinanceBalances has their balances.
SELECT a.*, (SELECT count(*) FROM finance_transactions t WHERE t.account_id = a.id)::int AS transaction_count
FROM finance_accounts a
WHERE a.workspace_id = @workspace_id AND a.user_id = @user_id
ORDER BY a.created_at, a.id;

-- name: GetFinanceAccount :one
SELECT * FROM finance_accounts WHERE id = @id AND workspace_id = @workspace_id AND user_id = @user_id;

-- name: GetFinanceAccountByOFX :one
SELECT * FROM finance_accounts WHERE workspace_id = @workspace_id AND user_id = @user_id AND ofx_account_id = @ofx_account_id;

-- name: CreateFinanceAccount :one
INSERT INTO finance_accounts (workspace_id, user_id, name, kind, mask, ofx_account_id)
VALUES (@workspace_id, @user_id, @name, @kind, @mask, sqlc.narg(ofx_account_id))
RETURNING *;

-- name: SetFinanceAccountOFX :exec
-- The first OFX import into an account chosen by hand ties later ones to it.
UPDATE finance_accounts SET ofx_account_id = @ofx_account_id WHERE id = @id AND ofx_account_id IS NULL;

-- name: UpsertFinanceBalance :exec
INSERT INTO finance_balances (account_id, as_of, balance_cents) VALUES (@account_id, @as_of, @balance_cents)
ON CONFLICT (account_id, as_of) DO UPDATE SET balance_cents = EXCLUDED.balance_cents;

-- name: LatestFinanceBalances :many
SELECT DISTINCT ON (b.account_id) b.account_id, b.as_of, b.balance_cents
FROM finance_balances b JOIN finance_accounts a ON a.id = b.account_id
WHERE a.workspace_id = @workspace_id AND a.user_id = @user_id
ORDER BY b.account_id, b.as_of DESC;

-- name: CreateFinanceImport :one
INSERT INTO finance_imports (account_id, filename, format, parsed, inserted, duplicates, transfers, categorized)
VALUES (@account_id, @filename, @format, @parsed, 0, 0, 0, 0)
RETURNING *;

-- name: FinishFinanceImport :one
UPDATE finance_imports SET inserted = @inserted, duplicates = @duplicates, transfers = @transfers, categorized = @categorized
WHERE id = @id
RETURNING *;

-- name: InsertFinanceTransaction :one
-- No row (pgx.ErrNoRows) when the account already has it.
INSERT INTO finance_transactions (account_id, import_id, posted_on, amount_cents, description, merchant, category, category_source, dedupe_key)
VALUES (@account_id, @import_id, @posted_on, @amount_cents, @description, @merchant, sqlc.narg(category), sqlc.narg(category_source), @dedupe_key)
ON CONFLICT (account_id, dedupe_key) DO NOTHING
RETURNING *;

-- name: GetFinanceTransaction :one
SELECT t.* FROM finance_transactions t JOIN finance_accounts a ON a.id = t.account_id
WHERE t.id = @id AND a.workspace_id = @workspace_id AND a.user_id = @user_id;

-- name: SetFinanceTransactionCategory :exec
-- Choosing a category by hand ends a matched transfer on both sides.
UPDATE finance_transactions SET category = @category, category_source = @category_source,
  transfer_id = CASE WHEN @category = 'Transfer' THEN transfer_id END
WHERE id = @id;

-- name: UnlinkFinanceTransfer :exec
-- The other side of a transfer the person re-categorized goes back to review.
UPDATE finance_transactions SET transfer_id = NULL, category = NULL, category_source = NULL
WHERE transfer_id = @id AND category_source = 'match';

-- name: FindFinanceTransferMatch :one
-- The other side of a transfer: the opposite amount in another of the
-- person's accounts within five days, not already matched, and not given
-- another category by hand.
SELECT t.* FROM finance_transactions t JOIN finance_accounts a ON a.id = t.account_id
WHERE a.workspace_id = @workspace_id AND a.user_id = @user_id AND t.account_id <> @account_id
  AND t.amount_cents = -(@amount_cents::bigint) AND t.transfer_id IS NULL
  AND t.posted_on BETWEEN @earliest::date AND @latest::date
  AND (t.category_source IS DISTINCT FROM 'person' OR t.category = 'Transfer')
ORDER BY abs(t.posted_on - @posted_on::date), t.created_at
LIMIT 1;

-- name: LinkFinanceTransfer :exec
UPDATE finance_transactions SET category = 'Transfer', category_source = 'match',
  transfer_id = CASE WHEN id = @first_id::uuid THEN @second_id::uuid ELSE @first_id::uuid END
WHERE id IN (@first_id::uuid, @second_id::uuid);

-- name: ListFinanceRules :many
SELECT * FROM finance_rules WHERE workspace_id = @workspace_id AND user_id = @user_id ORDER BY length(pattern) DESC, pattern;

-- name: UpsertFinanceRule :one
INSERT INTO finance_rules (workspace_id, user_id, pattern, category) VALUES (@workspace_id, @user_id, @pattern, @category)
ON CONFLICT (workspace_id, user_id, pattern) DO UPDATE SET category = EXCLUDED.category
RETURNING *;

-- name: ApplyFinanceRule :execrows
-- A new rule recategorizes past matches, except ones the person set by hand,
-- matched transfers, and the transaction it was made from (already set).
UPDATE finance_transactions t SET category = @category::text, category_source = 'rule'
FROM finance_accounts a
WHERE a.id = t.account_id AND a.workspace_id = @workspace_id AND a.user_id = @user_id
  AND (t.merchant = @pattern::text OR starts_with(t.merchant, @pattern::text || ' ')) AND t.id <> @except_id
  AND (t.category_source IS NULL OR t.category_source IN ('default', 'rule'))
  AND t.category IS DISTINCT FROM @category::text;

-- name: FinanceMonthlyTotals :many
-- Per month since from: money in and out, leaving transfers out, and the
-- transfers out on their own (what counting card payments would add).
SELECT date_trunc('month', t.posted_on)::date AS month,
  coalesce(sum(t.amount_cents) FILTER (WHERE t.amount_cents > 0 AND t.category IS DISTINCT FROM 'Transfer'), 0)::bigint AS inflow_cents,
  coalesce(-sum(t.amount_cents) FILTER (WHERE t.amount_cents < 0 AND t.category IS DISTINCT FROM 'Transfer'), 0)::bigint AS outflow_cents,
  coalesce(-sum(t.amount_cents) FILTER (WHERE t.amount_cents < 0 AND t.category = 'Transfer'), 0)::bigint AS transfer_out_cents
FROM finance_transactions t JOIN finance_accounts a ON a.id = t.account_id
WHERE a.workspace_id = @workspace_id AND a.user_id = @user_id AND t.posted_on >= @from_date::date
GROUP BY 1 ORDER BY 1;

-- name: FinanceAccountDailyNet :many
-- Every account's net change per day, oldest first, which rolls its newest
-- balance back and forward into a balance on any date.
SELECT t.account_id, t.posted_on, sum(t.amount_cents)::bigint AS net_cents
FROM finance_transactions t JOIN finance_accounts a ON a.id = t.account_id
WHERE a.workspace_id = @workspace_id AND a.user_id = @user_id
GROUP BY 1, 2 ORDER BY 1, 2;

-- name: FinanceCategoryMonthly :many
-- Spending per category per month since from; uncategorized spending is its own bucket.
SELECT date_trunc('month', t.posted_on)::date AS month, coalesce(t.category, 'Uncategorized')::text AS category,
  (-sum(t.amount_cents))::bigint AS spent_cents
FROM finance_transactions t JOIN finance_accounts a ON a.id = t.account_id
WHERE a.workspace_id = @workspace_id AND a.user_id = @user_id AND t.posted_on >= @from_date::date
  AND t.amount_cents < 0 AND t.category IS DISTINCT FROM 'Transfer'
GROUP BY 1, 2 ORDER BY 1, 3 DESC;

-- name: FinanceReviewQueue :many
-- Uncategorized transactions, newest first.
SELECT t.*, a.name AS account_name, a.mask AS account_mask
FROM finance_transactions t JOIN finance_accounts a ON a.id = t.account_id
WHERE a.workspace_id = @workspace_id AND a.user_id = @user_id AND t.category IS NULL
ORDER BY t.posted_on DESC, t.id
LIMIT @row_limit;

-- name: FinanceCounts :one
SELECT count(t.id)::int AS total, count(t.id) FILTER (WHERE t.category IS NULL)::int AS uncategorized
FROM finance_transactions t JOIN finance_accounts a ON a.id = t.account_id
WHERE a.workspace_id = @workspace_id AND a.user_id = @user_id;

-- name: LastFinanceImport :one
SELECT i.* FROM finance_imports i JOIN finance_accounts a ON a.id = i.account_id
WHERE a.workspace_id = @workspace_id AND a.user_id = @user_id
ORDER BY i.created_at DESC LIMIT 1;

-- name: FinanceOutflowsSince :many
-- Spending since from, oldest first: what recurring charges are found in.
SELECT t.merchant, t.description, t.posted_on, t.amount_cents
FROM finance_transactions t JOIN finance_accounts a ON a.id = t.account_id
WHERE a.workspace_id = @workspace_id AND a.user_id = @user_id AND t.posted_on >= @from_date::date
  AND t.amount_cents < 0 AND t.category IS DISTINCT FROM 'Transfer'
ORDER BY t.posted_on, t.id;

-- name: FinanceMerchantCategories :many
-- How the person categorized each merchant's first word, most used first:
-- the review queue's suggestions.
SELECT split_part(t.merchant, ' ', 1)::text AS word, t.category::text AS category, count(*)::int AS uses
FROM finance_transactions t JOIN finance_accounts a ON a.id = t.account_id
WHERE a.workspace_id = @workspace_id AND a.user_id = @user_id AND t.category IS NOT NULL AND t.category <> 'Transfer'
  AND t.category_source IN ('person', 'rule')
GROUP BY 1, 2 ORDER BY 3 DESC;

-- name: QueryFinanceTransactions :many
-- Transactions matching every filter given, newest first, with the count and
-- sum of all matches (not just the page).
SELECT t.*, a.name AS account_name, a.mask AS account_mask,
  count(*) OVER ()::int AS match_count, sum(t.amount_cents) OVER ()::bigint AS match_sum_cents
FROM finance_transactions t JOIN finance_accounts a ON a.id = t.account_id
WHERE a.workspace_id = @workspace_id AND a.user_id = @user_id
  AND (sqlc.narg(from_date)::date IS NULL OR t.posted_on >= sqlc.narg(from_date)::date)
  AND (sqlc.narg(to_date)::date IS NULL OR t.posted_on <= sqlc.narg(to_date)::date)
  AND (sqlc.narg(category)::text IS NULL OR t.category = sqlc.narg(category)::text)
  AND (NOT @uncategorized::bool OR t.category IS NULL)
  AND (sqlc.narg(account_id)::uuid IS NULL OR t.account_id = sqlc.narg(account_id)::uuid)
  AND (sqlc.narg(search)::text IS NULL OR t.description ILIKE '%' || sqlc.narg(search)::text || '%')
  AND (sqlc.narg(direction)::text IS NULL OR (sqlc.narg(direction)::text = 'out' AND t.amount_cents < 0) OR (sqlc.narg(direction)::text = 'in' AND t.amount_cents > 0))
ORDER BY t.posted_on DESC, t.id
LIMIT @row_limit;

-- name: GetFinanceCSVMapping :one
SELECT mapping FROM finance_csv_mappings WHERE workspace_id = @workspace_id AND user_id = @user_id AND fingerprint = @fingerprint;

-- name: SaveFinanceCSVMapping :exec
INSERT INTO finance_csv_mappings (workspace_id, user_id, fingerprint, mapping) VALUES (@workspace_id, @user_id, @fingerprint, @mapping)
ON CONFLICT (workspace_id, user_id, fingerprint) DO UPDATE SET mapping = EXCLUDED.mapping, updated_at = now();

-- name: CreateFinancePendingImport :one
INSERT INTO finance_pending_imports (workspace_id, user_id, account_id, filename, data, prefill)
VALUES (@workspace_id, @user_id, @account_id, @filename, @data, @prefill)
RETURNING *;

-- name: GetFinancePendingImport :one
SELECT * FROM finance_pending_imports WHERE id = @id AND workspace_id = @workspace_id AND user_id = @user_id;

-- name: SetFinancePendingChat :exec
UPDATE finance_pending_imports SET chat_id = @chat_id WHERE id = @id;

-- name: SetFinancePendingMapping :exec
UPDATE finance_pending_imports SET mapping = @mapping, mapping_note = @mapping_note WHERE id = @id;

-- name: DeleteFinancePendingImport :execrows
-- Its hidden chat is temporary: the runner deletes it once it has sat idle for a day.
DELETE FROM finance_pending_imports WHERE id = @id AND workspace_id = @workspace_id AND user_id = @user_id;

-- name: SweepFinancePendingImports :exec
-- Pending imports nobody finished within a day.
DELETE FROM finance_pending_imports WHERE workspace_id = @workspace_id AND user_id = @user_id AND created_at < @before;
