package agents

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// TokenPrefix marks LazyAgents agent tokens so secret scanners can find them.
const TokenPrefix = "la_"

func hashToken(t string) []byte {
	h := sha256.Sum256([]byte(t))
	return h[:]
}

// IssueToken mints an agent bearer token. The plaintext is returned once and
// never stored. runID and expires are set for managed-run tokens (Phase 5).
func IssueToken(ctx context.Context, q *store.Queries, agentID uuid.UUID, kind, label string, runID, by *uuid.UUID, expires *time.Time) (string, store.AgentToken, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", store.AgentToken{}, err
	}
	token := TokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	row, err := q.CreateAgentToken(ctx, store.CreateAgentTokenParams{
		AgentID: agentID, TokenHash: hashToken(token), Prefix: token[:len(TokenPrefix)+4], Kind: kind,
		Label: strings.TrimSpace(label), RunID: runID, CreatedByUserID: by, ExpiresAt: expires,
	})
	return token, row, err
}

// Caller is an authenticated agent request.
type Caller struct {
	Agent   store.Agent
	TokenID uuid.UUID
	Kind    string     // run | unmanaged
	RunID   *uuid.UUID // set for managed-run tokens
}

// ErrBadToken covers unknown, revoked and expired tokens alike.
var ErrBadToken = errors.New("agents: invalid or revoked token")

// ResolveToken authenticates a bearer token.
func ResolveToken(ctx context.Context, q *store.Queries, token string) (Caller, error) {
	if !strings.HasPrefix(token, TokenPrefix) {
		return Caller{}, ErrBadToken
	}
	row, err := q.ResolveAgentToken(ctx, hashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return Caller{}, ErrBadToken
	} else if err != nil {
		return Caller{}, err
	}
	// Recording use is best-effort and throttled to once a minute.
	if row.TokenLastUsedAt == nil || time.Since(*row.TokenLastUsedAt) > time.Minute {
		_ = q.TouchAgentToken(ctx, row.TokenID)
	}
	return Caller{Agent: row.Agent, TokenID: row.TokenID, Kind: row.TokenKind, RunID: row.TokenRunID}, nil
}

// RevokeToken stops a token from working immediately.
func RevokeToken(ctx context.Context, q *store.Queries, agentID, tokenID uuid.UUID) error {
	n, err := q.RevokeAgentToken(ctx, store.RevokeAgentTokenParams{ID: tokenID, AgentID: agentID})
	if err != nil {
		return err
	}
	if n == 0 {
		return apperr.ErrNotFound
	}
	return nil
}
