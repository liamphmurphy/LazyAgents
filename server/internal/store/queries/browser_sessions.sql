-- name: CreateBrowserSignInCode :exec
INSERT INTO browser_sign_in_codes (code_hash, expires_at) VALUES ($1, $2);

-- name: RedeemBrowserSignInCode :one
-- Deleting the row is what makes a code single-use, even under concurrent redeems.
DELETE FROM browser_sign_in_codes WHERE code_hash = $1 AND expires_at > now()
RETURNING code_hash;

-- name: DeleteExpiredBrowserSignInCodes :exec
DELETE FROM browser_sign_in_codes WHERE expires_at <= now();

-- name: CreateBrowserSession :exec
INSERT INTO browser_sessions (token_hash, expires_at) VALUES ($1, $2);

-- name: GetBrowserSession :one
SELECT * FROM browser_sessions WHERE token_hash = $1 AND expires_at > now();

-- name: TouchBrowserSession :exec
UPDATE browser_sessions SET last_used_at = now() WHERE token_hash = $1;

-- name: CountBrowserSessions :one
SELECT count(*) FROM browser_sessions WHERE expires_at > now();

-- name: SetOwnerPassword :one
-- Sets the owner's login and password. Unclaimed, anyone may (first-run
-- setup); claimed but passwordless, only a signed-in browser may. Once a
-- password is set this matches nothing: changing it is a separate path.
UPDATE users
SET login = sqlc.arg(login), password_hash = sqlc.arg(password_hash), password_set_at = now(), claimed_at = coalesce(claimed_at, now())
WHERE id = sqlc.arg(id) AND password_set_at IS NULL AND (claimed_at IS NULL OR sqlc.arg(signed_in)::bool)
RETURNING *;
