package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/secrets"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
)

// Secret values are write-only: neither the set response nor the list ever
// carries the value or any part of it (LAZYAGENTS-44).
func TestSecretResponsesNeverIncludeValues(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	if _, err := seed.Run(ctx, pool, seed.Options{Demo: true}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	box, _ := secrets.NewBox(base64.StdEncoding.EncodeToString(k))
	h := (&Server{AnonymousAPI: true, Pool: pool, Q: q, Secrets: &secrets.Service{Q: q, Box: box}}).Handler()
	const value = "sk-ant-very-secret-value-WXYZ"
	do := func(method, body string) string {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/v1/secrets", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", method, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	set := do("PUT", `{"name":"harness:anthropic","value":"`+value+`"}`)
	list := do("GET", "")
	if !strings.Contains(list, `"harness:anthropic"`) || !strings.Contains(list, `"key_configured":true`) {
		t.Fatalf("list = %s", list)
	}
	for _, body := range []string{set, list} {
		if strings.Contains(body, "WXYZ") || strings.Contains(body, "sk-ant") || strings.Contains(body, `"preview"`) || strings.Contains(body, `"value"`) {
			t.Fatalf("response leaks the value: %s", body)
		}
	}
}
