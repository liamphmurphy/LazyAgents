package runs

import (
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

func TestPersonaMentions(t *testing.T) {
	ada := store.Persona{ID: uuid.New(), Name: "Ada"}
	adaL := store.Persona{ID: uuid.New(), Name: "Ada Lovelace"}
	bea := store.Persona{ID: uuid.New(), Name: "Writing coach"}
	ps := []store.Persona{ada, adaL, bea}
	cases := []struct {
		body string
		want []uuid.UUID
	}{
		{"no mentions here", []uuid.UUID{}},
		{"@ada what do you think?", []uuid.UUID{ada.ID}},
		{"Hey @Ada Lovelace, and @writing coach too", []uuid.UUID{adaL.ID, bea.ID}},
		{"@Writing coach, then @Ada. Again @ada", []uuid.UUID{bea.ID, ada.ID}},
		{"mail me at me@ada.dev", []uuid.UUID{}},
		{"@Adam isn't anyone", []uuid.UUID{}},
		{"(@Ada)", []uuid.UUID{ada.ID}},
		{"@", []uuid.UUID{}},
	}
	for _, c := range cases {
		if got := PersonaMentions(c.body, ps); !slices.Equal(got, c.want) {
			t.Errorf("%q: got %v, want %v", c.body, got, c.want)
		}
	}
}
