package chatmodels

import (
	"encoding/json"
	"path"
	"strings"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
)

// ClaudeOAuthToken reports whether a harness:anthropic value is a plan login
// from `claude setup-token` rather than an API key.
func ClaudeOAuthToken(v string) bool { return strings.HasPrefix(v, "sk-ant-oat") }

// CodexAuth is the part of Codex's auth.json Vesper reads.
type CodexAuth struct {
	AuthMode    string    `json:"auth_mode"`
	LastRefresh time.Time `json:"last_refresh"`
	Tokens      struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		AccountID    string `json:"account_id"`
	} `json:"tokens"`
}

// ParseCodexAuth reports whether a harness:openai value is a ChatGPT login
// (an auth.json from `codex login`) rather than an API key.
func ParseCodexAuth(v string) (CodexAuth, bool) {
	var a CodexAuth
	if !strings.HasPrefix(strings.TrimSpace(v), "{") || json.Unmarshal([]byte(v), &a) != nil {
		return a, false
	}
	return a, a.AuthMode == "chatgpt" && a.Tokens.RefreshToken != ""
}

// CodexAuthRefreshed reports whether next is a later login for the same
// account as current. next comes from a container an agent could write to.
func CodexAuthRefreshed(current, next string) bool {
	cur, ok := ParseCodexAuth(current)
	if !ok {
		return false
	}
	n, ok := ParseCodexAuth(next)
	return ok && n.Tokens.AccountID == cur.Tokens.AccountID && n.LastRefresh.After(cur.LastRefresh) &&
		n.LastRefresh.Before(time.Now().Add(5*time.Minute))
}

// AntigravityAuth is the serialized container login state produced by
// `agy acp --login`, stored in harness:google and restored into a run
// container's $HOME before starting `agy`.
type AntigravityAuth struct {
	Version int               `json:"v"`
	Files   map[string]string `json:"files"`
}

// AntigravitySecretsRelPath is the file-based credential store `agy` writes
// when no D-Bus session bus is present inside headless Linux containers.
const AntigravitySecretsRelPath = ".config/antigravity/plugin-secrets.json"

// CleanAntigravityRelPath validates that rel is a safe relative path under
// either .config/antigravity or .gemini with no traversal components.
func CleanAntigravityRelPath(rel string) (string, bool) {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "\\") || strings.ContainsRune(rel, 0) {
		return "", false
	}
	cleaned := path.Clean(rel)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	if cleaned != rel {
		return "", false
	}
	if !strings.HasPrefix(cleaned, ".config/antigravity/") && !strings.HasPrefix(cleaned, ".gemini/") {
		return "", false
	}
	return cleaned, true
}

// ParseAntigravityAuth reports whether a harness:google value is a native
// Antigravity login bundle captured from `agy acp --login`.
func ParseAntigravityAuth(v string) (AntigravityAuth, bool) {
	var a AntigravityAuth
	if !strings.HasPrefix(strings.TrimSpace(v), "{") || json.Unmarshal([]byte(v), &a) != nil {
		return a, false
	}
	if a.Version != 1 || len(a.Files) == 0 {
		return AntigravityAuth{}, false
	}
	for rel, content := range a.Files {
		if _, ok := CleanAntigravityRelPath(rel); !ok || content == "" {
			return AntigravityAuth{}, false
		}
	}
	if a.Files[AntigravitySecretsRelPath] == "" {
		return AntigravityAuth{}, false
	}
	return a, true
}

func subscriptionLogin(harness, key string) bool {
	switch harness {
	case agentdef.HarnessClaudeCode:
		return ClaudeOAuthToken(key)
	case agentdef.HarnessAntigravity:
		_, ok := ParseAntigravityAuth(key)
		return ok
	}
	_, ok := ParseCodexAuth(key)
	return ok
}
