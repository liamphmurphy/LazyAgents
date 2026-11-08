package runs

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// DefaultHandoffChars bounds the rendered handoff header and history.
	DefaultHandoffChars = 24000
	// handoffRows bounds the history query; older rows are only counted.
	handoffRows = 40
	// handoffRowChars clips one message body in SQL.
	handoffRowChars = 8000
)

// HandoffMessage is one prior chat message offered to a new harness segment.
type HandoffMessage struct {
	Seq            int64
	Role           string
	Harness, Model string
	// Persona names who wrote an assistant message in a chat with several
	// personas; empty otherwise.
	Persona string
	Body    string
}

const handoffClip = "\n[…message clipped]"

func handoffHeader(omitted int) string {
	h := "This conversation continues from earlier turns answered by a different harness or model. Their messages are below, oldest first, as context only."
	if omitted > 0 {
		h += fmt.Sprintf(" %d earlier message(s) were omitted to fit.", omitted)
	}
	return h + "\n\n"
}

func handoffEntry(m HandoffMessage, body string) string {
	who := m.Role
	if m.Persona != "" {
		who += " " + m.Persona
	}
	if m.Harness != "" {
		who += " (" + m.Harness
		if m.Model != "" {
			who += "/" + m.Model
		}
		who += ")"
	}
	return fmt.Sprintf("[%d] %s:\n%s\n\n", m.Seq, who, body)
}

const handoffFooter = "---\nNew message from the user (answer this one):\n"

// relayFooter ends the handoff of a turn one persona handed to another: there
// is no new message from the user, only the hand-over note.
const relayFooter = "---\n"

// RenderHandoff builds the prompt for the first turn of a follow-up segment.
// history is newest first (as queried); total is the full count of prior
// messages. Header and history together stay within budget characters; the
// newest messages win and the header reports how many were omitted. The new
// message follows the footer, outside the budget.
func RenderHandoff(history []HandoffMessage, total int, budget int, body string) string {
	return renderHandoff(history, total, budget, handoffFooter, body)
}

func renderHandoff(history []HandoffMessage, total int, budget int, footer, body string) string {
	if budget <= 0 {
		budget = DefaultHandoffChars
	}
	if total < len(history) {
		total = len(history)
	}
	// Reserve for the longest header this call can produce.
	left := budget - utf8.RuneCountInString(handoffHeader(total))
	var kept []string
	for _, m := range history {
		entry := handoffEntry(m, m.Body)
		n := utf8.RuneCountInString(entry)
		if n > left {
			if len(kept) > 0 {
				break
			}
			// The newest message alone is too big: keep its start.
			room := left - utf8.RuneCountInString(handoffEntry(m, "")) - utf8.RuneCountInString(handoffClip)
			if room <= 0 {
				break
			}
			entry = handoffEntry(m, string([]rune(m.Body)[:min(room, utf8.RuneCountInString(m.Body))])+handoffClip)
			n = utf8.RuneCountInString(entry)
		}
		kept = append(kept, entry)
		left -= n
	}
	var b strings.Builder
	b.WriteString(handoffHeader(total - len(kept)))
	for i := len(kept) - 1; i >= 0; i-- {
		b.WriteString(kept[i])
	}
	b.WriteString(footer)
	b.WriteString(body)
	return b.String()
}
