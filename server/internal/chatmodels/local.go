package chatmodels

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
)

// DiscoverLocal lists models through the fake backend's stand-in binary
// (RUNNER_CLAUDE_BIN, RUNNER_CODEX_BIN), over the harness control protocol.
// Without one it refuses: the API never runs a real CLI on the host. It never
// sends a user prompt or starts model inference.
func DiscoverLocal(ctx context.Context, name, _ string) ([]Model, error) {
	argv, err := DiscoveryArgv(name)
	if err != nil {
		return nil, err
	}
	args := argv[1:]
	override := map[string]string{agentdef.HarnessClaudeCode: "RUNNER_CLAUDE_BIN", agentdef.HarnessCodex: "RUNNER_CODEX_BIN", agentdef.HarnessAntigravity: "RUNNER_ANTIGRAVITY_BIN"}[name]
	bin := os.Getenv(override)
	if bin == "" {
		return nil, fmt.Errorf("chatmodels: %s is not set; only a fake harness is asked for models on the host", override)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	// Avoid loading repository instructions, hooks and MCP configurations.
	cmd.Dir = os.TempDir()
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		stdin.Close()
		return nil, fmt.Errorf("local harness could not start")
	}
	defer func() { stdin.Close(); cmd.Process.Kill(); cmd.Wait() }()
	if name == agentdef.HarnessAntigravity {
		// `agy models` prints a list and exits: nothing to converse with.
		return ParseAgyModels(io.LimitReader(stdout, 1<<20))
	}
	return ListOverStdio(name, stdin, stdout)
}

