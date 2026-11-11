package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

// SessionCookie holds a browser's session token.
const SessionCookie = "vesper_session"

// SignInCodeTTL is how long a sign-in link works: long enough to copy it from
// a terminal, short enough that one left in scrollback or a log goes stale.
const SignInCodeTTL = 15 * time.Minute

// SessionTTL is how long a browser stays signed in.
const SessionTTL = 365 * 24 * time.Hour

// ErrBadSignInCode covers unknown, used and expired codes alike.
var ErrBadSignInCode = errors.New("auth: invalid or expired sign-in code")

func hashSecret(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

func randomSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// NewSignInCode mints a one-time code for a sign-in link. The plaintext is
// returned once; only its hash is stored.
func NewSignInCode(ctx context.Context, q *store.Queries) (string, error) {
	if err := q.DeleteExpiredBrowserSignInCodes(ctx); err != nil {
		return "", err
	}
	code, err := randomSecret()
	if err != nil {
		return "", err
	}
	err = q.CreateBrowserSignInCode(ctx, store.CreateBrowserSignInCodeParams{CodeHash: hashSecret(code), ExpiresAt: time.Now().Add(SignInCodeTTL)})
	return code, err
}

// RedeemSignInCode spends a sign-in code on a new browser session and
// returns the session token for its cookie.
func RedeemSignInCode(ctx context.Context, q *store.Queries, code string) (string, error) {
	if code == "" {
		return "", ErrBadSignInCode
	}
	if _, err := q.RedeemBrowserSignInCode(ctx, hashSecret(code)); errors.Is(err, pgx.ErrNoRows) {
		return "", ErrBadSignInCode
	} else if err != nil {
		return "", err
	}
	return NewSession(ctx, q)
}

// NewSession starts a browser session and returns the token for its cookie.
// Callers must already have proved the browser is the person.
func NewSession(ctx context.Context, q *store.Queries) (string, error) {
	token, err := randomSecret()
	if err != nil {
		return "", err
	}
	err = q.CreateBrowserSession(ctx, store.CreateBrowserSessionParams{TokenHash: hashSecret(token), ExpiresAt: time.Now().Add(SessionTTL)})
	return token, err
}

// ValidSession reports whether token belongs to an unexpired browser session.
func ValidSession(ctx context.Context, q *store.Queries, token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	row, err := q.GetBrowserSession(ctx, hashSecret(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	// Recording use is best-effort and throttled to once a minute.
	if time.Since(row.LastUsedAt) > time.Minute {
		_ = q.TouchBrowserSession(ctx, row.TokenHash)
	}
	return true, nil
}
