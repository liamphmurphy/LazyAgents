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
		{"Explain this\n\n<!-- vesper:knowledge-context -->\nSelected passage:\n> @Ada", []uuid.UUID{}},
		{"@Writing coach, explain this\n\n<!-- vesper:knowledge-context -->\n> @Ada Lovelace", []uuid.UUID{bea.ID}},
	}
	for _, c := range cases {
		if got := PersonaMentions(c.body, ps); !slices.Equal(got, c.want) {
			t.Errorf("%q: got %v, want %v", c.body, got, c.want)
		}
	}
}

func TestRelayLimitIn(t *testing.T) {
	cases := []struct {
		body string
		want int
	}{
		{"@Ada and @Bea, discuss", 0},
		{"@Ada @Bea talk up to 25 messages about this", 25},
		{"Go for another ten rounds", 10},
		{"keep it to 2 replies please", 2},
		{"no more than 6 more turns", 6},
		{"chat 12 messages", 12},
		{"talk up to 500 messages", MaxRelayLimit},
		{"argue it out, no limit", MaxRelayLimit},
		{"I read 25 messages today", 0},
		{"fix these 3 replies", 0},
		{"talk up to 0 messages", 0},
		{"Explain this\n\n<!-- vesper:knowledge-context -->\n> argue it out, no limit", 0},
		{"talk up to 2 replies\n\n<!-- vesper:knowledge-context -->\n> talk up to 25 messages", 2},
	}
	for _, c := range cases {
		if got := RelayLimitIn(c.body); got != c.want {
			t.Errorf("%q: got %d, want %d", c.body, got, c.want)
		}
	}
}
