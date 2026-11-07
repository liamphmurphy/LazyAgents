// Package secrets stores credentials encrypted at rest (AES-256-GCM, key from
// SECRETS_KEY) and hands them only to the broker and the runner. The API is
// write-only: values never leave the server through it.
package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// ErrNoKey means SECRETS_KEY isn't configured, so secrets can't be stored or read.
var ErrNoKey = errors.New("secrets: SECRETS_KEY is not set")

// Box seals values with one AES-256-GCM key.
type Box struct{ aead cipher.AEAD }

// NewBox parses a base64 32-byte key (openssl rand -base64 32). An empty
// key returns a nil Box: storing secrets is then refused with ErrNoKey.
func NewBox(b64 string) (*Box, error) {
	if strings.TrimSpace(b64) == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil || len(key) != 32 {
		return nil, errors.New("secrets: SECRETS_KEY must be 32 bytes, base64 (openssl rand -base64 32)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// aad binds a ciphertext to the secret's identity, so a value can't be moved
// to another name or project by editing rows in the database.
func aad(ws uuid.UUID, project *uuid.UUID, name string) []byte {
	p := "workspace"
	if project != nil {
		p = project.String()
	}
	return []byte(ws.String() + "|" + p + "|" + name)
}

func (b *Box) seal(ws uuid.UUID, project *uuid.UUID, name string, plaintext []byte) (nonce, ct []byte, err error) {
	nonce = make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return nonce, b.aead.Seal(nil, nonce, plaintext, aad(ws, project, name)), nil
}

func (b *Box) open(ws uuid.UUID, project *uuid.UUID, name string, nonce, ct []byte) ([]byte, error) {
	pt, err := b.aead.Open(nil, nonce, ct, aad(ws, project, name))
	if err != nil {
		return nil, fmt.Errorf("secrets: %s can't be decrypted (wrong SECRETS_KEY, or the row was tampered with)", name)
	}
	return pt, nil
}

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*:[a-z0-9][a-z0-9-]*$`)

// DefaultGrants maps well-known secret names to the scope they back. A
// secret with a grant is injected only when that scope is allowed (§6.3).
var DefaultGrants = map[string]string{
	"git:pat":             "repo:write",
	"git:ssh":             "repo:write",
	"db:staging":          "db:migrate",
	"db:staging-readonly": "db:read-staging",
	"deploy:staging":      "deploy:staging",
}

type Service struct {
	Q   *store.Queries
	Box *Box
}

type SetInput struct {
	Name        string
	Value       string
	GrantsScope string // "" = default for the name, if any
	ProjectID   *uuid.UUID
}

// Set creates or replaces a secret's value.
func (s *Service) Set(ctx context.Context, ws uuid.UUID, by uuid.UUID, in SetInput) (store.Secret, error) {
	if s.Box == nil {
		return store.Secret{}, ErrNoKey
	}
	name := strings.ToLower(strings.TrimSpace(in.Name))
	if !nameRe.MatchString(name) {
		return store.Secret{}, apperr.Invalid("name", "must look like kind:name, e.g. git:pat or harness:anthropic")
	}
	if in.Value == "" || len(in.Value) > 16<<10 || !utf8.ValidString(in.Value) {
		return store.Secret{}, apperr.Invalid("value", "must be 1 B–16 KB of text")
	}
	grant := strings.TrimSpace(in.GrantsScope)
	if grant == "" {
		grant = DefaultGrants[name]
	}
	var grants *string
	if grant != "" {
		grants = &grant
	}
	nonce, ct, err := s.Box.seal(ws, in.ProjectID, name, []byte(in.Value))
	if err != nil {
		return store.Secret{}, err
	}
	return s.Q.UpsertSecret(ctx, store.UpsertSecretParams{
		ID: uuid.Must(uuid.NewV7()), WorkspaceID: ws, ProjectID: in.ProjectID, Name: name, GrantsScope: grants,
		Nonce: nonce, Ciphertext: ct, CreatedByUserID: &by,
	})
}

// Reveal decrypts a secret for server-side use (broker, runner). A project's
// own secret wins over the workspace one. Never expose the result via the API.
func (s *Service) Reveal(ctx context.Context, ws uuid.UUID, project *uuid.UUID, name string) (string, error) {
	if s.Box == nil {
		return "", ErrNoKey
	}
	row, err := s.Q.FindSecret(ctx, store.FindSecretParams{WorkspaceID: ws, Name: name, ProjectID: project})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperr.ErrNotFound
	} else if err != nil {
		return "", err
	}
	pt, err := s.Box.open(ws, row.ProjectID, row.Name, row.Nonce, row.Ciphertext)
	if err != nil {
		return "", err
	}
	_ = s.Q.TouchSecret(ctx, row.ID)
	return string(pt), nil
}
