package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

// A persona's own memory loads in full every turn, up to PersonaBudget
// characters: who it is first, with bodies, then its newest moments as one
// line each (at most personaMoments, in personaMomentBudget characters).
const (
	PersonaBudget       = 12000
	personaMomentBudget = 3000
	personaMoments      = 30
	personaFetch        = 200
)

// PersonaContext returns the live entries of o.PersonaID's own memory to load
// into a chat (who it is first, then moments newest first) and how many live
// entries it has in all.
func (s *Service) PersonaContext(ctx context.Context, o Owner) ([]store.Memory, int, error) {
	if o.PersonaID == nil {
		return nil, 0, nil
	}
	o = o.Persona(*o.PersonaID)
	rows, err := s.Q.PersonaContextMemories(ctx, store.PersonaContextMemoriesParams{
		WorkspaceID: o.WorkspaceID, UserID: o.UserID, PersonaID: o.PersonaID, Lim: personaFetch})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.Count(ctx, o, Query{})
	return rows, total, err
}

// PersonaMarkdown renders a persona's own memory for its chat's context,
// within PersonaBudget, saying how many entries didn't fit. entries come from
// PersonaContext.
func PersonaMarkdown(entries []store.Memory, total int) string {
	if total == 0 {
		return "Nothing remembered yet.\n"
	}
	var moments strings.Builder
	shown := 0
	for _, m := range entries {
		if m.Type != TypeMoment || shown >= personaMoments {
			continue
		}
		line := fmt.Sprintf("- %s `%s` %s", m.CreatedAt.UTC().Format("2006-01-02"), m.ID, m.Title)
		if m.Summary != "" {
			line += ": " + m.Summary
		}
		line += "\n"
		if moments.Len()+len(line) > personaMomentBudget {
			break
		}
		moments.WriteString(line)
		shown++
	}

	var self strings.Builder
	left := PersonaBudget - moments.Len()
	for _, m := range entries {
		if m.Type == TypeMoment {
			continue
		}
		head := fmt.Sprintf("#### %s\n\n`%s` [%s]", m.Title, m.ID, m.Type)
		if m.Summary != "" {
			head += " " + m.Summary
		}
		block := head + "\n\n"
		if m.Body != "" {
			block = head + "\n\n" + m.Body + "\n\n"
		}
		if len(block) > left {
			// Too long to load whole: the title and summary still say it's there.
			block = head + " (body not loaded: read_memory)\n\n"
			if len(block) > left {
				continue
			}
		}
		self.WriteString(block)
		left -= len(block)
		shown++
	}

	var b strings.Builder
	if self.Len() > 0 {
		b.WriteString("### Who you are\n\n" + self.String())
	}
	if moments.Len() > 0 {
		b.WriteString("### Shared moments\n\nNewest first, title and summary only; read_memory(id) opens one.\n\n" + moments.String() + "\n")
	}
	if total > shown {
		fmt.Fprintf(&b, "%d more of your entries aren't loaded here; search_memory with about: \"persona\" finds them.\n", total-shown)
	}
	return b.String()
}
