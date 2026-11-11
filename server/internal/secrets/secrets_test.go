package secrets

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"

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

func TestNormalizeSSHKey(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(private, "test")
	if err != nil {
		t.Fatal(err)
	}
	openssh := string(pem.EncodeToMemory(block))
	pkcs8, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"OpenSSH":              openssh,
		"PKCS8":                string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})),
		"RSA PEM":              string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsaKey)})),
		"CRLF":                 strings.ReplaceAll(openssh, "\n", "\r\n"),
		"no final newline":     strings.TrimSuffix(openssh, "\n"),
		"clipboard whitespace": " \n" + openssh + "\n\t",
	} {
		t.Run(name, func(t *testing.T) {
			got, err := normalizeSSHKey(value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(got, "\r") || !strings.HasSuffix(got, "\n") {
				t.Fatal("key line endings were not normalized")
			}
			parsed, err := ssh.ParseRawPrivateKey([]byte(got))
			if err != nil {
				t.Fatal(err)
			}
			want := any(private)
			if name == "RSA PEM" {
				want = rsaKey
			}
			signer, _ := ssh.NewSignerFromKey(parsed)
			original, _ := ssh.NewSignerFromKey(want)
			if string(signer.PublicKey().Marshal()) != string(original.PublicKey().Marshal()) {
				t.Fatal("normalization changed the key")
			}
		})
	}

	public, _ := ssh.NewPublicKey(private.Public())
	encrypted, err := ssh.MarshalPrivateKeyWithPassphrase(private, "test", []byte("test-passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	box, _ := NewBox(key())
	s := &Service{Box: box}
	for name, value := range map[string]string{
		"public":           string(ssh.MarshalAuthorizedKey(public)),
		"flattened":        strings.ReplaceAll(openssh, "\n", " "),
		"escaped newlines": strings.ReplaceAll(openssh, "\n", `\n`),
		"truncated":        strings.TrimSuffix(strings.TrimSpace(openssh), "-----END OPENSSH PRIVATE KEY-----"),
		"invalid":          "not-a-private-key",
		"encrypted":        string(pem.EncodeToMemory(encrypted)),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := s.Set(context.Background(), uuid.New(), uuid.New(), SetInput{Name: " GIT:SSH ", Value: value})
			var invalid *apperr.ValidationError
			if !errors.As(err, &invalid) {
				t.Fatal("invalid key must be rejected before reaching storage")
			}
			if strings.Contains(err.Error(), value) {
				t.Fatal("validation error leaked the submitted key")
			}
			if name == "encrypted" && !strings.Contains(err.Error(), "passphrase") {
				t.Fatal("encrypted key needs an actionable passphrase error")
			}
		})
	}
}

func TestSSHKeyStorageRoundTrip(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	box, _ := NewBox(key())
	s := &Service{Q: q, Box: box}
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	block, _ := ssh.MarshalPrivateKey(private, "test")
	value := string(pem.EncodeToMemory(block))
	if _, err := s.Set(ctx, me.WorkspaceID, me.ID, SetInput{Name: "git:ssh", Value: strings.ReplaceAll(value, "\n", "\r\n")}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Reveal(ctx, me.WorkspaceID, nil, "git:ssh")
	if err != nil || got != value {
		t.Fatal("stored SSH key did not retain its contents and line breaks")
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
