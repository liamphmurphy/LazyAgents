package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

const oddStatement = "Statement for account ****0921\n\nTxn Dt|Payee/Memo|Ref|Value|DR/CR\n" +
	"20261105|SQ *POWELLS BOOKS|A1|34.18|D\n20261104|PAYMENT RECEIVED|A2|1200.00|C\n"

// An unknown CSV layout: the upload waits as a pending import, a hidden,
// temporary chat in the Finance space asks the assistant, whose mapping is
// checked against every row; the person previews and commits it, and the
// bank's next export imports straight away.
func TestFinanceCSVMappingFlow(t *testing.T) {
	e := newChatEnv(t)
	pinFinanceToday(t, time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC))
	space := e.want("GET", "/finance", nil, http.StatusOK)["space_id"].(string)
	visa := e.want("POST", "/finance/accounts", map[string]any{"name": "Visa", "kind": "credit"}, http.StatusCreated)["id"].(string)

	code, pending := e.importStatement("odd.csv", oddStatement, map[string]string{"account_id": visa})
	if code != http.StatusAccepted || pending["assistant"] != "asking" || pending["columns"].(float64) != 5 || len(pending["rows"].([]any)) != 4 {
		t.Fatalf("upload = %d %v", code, pending)
	}
	id := pending["id"].(string)
	if p := pending["prefill"].(map[string]any); p["delimiter"] != "|" || p["header_row"].(float64) != 2 || p["date_format"] != "YYYYMMDD" {
		t.Fatalf("prefill %v", p)
	}

	// The hidden chat: in the Finance space, temporary, out of chat lists, asked to map the file.
	ctx := context.Background()
	var chatID uuid.UUID
	if err := e.pool.QueryRow(ctx, `SELECT chat_id FROM finance_pending_imports WHERE id = $1`, id).Scan(&chatID); err != nil {
		t.Fatal(err)
	}
	chat, err := e.q.GetChatSession(ctx, store.GetChatSessionParams{ID: chatID, WorkspaceID: e.me.WorkspaceID})
	if err != nil || !chat.Hidden || !chat.Temporary || chat.SpaceID == nil || chat.SpaceID.String() != space || chat.CurrentRunID == nil {
		t.Fatalf("chat %+v %v", chat, err)
	}
	var prompt string
	if err := e.pool.QueryRow(ctx, `SELECT body_md FROM chat_messages WHERE session_id = $1 ORDER BY seq LIMIT 1`, chatID).Scan(&prompt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "finance_map_csv") || !strings.Contains(prompt, id) || !strings.Contains(prompt, "credit account") {
		t.Fatalf("prompt %s", prompt)
	}
	for _, c := range ids(e.want("GET", "/chats?persona_id=all&space=all", nil, http.StatusOK)["chats"], "id") {
		if c == chatID.String() {
			t.Fatal("the mapping chat shows in chat lists")
		}
	}

	token, _, err := agents.IssueToken(ctx, e.q, chat.AgentID, "run", "chat", chat.CurrentRunID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	mapping := map[string]any{"import_id": id, "delimiter": "|", "header_row": 2, "date": 0, "date_format": "YYYYMMDD", "description": []int{1}, "amount": 3}
	// A date format that doesn't fit is refused, naming the row, so the assistant can try again.
	code, out := e.tool(token, "finance_map_csv", map[string]any{"import_id": id, "delimiter": "|", "header_row": 2, "date": 0, "date_format": "MM/DD/YYYY", "description": []int{1}, "amount": 3})
	if code != http.StatusUnprocessableEntity || !strings.Contains(out["message"].(string), "row 3") {
		t.Fatalf("misread = %d %v", code, out)
	}
	mapping["direction"], mapping["debit_values"], mapping["note"] = 4, []string{"D"}, "Pipe-separated; D/C marks direction."
	code, out = e.tool(token, "finance_map_csv", mapping)
	if code != http.StatusOK || out["transactions"].(float64) != 2 || out["money_out"].(float64) != 34.18 {
		t.Fatalf("map = %d %v", code, out)
	}
	if code, _ := e.tool(token, "finance_map_csv", map[string]any{"import_id": uuid.NewString(), "delimiter": "|", "date_format": "YYYYMMDD", "description": []int{1}, "amount": 3}); code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown import = %d", code)
	}

	got := e.want("GET", "/finance/imports/pending/"+id, nil, http.StatusOK)
	if got["assistant"] != "mapped" || got["mapping_note"] != "Pipe-separated; D/C marks direction." {
		t.Fatalf("pending %v", got)
	}
	m := got["mapping"].(map[string]any)
	pv := e.want("POST", "/finance/imports/pending/"+id+"/preview", m, http.StatusOK)
	if pv["count"].(float64) != 2 || pv["out_cents"].(float64) != 3418 || pv["first_date"] != "2026-11-04" {
		t.Fatalf("preview %v", pv)
	}
	bad := map[string]any{"delimiter": "|", "header_row": 2, "date": 0, "date_format": "YYYYMMDD", "description": []int{1}, "amount": 9}
	e.invalid("POST", "/finance/imports/pending/"+id+"/preview", bad, "amount", "doesn't exist")

	done := e.want("POST", "/finance/imports/pending/"+id+"/commit", m, http.StatusCreated)
	if done["import"].(map[string]any)["inserted"].(float64) != 2 || done["account"].(map[string]any)["id"] != visa {
		t.Fatalf("commit %v", done)
	}
	e.want("GET", "/finance/imports/pending/"+id, nil, http.StatusNotFound)

	code, out = e.importStatement("odd-2.csv", oddStatement+"20261108|NEW SEASONS MKT|A3|62.40|D\n", map[string]string{"account_id": visa})
	if code != http.StatusCreated || out["import"].(map[string]any)["inserted"].(float64) != 1 {
		t.Fatalf("saved mapping = %d %v", code, out)
	}

	code, out = e.importStatement("x.csv", "a;b\nc;d\n", map[string]string{"account_id": visa})
	if code != http.StatusAccepted {
		t.Fatalf("second unknown = %d %v", code, out)
	}
	e.want("DELETE", "/finance/imports/pending/"+out["id"].(string), nil, http.StatusNoContent)
	e.want("DELETE", "/finance/imports/pending/"+out["id"].(string), nil, http.StatusNotFound)
}
