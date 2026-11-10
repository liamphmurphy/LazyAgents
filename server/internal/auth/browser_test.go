package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
)

func TestExpiredSignInCodesAndSessionsDontWork(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	q := store.New(pool)

	code, err := NewSignInCode(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE browser_sign_in_codes SET expires_at = now() - interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	if _, err := RedeemSignInCode(ctx, q, code); !errors.Is(err, ErrBadSignInCode) {
		t.Fatalf("expired code: err = %v, want ErrBadSignInCode", err)
	}

	code, err = NewSignInCode(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	token, err := RedeemSignInCode(ctx, q, code)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := ValidSession(ctx, q, token); err != nil || !ok {
		t.Fatalf("fresh session: ok = %v, err = %v", ok, err)
	}
	if ok, _ := ValidSession(ctx, q, code); ok {
		t.Fatal("the sign-in code itself works as a session token")
	}
	if _, err := pool.Exec(ctx, "UPDATE browser_sessions SET expires_at = now() - interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	if ok, err := ValidSession(ctx, q, token); err != nil || ok {
		t.Fatalf("expired session: ok = %v, err = %v", ok, err)
	}
}
