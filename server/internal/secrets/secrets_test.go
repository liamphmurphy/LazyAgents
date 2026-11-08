package secrets

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
)

func key() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func TestBoxBindsIdentity(t *testing.T) {
	b, err := NewBox(key())
	if err != nil {
		t.Fatal(err)
	}
	ws, p := uuid.New(), uuid.New()
	nonce, ct, _ := b.seal(ws, nil, "git:pat", []byte("ghp_secret"))
	if pt, err := b.open(ws, nil, "git:pat", nonce, ct); err != nil || string(pt) != "ghp_secret" {
		t.Fatalf("round trip: %q %v", pt, err)
	}
	if _, err := b.open(ws, nil, "deploy:staging", nonce, ct); err == nil {
		t.Fatal("ciphertext moved to another name decrypted")
	}
	if _, err := b.open(ws, &p, "git:pat", nonce, ct); err == nil {
		t.Fatal("ciphertext moved to a project decrypted")
	}
	other, _ := NewBox(key())
	if _, err := other.open(ws, nil, "git:pat", nonce, ct); err == nil {
		t.Fatal("wrong key decrypted")
	}
	if _, err := NewBox("dG9vIHNob3J0"); err == nil {
		t.Fatal("short key accepted")
	}
	if b, err := NewBox(""); b != nil || err != nil {
		t.Fatal("empty key should mean no box, no error")
	}
}

func TestSetRevealAndProjectPrecedence(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	ws := me.WorkspaceID
	pay, _ := q.GetProjectByKey(ctx, store.GetProjectByKeyParams{WorkspaceID: ws, Key: "PAY"})
	box, _ := NewBox(key())
	s := &Service{Q: q, Box: box}

	sec, err := s.Set(ctx, ws, me.ID, SetInput{Name: "db:staging", Value: "postgres://ws-level-dsn-1234"})
	if err != nil {
		t.Fatal(err)
	}
	if sec.GrantsScope == nil || *sec.GrantsScope != "db:migrate" {
		t.Fatalf("defaults: grants=%v", sec.GrantsScope)
	}
	// Replacing keeps one row and still decrypts (identity-bound, not id-bound).
	if _, err := s.Set(ctx, ws, me.ID, SetInput{Name: "db:staging", Value: "postgres://ws-level-dsn-5678"}); err != nil {
		t.Fatal(err)
	}
	if v, err := s.Reveal(ctx, ws, nil, "db:staging"); err != nil || v != "postgres://ws-level-dsn-5678" {
		t.Fatalf("reveal after replace: %q %v", v, err)
	}
	// A project secret wins for that project only.
	if _, err := s.Set(ctx, ws, me.ID, SetInput{Name: "db:staging", Value: "postgres://pay-dsn", ProjectID: &pay.ID}); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Reveal(ctx, ws, &pay.ID, "db:staging"); v != "postgres://pay-dsn" {
		t.Fatalf("project secret = %q", v)
	}
	other := uuid.New()
	if v, _ := s.Reveal(ctx, ws, &other, "db:staging"); v != "postgres://ws-level-dsn-5678" {
		t.Fatalf("fallback to workspace = %q", v)
	}
	rows, _ := q.ListSecrets(ctx, ws)
	if len(rows) != 2 {
		t.Fatalf("secrets = %d, want 2", len(rows))
	}
	if _, err := s.Reveal(ctx, ws, nil, "nope:missing"); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("missing = %v", err)
	}
	var ve *apperr.ValidationError
	if _, err := s.Set(ctx, ws, me.ID, SetInput{Name: "Bad Name", Value: "x"}); !errors.As(err, &ve) {
		t.Fatalf("bad name: %v", err)
	}
	if _, err := (&Service{Q: q}).Set(ctx, ws, me.ID, SetInput{Name: "git:pat", Value: "x"}); !errors.Is(err, ErrNoKey) {
		t.Fatalf("no key: %v", err)
	}
}
