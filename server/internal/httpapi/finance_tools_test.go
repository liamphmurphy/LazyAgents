package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// The finance_* tools work in the person's Finance space and nowhere else.
func TestFinanceToolsOnlyInFinanceSpace(t *testing.T) {
	e := newChatEnv(t)
	pinFinanceToday(t, time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC))

	space := e.want("GET", "/finance", nil, http.StatusOK)["space_id"].(string)
	if code, out := e.importStatement("checking.ofx", financeOFX, nil); code != http.StatusCreated {
		t.Fatalf("import = %d %v", code, out)
	}

	elsewhere := e.canvasToken(uuid.MustParse(e.newChat(nil)["id"].(string)))
	if code, out := e.tool(elsewhere, "finance_summary", map[string]any{}); code != http.StatusConflict || !strings.Contains(out["message"].(string), "Finance space") {
		t.Fatalf("outside the space = %d %v", code, out)
	}
	other := e.want("POST", "/chat-spaces", map[string]any{"name": "Money talk"}, http.StatusCreated)["id"].(string)
	inOther := e.canvasToken(uuid.MustParse(e.want("POST", "/chats", map[string]any{"space_id": other}, http.StatusCreated)["id"].(string)))
	if code, _ := e.tool(inOther, "finance_query", map[string]any{}); code != http.StatusConflict {
		t.Fatalf("another space = %d", code)
	}

	token := e.canvasToken(uuid.MustParse(e.want("POST", "/chats", map[string]any{"space_id": space}, http.StatusCreated)["id"].(string)))
	code, sum := e.tool(token, "finance_summary", map[string]any{"months": 2})
	if code != http.StatusOK {
		t.Fatalf("summary = %d %v", code, sum)
	}
	months := sum["months"].([]any)
	oct := months[0].(map[string]any)
	if len(months) != 2 || oct["month"] != "2026-10" || oct["in"].(float64) != 6800 || oct["out"].(float64) != 62.4 || months[1].(map[string]any)["to_date"] != true {
		t.Fatalf("months %v", months)
	}
	if sum["net_worth"].(float64) != 9000 || sum["uncategorized_transactions"].(float64) != 1 || sum["spending_by_category"].(map[string]any)["month"] != "2026-10" {
		t.Fatalf("summary %v", sum)
	}
	e.toolInvalid(token, "finance_summary", map[string]any{"month": "October"}, "month")

	code, q := e.tool(token, "finance_query", map[string]any{"month": "2026-11", "direction": "out"})
	if code != http.StatusOK || q["count"].(float64) != 2 || q["total"].(float64) != -75.99 {
		t.Fatalf("query = %d %v", code, q)
	}
	e.toolInvalid(token, "finance_query", map[string]any{"month": "2026-11", "from": "2026-11-01"}, "month")
	var parking string
	for _, r := range q["transactions"].([]any) {
		if m := r.(map[string]any); strings.Contains(m["description"].(string), "PARKING") {
			parking = m["id"].(string)
		}
	}
	code, cat := e.tool(token, "finance_categorize", map[string]any{"transaction_id": parking, "category": "travel"})
	if code != http.StatusOK || cat["category"] != "Travel" || cat["rule"] != "merchants starting PDX AIRPORT" {
		t.Fatalf("categorize = %d %v", code, cat)
	}
	if code, rec := e.tool(token, "finance_recurring", map[string]any{}); code != http.StatusOK || len(rec["recurring"].([]any)) != 0 {
		t.Fatalf("recurring = %d %v", code, rec)
	}
}

func (e *chatEnv) toolInvalid(token, name string, args any, field string) {
	e.t.Helper()
	code, out := e.tool(token, name, args)
	if msg, _ := out["message"].(string); code != http.StatusUnprocessableEntity || !strings.HasPrefix(msg, field+":") {
		e.t.Fatalf("%s(%v) = %d %v, want 422 on %s", name, args, code, out, field)
	}
}

// The Finance app keeps its Ask chats in a space of its own, but that space is
// the app's plumbing: it stays out of the person's Spaces list (and so out of
// the menu, the pickers and the space limit), while the app can still read it.
func TestFinanceSpaceStaysOutOfSpacesList(t *testing.T) {
	e := newChatEnv(t)
	mine := e.want("POST", "/chat-spaces", map[string]any{"name": "Health"}, http.StatusCreated)["id"].(string)
	finance := e.want("GET", "/finance", nil, http.StatusOK)["space_id"].(string)

	var listed []string
	for _, sp := range e.want("GET", "/chat-spaces", nil, http.StatusOK)["spaces"].([]any) {
		listed = append(listed, sp.(map[string]any)["id"].(string))
	}
	if len(listed) != 1 || listed[0] != mine {
		t.Fatalf("listed %v, want only %s (not the Finance space %s)", listed, mine, finance)
	}
	if got := e.want("GET", "/chat-spaces/"+finance, nil, http.StatusOK)["id"]; got != finance {
		t.Fatalf("Finance space by id = %v", got)
	}
	e.want("GET", "/chats?space="+finance, nil, http.StatusOK)
}
