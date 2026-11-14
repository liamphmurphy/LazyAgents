package chatmodels

import (
	"fmt"
	"testing"
	"time"
)

func codexAuth(account string, at time.Time) string {
	return fmt.Sprintf(`{"auth_mode":"chatgpt","last_refresh":%q,"tokens":{"access_token":"a","refresh_token":"r","id_token":"i","account_id":%q}}`, at.Format(time.RFC3339), account)
}

func TestParseCodexAuth(t *testing.T) {
	for v, want := range map[string]bool{
		codexAuth("acct", time.Now()):                 true,
		"sk-proj-apikey":                              false,
		`{"auth_mode":"apikey","OPENAI_API_KEY":"x"}`: false,
		`{"auth_mode":"chatgpt","tokens":{}}`:         false,
		`{not json`:                                   false,
	} {
		if _, ok := ParseCodexAuth(v); ok != want {
			t.Errorf("ParseCodexAuth(%q) = %v, want %v", v, ok, want)
		}
	}
}

func TestCodexAuthRefreshed(t *testing.T) {
	now := time.Now().Add(-time.Hour)
	cur := codexAuth("acct", now)
	for name, tc := range map[string]struct {
		next string
		want bool
	}{
		"newer":         {codexAuth("acct", now.Add(time.Minute)), true},
		"unchanged":     {cur, false},
		"older":         {codexAuth("acct", now.Add(-time.Minute)), false},
		"other account": {codexAuth("attacker", now.Add(time.Minute)), false},
		"far future":    {codexAuth("acct", time.Now().Add(time.Hour)), false},
		"garbage":       {"{}", false},
	} {
		if got := CodexAuthRefreshed(cur, tc.next); got != tc.want {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
		}
	}
}

func TestClaudeOAuthToken(t *testing.T) {
	if !ClaudeOAuthToken("sk-ant-oat01-abc") || ClaudeOAuthToken("sk-ant-api03-abc") {
		t.Fatal("token shapes misread")
	}
}

func TestParseAntigravityAuth(t *testing.T) {
	valid := `{"v":1,"files":{".config/antigravity/plugin-secrets.json":"{\"token\":\"secret\"}",".gemini/antigravity-cli/installation_id":"id-1"}}`
	for name, tc := range map[string]struct {
		v    string
		want bool
	}{
		"valid bundle":      {valid, true},
		"legacy gemini key": {"AIzaSyTestKey", false},
		"wrong version":     {`{"v":2,"files":{".config/antigravity/plugin-secrets.json":"x"}}`, false},
		"missing secrets":   {`{"v":1,"files":{".gemini/config/config.json":"{}"}}`, false},
		"empty secret file": {`{"v":1,"files":{".config/antigravity/plugin-secrets.json":""}}`, false},
		"path traversal":    {`{"v":1,"files":{".config/antigravity/plugin-secrets.json":"x",".gemini/../.ssh/id_rsa":"k"}}`, false},
		"absolute path":     {`{"v":1,"files":{".config/antigravity/plugin-secrets.json":"x","/etc/passwd":"k"}}`, false},
		"disallowed dotdir": {`{"v":1,"files":{".config/antigravity/plugin-secrets.json":"x",".ssh/authorized_keys":"k"}}`, false},
		"invalid json":      {`{not json`, false},
	} {
		if _, ok := ParseAntigravityAuth(tc.v); ok != tc.want {
			t.Errorf("%s: ParseAntigravityAuth = %v, want %v", name, ok, tc.want)
		}
	}
}
