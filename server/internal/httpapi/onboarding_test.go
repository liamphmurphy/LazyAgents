package httpapi

import (
	"net/http"
	"testing"
)

func TestOnboardingCompleteIsRecordedOnce(t *testing.T) {
	e := newChatEnv(t)
	me := e.want("GET", "/me", nil, http.StatusOK)
	if me["onboarded_at"] != nil || me["harness_connected"] != true {
		t.Fatalf("fresh profile %v: want no onboarded_at and harness_connected (local CLI logins)", me)
	}
	first := e.want("POST", "/me/onboarding/complete", nil, http.StatusOK)["onboarded_at"]
	if first == nil {
		t.Fatal("onboarded_at not set")
	}
	// Running setup again from Settings keeps the first completion.
	if again := e.want("POST", "/me/onboarding/complete", nil, http.StatusOK)["onboarded_at"]; again != first {
		t.Fatalf("onboarded_at moved from %v to %v", first, again)
	}
	boot := e.want("GET", "/bootstrap", nil, http.StatusOK)
	if boot["me"].(map[string]any)["onboarded_at"] != first {
		t.Fatalf("bootstrap me %v", boot["me"])
	}
	// Teammates never see it.
	for _, u := range boot["users"].([]any) {
		if _, ok := u.(map[string]any)["onboarded_at"]; ok {
			t.Fatalf("teammate %v carries onboarded_at", u)
		}
	}
}
