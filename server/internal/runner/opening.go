package runner

import "strings"

// openingLine finds a turn's opening line: the assistant's first text, kept
// only once a tool call follows it. A turn that answers straight away has no
// tool call, so its only text is the reply itself.
type openingLine struct {
	first string // the first assistant text seen
	line  string // first, confirmed by a later tool call
}

func (o *openingLine) see(typ string, p map[string]any) {
	switch {
	case typ == "message" && o.first == "" && p["role"] == "assistant" && p["kind"] == nil:
		if s, ok := p["text"].(string); ok {
			o.first = strings.TrimSpace(s)
		}
	case typ == "tool_call" && o.first != "" && o.line == "":
		o.line = o.first
	}
}
