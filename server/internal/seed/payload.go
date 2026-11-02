package seed

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

func commentID(ev store.ActivityEvent) (uuid.UUID, bool) {
	var p struct {
		CommentID uuid.UUID `json:"comment_id"`
	}
	if err := json.Unmarshal(ev.Payload, &p); err != nil || p.CommentID == uuid.Nil {
		return uuid.Nil, false
	}
	return p.CommentID, true
}
