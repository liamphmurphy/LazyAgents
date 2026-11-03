package harness

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// The OpenCode 2.2.1 wire format has NOT been captured (ADR.md: "Normalization
// boundary"). This parser reads an assumed JSON-lines envelope, kept in one
// place so a real capture can replace it:
//
//	{"type":"session","session_id":"…"}
//	{"type":"text","text":"…"}                              assistant text
//	{"type":"tool_start","call_id":"…","tool":"…","input":{…}}
//	{"type":"tool_result","call_id":"…","output":"…","is_error":false}
//	{"type":"completed","usage":{"input_tokens":n,"output_tokens":n,"cached_tokens":n,"cost":x}}
//	{"type":"error","message":"…"}
//
// Success needs the "completed" marker AND a zero process exit (Exited);
// EOF alone is a failure. Unknown event types are ignored, never read as
// completion.

const (
	opencodeMaxLine   = 1 << 20
	opencodeMaxErrLen = 500
)

func (OpenCode) NewParser(model string) Parser {
	return &opencodeParser{model: model, tools: map[string]string{}}
}

type opencodeParser struct {
	model     string
	res       Result
	tools     map[string]string
	completed bool
	exited    bool
	exitCode  int
	failed    bool
}

type opencodeLine struct {
	Type      string          `json:"type"`
	SessionID string          `json:"session_id"`
	Text      string          `json:"text"`
	CallID    string          `json:"call_id"`
	Tool      string          `json:"tool"`
	Input     json.RawMessage `json:"input"`
	Output    json.RawMessage `json:"output"`
	IsError   bool            `json:"is_error"`
	Message   string          `json:"message"`
	Usage     *struct {
		Input  *int64   `json:"input_tokens"`
		Output *int64   `json:"output_tokens"`
		Cached *int64   `json:"cached_tokens"`
		Cost   *float64 `json:"cost"`
	} `json:"usage"`
}

var opencodeSecrets = regexp.MustCompile(`(?i)(bearer\s+\S+|\bsk-[A-Za-z0-9_-]{8,}|\bla_[A-Za-z0-9_-]{8,}|(api[_-]?key|token|authorization)["':= ]+[^\s"',}]+)`)

// redactSecrets removes credential-shaped substrings from text that is
// persisted or shown.
func redactSecrets(s string) string { return opencodeSecrets.ReplaceAllString(s, "[redacted]") }

func (p *opencodeParser) fail(msg string) []Event {
	if p.failed {
		return nil
	}
	p.failed = true
	p.res.IsError = true
	p.res.Error = msg
	return []Event{{"error", map[string]any{"message": msg}}}
}

func (p *opencodeParser) Line(b []byte) []Event {
	if len(b) > opencodeMaxLine {
		return p.fail("Harness event exceeded the size limit.")
	}
	trimmed := strings.TrimSpace(string(b))
	var l opencodeLine
	if err := json.Unmarshal([]byte(trimmed), &l); err != nil {
		if strings.HasPrefix(trimmed, "{") {
			return p.fail("Harness emitted a malformed event.")
		}
		return nil // stderr noise; the raw log keeps it
	}
	if l.Type == "" {
		return nil
	}
	switch l.Type {
	case "session":
		if l.SessionID == "" {
			return nil
		}
		p.res.SessionID = l.SessionID
		return []Event{{"lifecycle", map[string]any{"state": "harness_started", "harness": HarnessOpenCode, "model": p.model, "session_id": l.SessionID}}}
	case "text":
		if strings.TrimSpace(l.Text) == "" {
			return nil
		}
		p.res.Final = l.Text
		return []Event{{"message", map[string]any{"role": "assistant", "text": l.Text}}}
	case "tool_start":
		p.tools[l.CallID] = l.Tool
		p.res.Usage.ToolCalls++
		var input any
		_ = json.Unmarshal(l.Input, &input)
		return []Event{{"tool_call", map[string]any{"id": l.CallID, "tool": l.Tool, "input": input}}}
	case "tool_result":
		// A failed tool (e.g. MCP isError) is not a provider failure.
		return []Event{{"tool_result", map[string]any{
			"id": l.CallID, "tool": p.tools[l.CallID], "is_error": l.IsError,
			"output": clip(redactSecrets(blockText(l.Output)), 4000),
		}}}
	case "error":
		msg := redactSecrets(strings.TrimSpace(l.Message))
		if msg == "" {
			msg = "Harness reported an error."
		}
		msg = clip(msg, opencodeMaxErrLen)
		if UsageLimit(msg) {
			p.res.UsageLimited = true
		}
		return p.fail(msg)
	case "completed":
		if p.completed {
			return nil // repeated terminal summary: usage counts once
		}
		p.completed = true
		return p.usage(l)
	}
	return nil
}

func (p *opencodeParser) usage(l opencodeLine) []Event {
	pl := map[string]any{"tool_calls": p.res.Usage.ToolCalls, "cost_source": "unknown", "cost_known": false}
	if u := l.Usage; u != nil {
		for _, c := range []struct {
			v    *int64
			key  string
			dest *int64
		}{{u.Input, "input_tokens", &p.res.Usage.InputTokens}, {u.Output, "output_tokens", &p.res.Usage.OutputTokens}, {u.Cached, "cached_tokens", &p.res.Usage.CachedTokens}} {
			if c.v != nil && *c.v >= 0 { // missing counters stay unavailable
				*c.dest = *c.v
				pl[c.key] = *c.v
			}
		}
		if u.Cost != nil && *u.Cost >= 0 && !math.IsInf(*u.Cost, 0) && !math.IsNaN(*u.Cost) {
			// Explicit native cost, including zero. Prices are not guessed:
			// computed cost needs verified accounting units.
			p.res.Usage.CostUSD, p.res.Usage.CostSource = *u.Cost, "harness"
			pl["cost_source"], pl["cost_known"], pl["cost_usd"] = "harness", true, *u.Cost
		}
	}
	if p.res.Usage.CostSource == "" {
		p.res.Usage.CostSource = "unknown"
	}
	return []Event{{"usage", pl}}
}

// Exited records the process exit status. The runner calls it after the stream
// ends; a nonzero exit fails the turn even if a completion marker was seen.
func (p *opencodeParser) Exited(code int) {
	p.exited, p.exitCode = true, code
}

func (p *opencodeParser) Result() Result {
	r := p.res
	if !r.IsError {
		switch {
		case p.exited && p.exitCode != 0:
			r.IsError, r.Error = true, fmt.Sprintf("Harness exited with code %d.", p.exitCode)
		case !p.completed || !p.exited:
			r.IsError, r.Error = true, "Harness ended without completion."
		}
	}
	if r.IsError {
		r.Final = "" // never present partial text as a successful reply
	}
	return r
}