// DiscoveryArgv is the command that speaks the harness control protocol
// ListOverStdio drives, with the CLI found on PATH.
func DiscoveryArgv(name string) ([]string, error) {
	switch name {
	case agentdef.HarnessClaudeCode:
		return []string{"claude", "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--no-session-persistence", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`}, nil
	case agentdef.HarnessCodex:
		return []string{"codex", "app-server"}, nil
	case agentdef.HarnessAntigravity:
		return []string{"agy", "models"}, nil
	}
	return nil, fmt.Errorf("unsupported harness")
}

// ListOverStdio asks a running DiscoveryArgv process for its models.
func ListOverStdio(name string, input io.Writer, output io.Reader) ([]Model, error) {
	send := json.NewEncoder(input)
	if name == agentdef.HarnessClaudeCode {
		if err := send.Encode(map[string]any{"type": "control_request", "request_id": "models", "request": map[string]any{"subtype": "initialize"}}); err != nil {
			return nil, err
		}
	} else {
		if err := send.Encode(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]string{"name": "lazyagents_models", "version": "1.0"}, "capabilities": map[string]any{}}}); err != nil {
			return nil, err
		}
	}
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	out := []Model{}
	seen := map[string]bool{}
	cursors := map[string]bool{}
	id := 2
	for scanner.Scan() {
		var msg struct {
			Type     string          `json:"type"`
			ID       int             `json:"id"`
			Error    json.RawMessage `json:"error"`
			Result   json.RawMessage `json:"result"`
			Response struct {
				Subtype   string `json:"subtype"`
				RequestID string `json:"request_id"`
				Response  struct {
					Models []struct {
						Value          string   `json:"value"`
						Label          string   `json:"displayName"`
						Description    string   `json:"description"`
						SupportsEffort bool     `json:"supportsEffort"`
						Efforts        []string `json:"supportedEffortLevels"`
					} `json:"models"`
				} `json:"response"`
			} `json:"response"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			return nil, fmt.Errorf("invalid harness discovery response")
		}
		if name == agentdef.HarnessClaudeCode {
			if msg.Type != "control_response" || msg.Response.RequestID != "models" {
				continue
			}
			if msg.Response.Subtype != "success" {
				return nil, fmt.Errorf("local harness initialization failed")
			}
			for _, m := range msg.Response.Response.Models {
				if m.Value != "" && !seen[m.Value] {
					seen[m.Value] = true
					efforts := []string{}
					if m.SupportsEffort {
						efforts = knownEfforts(m.Efforts)
					}
					out = append(out, Model{ID: m.Value, Label: versionedLabel(m.Label, m.Description), Default: m.Value == "default", Efforts: efforts})
				}
			}
			return normalizeLocalModels(out), nil
		}
		if msg.ID == 0 {
			continue
		} // Ignore app-server notifications.
		if len(msg.Error) > 0 && string(msg.Error) != "null" {
			return nil, fmt.Errorf("local harness model discovery failed")
		}
		if msg.ID == 1 {
			if err := send.Encode(map[string]any{"method": "initialized"}); err != nil {
				return nil, err
			}
			if err := send.Encode(map[string]any{"id": id, "method": "model/list", "params": map[string]any{"limit": 100, "includeHidden": false}}); err != nil {
				return nil, err
			}
			continue
		}
		if msg.ID != id {
			continue
		}
		var page struct {
			Data []struct {
				Model   string `json:"model"`
				Label   string `json:"displayName"`
				Default bool   `json:"isDefault"`
				Hidden  bool   `json:"hidden"`
				Efforts []struct {
					Effort string `json:"reasoningEffort"`
				} `json:"supportedReasoningEfforts"`
				DefaultEffort string `json:"defaultReasoningEffort"`
			} `json:"data"`
			NextCursor *string `json:"nextCursor"`
		}
		if err := json.Unmarshal(msg.Result, &page); err != nil {
			return nil, fmt.Errorf("invalid model list")
		}
		for _, m := range page.Data {
			if m.Model != "" && !m.Hidden && !seen[m.Model] {
				seen[m.Model] = true
				levels := make([]string, len(m.Efforts))
				for i, e := range m.Efforts {
					levels[i] = e.Effort
				}
				efforts := knownEfforts(levels)
				def := m.DefaultEffort
				if !slices.Contains(efforts, def) {
					def = ""
				}
				out = append(out, Model{ID: m.Model, Label: m.Label, Default: m.Default, Efforts: efforts, DefaultEffort: def})
			}
		}
		if page.NextCursor == nil || *page.NextCursor == "" {
			return normalizeLocalModels(out), nil
		}
		cursor := *page.NextCursor
		if cursors[cursor] {
			return nil, fmt.Errorf("invalid model pagination")
		}
		cursors[cursor] = true
		id++
		if err := send.Encode(map[string]any{"id": id, "method": "model/list", "params": map[string]any{"limit": 100, "includeHidden": false, "cursor": cursor}}); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("local harness ended before returning models")
}

// versionedLabel names a Claude Code alias by the model it resolves to.
// The CLI labels its aliases without a version ("Fable") and leads the
// description with it ("Fable 5.2 · Most capable…"), and which version an
// alias means changes with the CLI release. A description that doesn't start
// with "<label> <version>" leaves the label as it is.
func versionedLabel(label, description string) string {
	name, _, _ := strings.Cut(description, " · ")
	version, ok := strings.CutPrefix(name, label+" ")
	if label == "" || !ok || version == "" || strings.Trim(version, "0123456789.") != "" {
		return label
	}
	return name
}

func normalizeLocalModels(models []Model) []Model {
	hasDefault := false
	for i := range models {
		if models[i].Label == "" {
			models[i].Label = models[i].ID
		}
		if models[i].Default {
			if hasDefault {
				models[i].Default = false
			}
			hasDefault = true
		}
	}
	if !hasDefault && len(models) > 0 {
		models[0].Default = true
	}
	return models
}

// agyModelSlug is what `agy models` leads a line with: a model slug such as
// gemini-3.8-flash-medium. Headings, blank lines and prose don't match.
var agyModelSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// ParseAgyModels reads the output of `agy models`: one model per line, its slug
// first, an optional description after it (a leading bullet or "*" marks the
// default). The listing's format is documented only as "list slugs with agy
// models", so anything that isn't slug-first is skipped rather than guessed at.
func ParseAgyModels(r io.Reader) ([]Model, error) {
	out := []Model{}
	seen := map[string]bool{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		isDefault := strings.HasPrefix(line, "*") || strings.Contains(strings.ToLower(line), "(default)")
		line = strings.TrimSpace(strings.TrimLeft(line, "*-• "))
		slug, rest, _ := strings.Cut(line, " ")
		if !agyModelSlug.MatchString(slug) || !strings.ContainsAny(slug, "-0123456789") || seen[slug] {
			continue
		}
		seen[slug] = true
		label := strings.TrimSpace(strings.ReplaceAll(rest, "(default)", ""))
		out = append(out, Model{ID: slug, Label: label, Default: isDefault})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("agy listed no models")
	}
	return normalizeLocalModels(out), nil
}
