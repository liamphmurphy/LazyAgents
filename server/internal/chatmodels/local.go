package chatmodels

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
)

// DiscoverLocal uses only the harness control protocol, with the same CLI login
// and binary overrides as the local runner. It never sends a user prompt or
// starts model inference, and never reads or exports login credentials.
func DiscoverLocal(ctx context.Context, name, _ string) ([]Model, error) {
	bin, args := "", []string{}
	switch name {
	case agentdef.HarnessClaudeCode:
		bin = os.Getenv("RUNNER_CLAUDE_BIN")
		if bin == "" {
			bin = "claude"
		}
		args = []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--no-session-persistence", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`}
	case agentdef.HarnessCodex:
		bin = os.Getenv("RUNNER_CODEX_BIN")
		if bin == "" {
			bin = "codex"
		}
		args = []string{"app-server"}
	default:
		return nil, fmt.Errorf("unsupported harness")
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
	return localModels(name, stdin, stdout)
}

func localModels(name string, input io.Writer, output io.Reader) ([]Model, error) {
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
					out = append(out, Model{ID: m.Value, Label: m.Label, Default: m.Value == "default", Efforts: efforts})
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
