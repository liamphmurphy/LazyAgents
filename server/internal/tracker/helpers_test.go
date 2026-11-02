package tracker_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

func ptr[T any](v T) *T { return &v }

func payloadID(t *testing.T, e store.ActivityEvent, field string) uuid.UUID {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(e.Payload, &m); err != nil {
		t.Fatal(err)
	}
	s, _ := m[field].(string)
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("payload %s = %v", field, m[field])
	}
	return id
}
