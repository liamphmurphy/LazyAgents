package httpapi

import (
	"net/http"
	"testing"
)

// A coding chat's commit address is one of its owner's, picked when it starts
// or later (even mid-turn, without a revision), and shown only while it is
// still theirs.
func TestChatCommitEmail(t *testing.T) {
	e := newChatEnv(t)
	e.want("PATCH", "/me", map[string]any{"emails": []string{e.me.Email, "Maya@Personal.example"}}, 200)
	dev := e.want("POST", "/personas", map[string]any{
		"name": "Dev", "capabilities": map[string]any{"code": true, "images": false}, "repos": []string{"https://github.com/acme/app"},
	}, http.StatusCreated)
	devID := dev["id"].(string)

	chat := e.want("POST", "/chats", map[string]any{"persona_id": devID, "commit_email": "maya@personal.example"}, http.StatusCreated)
	if chat["commit_email"] != "Maya@Personal.example" {
		t.Fatalf("picked at start = %v", chat["commit_email"])
	}
	id := chat["id"].(string)
	// Mark the chat running: picking still works, and needs no revision.
	if _, err := e.pool.Exec(t.Context(), `UPDATE chat_sessions SET state='running', revision=revision+5 WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if got := e.want("PUT", "/chats/"+id+"/commit-email", map[string]any{"email": e.me.Email}, 200); got["commit_email"] != e.me.Email {
		t.Fatalf("picked later = %v", got["commit_email"])
	}
	e.invalid("PUT", "/chats/"+id+"/commit-email", map[string]any{"email": "someone@else.example"}, "commit_email", "add someone@else.example")

	// Dropping the address from the profile unpicks it.
	e.want("PUT", "/chats/"+id+"/commit-email", map[string]any{"email": "Maya@Personal.example"}, 200)
	e.want("PATCH", "/me", map[string]any{"emails": []string{e.me.Email}}, 200)
	if got := e.want("GET", "/chats/"+id, nil, 200); got["commit_email"] != nil {
		t.Fatalf("a removed address is still shown: %v", got["commit_email"])
	}

	// A chat without a repository makes no commits.
	plain := e.want("POST", "/chats", map[string]any{}, http.StatusCreated)
	e.invalid("PUT", "/chats/"+plain["id"].(string)+"/commit-email", map[string]any{"email": e.me.Email}, "commit_email", "repository")
	e.invalid("POST", "/chats", map[string]any{"commit_email": e.me.Email}, "commit_email", "repository")
	if got := e.want("GET", "/chats/"+plain["id"].(string), nil, 200); got["commit_email"] != nil {
		t.Fatalf("plain chat commit_email = %v", got["commit_email"])
	}
}
