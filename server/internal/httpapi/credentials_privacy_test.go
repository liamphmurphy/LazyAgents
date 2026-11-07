package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/secrets"
	"github.com/liamphmurphy/lazyagents/server/internal/seed"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/testdb"
)

func TestSecretResponsesAreMetadataOnly(t *testing.T) {
	pool := testdb.New(t)
	q := store.New(pool)
	if _, err := seed.Run(context.Background(), pool, seed.Options{Demo: true, Artifacts: &artifacts.Service{Pool: pool}}); err != nil {
		t.Fatal(err)
	}
	box, err := secrets.NewBox(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	h := (&Server{Pool: pool, Q: q, Secrets: &secrets.Service{Q: q, Box: box}}).Handler()
	for _, value := range []string{"unique-original-secret-END1", "unique-replaced-secret-END2"} {
		body, _ := json.Marshal(map[string]string{"name": "harness:openai", "value": value})
		for _, method := range []string{"PUT", "GET"} {
			req := httptest.NewRequest(method, "/api/v1/secrets", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			h.ServeHTTP(res, req)
			if res.Code != 200 {
				t.Fatalf("%s: %d %s", method, res.Code, res.Body.String())
			}
			for _, forbidden := range []string{value, "END1", "END2", `"preview"`, `"value"`, `"ciphertext"`, `"nonce"`} {
				if strings.Contains(res.Body.String(), forbidden) {
					t.Fatalf("%s response contains secret material: %s", method, forbidden)
				}
			}
			if !strings.Contains(res.Body.String(), "harness:openai") {
				t.Fatal("missing metadata")
			}
		}
	}
}
