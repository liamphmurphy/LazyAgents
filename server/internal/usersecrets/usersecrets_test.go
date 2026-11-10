package usersecrets

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/secrets"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
)

func box(t *testing.T) *secrets.Box {
	t.Helper()
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	b, err := secrets.NewBox(base64.StdEncoding.EncodeToString(k))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCreateStoresEncryptedAndDerivesHandles(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	me, _ := q.GetLocalUser(ctx)
	ws := me.WorkspaceID
	s := &Service{Q: q, Box: box(t)}
	value := jwt(t)

	in := Input{Name: "Acme API JWT", Value: value, Placement: PlacementBearer, AllowedHosts: []string{"https://API.acme.com/v1", "api.acme.com"}}
	sec, err := s.Create(ctx, ws, me.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if sec.Handle != "acme-api-jwt" || len(sec.AllowedHosts) != 1 || sec.AllowedHosts[0] != "api.acme.com" {
		t.Fatalf("stored %q %v", sec.Handle, sec.AllowedHosts)
	}
	if sec.ReadPolicy != PolicyAllow || sec.WritePolicy != PolicyAsk || sec.ExpiresAt == nil {
		t.Fatalf("defaults: %s %s exp=%v", sec.ReadPolicy, sec.WritePolicy, sec.ExpiresAt)
	}
	if string(sec.Ciphertext) == value {
		t.Fatal("stored in plain text")
	}
	if got, err := s.Reveal(sec); err != nil || got != value {
		t.Fatalf("reveal: %v", err)
	}
	// The ciphertext is bound to its row: copied onto another secret, it won't open.
	again, err := s.Create(ctx, ws, me.ID, in)
	if err != nil || again.Handle != "acme-api-jwt-2" {
		t.Fatalf("second handle %q %v", again.Handle, err)
	}
	again.Nonce, again.Ciphertext = sec.Nonce, sec.Ciphertext
	if _, err := s.Reveal(again); err == nil {
		t.Fatal("a value moved to another row decrypted")
	}
	// A new value replaces the old one and its expiry.
	if sec, err = s.SetValue(ctx, sec, PlacementBearer, "sk_live_1234567890"); err != nil || sec.ExpiresAt != nil {
		t.Fatalf("set value: exp=%v %v", sec.ExpiresAt, err)
	}

	var ve *apperr.ValidationError
	for name, bad := range map[string]Input{
		"no hosts":      {Name: "x", Value: "v", Placement: PlacementBearer},
		"wildcard host": {Name: "x", Value: "v", Placement: PlacementBearer, AllowedHosts: []string{"*.acme.com"}},
		"no header":     {Name: "x", Value: "v", Placement: PlacementHeader, AllowedHosts: []string{"a.com"}},
		"cookie header": {Name: "x", Value: "v", Placement: PlacementHeader, PlacementKey: "Cookie", AllowedHosts: []string{"a.com"}},
		"basic no pass": {Name: "x", Value: "v", Placement: PlacementBasic, AllowedHosts: []string{"a.com"}},
		"empty value":   {Name: "x", Placement: PlacementBearer, AllowedHosts: []string{"a.com"}},
		"bad policy":    {Name: "x", Value: "v", Placement: PlacementBearer, AllowedHosts: []string{"a.com"}, WritePolicy: "maybe"},
	} {
		if _, err := s.Create(ctx, ws, me.ID, bad); !errors.As(err, &ve) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := (&Service{Q: q}).Create(ctx, ws, me.ID, in); !errors.Is(err, ErrNoKey) {
		t.Fatalf("no key: %v", err)
	}
}
