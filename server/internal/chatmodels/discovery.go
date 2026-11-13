package chatmodels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
)

// Discover asks the provider behind each supported harness for its current models.
// Credentials and provider response bodies never leave this server.
func Discover(ctx context.Context, harness, key string) ([]Model, error) {
	endpoint := "https://api.openai.com/v1/models"
	if harness == agentdef.HarnessClaudeCode {
		endpoint = "https://api.anthropic.com/v1/models?limit=1000"
		if key == "" {
			key = os.Getenv("ANTHROPIC_API_KEY")
		}
	} else if harness == agentdef.HarnessCodex {
		if key == "" {
			key = os.Getenv("CODEX_API_KEY")
			if key == "" {
				key = os.Getenv("OPENAI_API_KEY")
			}
		}
	} else if harness == agentdef.HarnessAntigravity {
		if key == "" {
			key = os.Getenv("GEMINI_API_KEY")
		}
		if key == "" {
			return nil, fmt.Errorf("missing provider credential")
		}
		return discoverGemini(ctx, &http.Client{Timeout: 10 * time.Second}, geminiModelsEndpoint, key)
	} else {
		return nil, fmt.Errorf("unsupported harness")
	}
	if key == "" {
		return nil, fmt.Errorf("missing provider credential")
	}
	return discoverHTTP(ctx, &http.Client{Timeout: 10 * time.Second}, endpoint, harness, key)
}

func discoverHTTP(ctx context.Context, client *http.Client, endpoint, harness, key string) ([]Model, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out := []Model{}
	seen := map[string]bool{}
	for {
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if err != nil {
			return nil, err
		}
		if harness == agentdef.HarnessClaudeCode {
			req.Header.Set("x-api-key", key)
			req.Header.Set("anthropic-version", "2023-06-01")
		} else {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		res, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("provider unavailable")
		}
		var page struct {
			Data []struct {
				ID    string `json:"id"`
				Label string `json:"display_name"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		err = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&page)
		res.Body.Close()
		if res.StatusCode != 200 || err != nil {
			return nil, fmt.Errorf("provider model discovery failed (%d)", res.StatusCode)
		}
		added := 0
		for _, m := range page.Data {
			if m.ID == "" || seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			added++
			// Codex needs a text reasoning model; omit media, embedding and realtime endpoints.
			if harness == agentdef.HarnessCodex && (!strings.HasPrefix(m.ID, "gpt-") && !strings.HasPrefix(m.ID, "o1") && !strings.HasPrefix(m.ID, "o3") && !strings.HasPrefix(m.ID, "o4") || strings.Contains(m.ID, "audio") || strings.Contains(m.ID, "realtime") || strings.Contains(m.ID, "image") || strings.Contains(m.ID, "transcribe")) {
				continue
			}
			label := m.Label
			if label == "" {
				label = m.ID
			}
			out = append(out, Model{ID: m.ID, Label: label, Efforts: fallbackEfforts[harness]})
		}
		if !page.HasMore {
			break
		}
		if added == 0 || page.LastID == "" {
			return nil, fmt.Errorf("invalid model pagination")
		}
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Set("after_id", page.LastID)
		u.RawQuery = q.Encode()
		endpoint = u.String()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > 0 {
		out[0].Default = true
	}
	return out, nil
}

// geminiModelsEndpoint lists the Gemini API's models, which is what `agy`
// talks to when its modelProvider is "gemini" (the only way Vesper signs it in).
const geminiModelsEndpoint = "https://generativelanguage.googleapis.com/v1beta/models?pageSize=200"

// notChatModel matches Gemini models that can't hold a coding conversation:
// embeddings, image/audio/video generators, live and tts endpoints.
var notChatModel = regexp.MustCompile(`embed|image|imagen|veo|tts|audio|live|native|aqa|robotics|computer-use|vision|learnlm|gemma|banana`)

// discoverGemini lists the Gemini models that support generateContent. The key
// goes in a header, never the URL.
func discoverGemini(ctx context.Context, client *http.Client, endpoint, key string) ([]Model, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out := []Model{}
	seen := map[string]bool{}
	for pages := 0; pages < 10; pages++ {
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-goog-api-key", key)
		res, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("provider unavailable")
		}
		var page struct {
			Models []struct {
				Name    string   `json:"name"`
				Label   string   `json:"displayName"`
				Methods []string `json:"supportedGenerationMethods"`
			} `json:"models"`
			Next string `json:"nextPageToken"`
		}
		err = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&page)
		res.Body.Close()
		if res.StatusCode != 200 || err != nil {
			return nil, fmt.Errorf("provider model discovery failed (%d)", res.StatusCode)
		}
		for _, m := range page.Models {
			id := strings.TrimPrefix(m.Name, "models/")
			if !strings.HasPrefix(id, "gemini-") || seen[id] || notChatModel.MatchString(id) || !slices.Contains(m.Methods, "generateContent") {
				continue
			}
			seen[id] = true
			label := m.Label
			if label == "" {
				label = id
			}
			out = append(out, Model{ID: id, Label: label, Efforts: fallbackEfforts[agentdef.HarnessAntigravity]})
		}
		if page.Next == "" {
			break
		}
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Set("pageToken", page.Next)
		u.RawQuery = q.Encode()
		endpoint = u.String()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	// The default is a stable pro model when there is one, else the first.
	def := 0
	for i, m := range out {
		if strings.Contains(m.ID, "pro") && !strings.Contains(m.ID, "preview") && !strings.Contains(m.ID, "exp") {
			def = i
		}
	}
	if len(out) > 0 {
		out[def].Default = true
	}
	return out, nil
}
