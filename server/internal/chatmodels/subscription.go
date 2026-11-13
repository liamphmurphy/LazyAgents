package chatmodels

import (
	"encoding/json"
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

func subscriptionLogin(harness, key string) bool {
	switch harness {
	case agentdef.HarnessClaudeCode:
		return ClaudeOAuthToken(key)
	case agentdef.HarnessAntigravity:
		return false // an API key only: see KeySecret
	}
	_, ok := ParseCodexAuth(key)
	return ok
}
