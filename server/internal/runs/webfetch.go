package runs

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

// webFetchAllowed is the assistant's WebFetch allowlist (ADR "Assistant web
// access"): the exact URL must be one the person wrote in the conversation, or
// a link in the Links array of one of the run's own WebSearch results. Only
// that array counts. The result's header echoes the query, and the run
// controls the query, so text there must never whitelist a URL.
func webFetchAllowed(ctx context.Context, q *store.Queries, run uuid.UUID, url string) (bool, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return false, nil
	}
	if mentioned, err := q.ChatUserMentions(ctx, store.ChatUserMentionsParams{Text: url, RunID: &run}); err != nil || mentioned {
		return mentioned, err
	}
	rows, err := q.RunWebSearchEvents(ctx, run)
	if err != nil {
		return false, err
	}
	queries := map[string]string{}
	for _, r := range rows {
		var p struct {
			ID      string `json:"id"`
			IsError bool   `json:"is_error"`
			Output  string `json:"output"`
			Input   struct {
				Query string `json:"query"`
			} `json:"input"`
		}
		if json.Unmarshal(r.Payload, &p) != nil || p.ID == "" {
			continue
		}
		if r.Type == "tool_call" {
			if _, seen := queries[p.ID]; !seen { // a repeated id can't swap the query
				queries[p.ID] = p.Input.Query
			}
			continue
		}
		query, ok := queries[p.ID]
		if !ok || p.IsError {
			continue
		}
		for _, link := range SearchResultLinks(query, p.Output) {
			if link == url {
				return true, nil
			}
		}
	}
	return false, nil
}

// SearchResultLinks returns the url fields of a Claude Code WebSearch result's
// Links array. The result must start with the header echoing exactly this
// query; anything else yields nothing (fail closed). A result clipped
// mid-array keeps the links decoded before the cut.
func SearchResultLinks(query, output string) []string {
	header := "Web search results for query: \"" + query + "\""
	rest, ok := strings.CutPrefix(output, header)
	if !ok {
		return nil
	}
	rest = strings.TrimLeft(rest, "\r\n")
	rest, ok = strings.CutPrefix(rest, "Links:")
	if !ok {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(rest))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('[') {
		return nil
	}
	var out []string
	for dec.More() {
		var l struct {
			URL string `json:"url"`
		}
		if dec.Decode(&l) != nil {
			break
		}
		if l.URL != "" {
			out = append(out, l.URL)
		}
	}
	return out
}
