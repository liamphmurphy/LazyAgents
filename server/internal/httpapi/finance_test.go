package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/finance"
)

func (e *chatEnv) importStatement(filename, data string, fields map[string]string) (int, map[string]any) {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	pw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		e.t.Fatal(err)
	}
	pw.Write([]byte(data))
	mw.Close()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/finance/imports", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	raw, _ := io.ReadAll(res.Body)
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

const financeOFX = `<OFX><BANKMSGSRSV1><STMTTRNRS><STMTRS>
<BANKACCTFROM><ACCTID>000123454417<ACCTTYPE>CHECKING</BANKACCTFROM><BANKTRANLIST>
<STMTTRN><DTPOSTED>20261015<TRNAMT>6800.00<FITID>c1<NAME>PAYROLL ACME INC</STMTTRN>
<STMTTRN><DTPOSTED>20261020<TRNAMT>-62.40<FITID>c2<NAME>NEW SEASONS MKT #7</STMTTRN>
<STMTTRN><DTPOSTED>20261106<TRNAMT>-48.00<FITID>c3<NAME>PDX AIRPORT PARKING</STMTTRN>
<STMTTRN><DTPOSTED>20261107<TRNAMT>-27.99<FITID>c4<NAME>ZIPPY CARTS 0042</STMTTRN>
</BANKTRANLIST><LEDGERBAL><BALAMT>9000.00<DTASOF>20261109</LEDGERBAL></STMTRS></STMTTRNRS></BANKMSGSRSV1></OFX>`

func pinFinanceToday(t *testing.T, today time.Time) {
	prev := finance.Now
	finance.Now = func() time.Time { return today }
	t.Cleanup(func() { finance.Now = prev })
}

// The dashboard API: opening it makes the Finance space once; imports,
// accounts, balances and categories show up in the overview.
func TestFinanceAPI(t *testing.T) {
	e := newChatEnv(t)
	pinFinanceToday(t, time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC))

	empty := e.want("GET", "/finance", nil, http.StatusOK)
	space := empty["space_id"].(string)
	if len(empty["months"].([]any)) != 12 || empty["net_worth_cents"] != nil || empty["review_count"].(float64) != 0 || len(empty["categories_all"].([]any)) != 16 {
		t.Fatalf("empty overview %v", empty)
	}
	if again := e.want("GET", "/finance?months=6", nil, http.StatusOK); again["space_id"] != space || len(again["months"].([]any)) != 6 {
		t.Fatalf("second open %v", again["space_id"])
	}
	e.invalid("GET", "/finance?months=30", nil, "months", "1–24")
	// It's an ordinary space too: chats start in it like any other.
	sp := e.want("GET", "/chat-spaces/"+space, nil, http.StatusOK)
	if sp["name"] != "Finance" || !strings.Contains(sp["instructions"].(string), "finance_summary") {
		t.Fatalf("space %v", sp)
	}

	code, out := e.importStatement("checking.ofx", financeOFX, nil)
	if code != http.StatusCreated {
		t.Fatalf("import = %d %v", code, out)
	}
	acct := out["account"].(map[string]any)
	if acct["name"] != "Checking ••4417" || acct["from_ofx"] != true || acct["balance_cents"].(float64) != 900000 || acct["transaction_count"].(float64) != 4 ||
		out["needs_review"].(float64) != 1 || out["first_date"] != "2026-10-15" || out["import"].(map[string]any)["inserted"].(float64) != 4 {
		t.Fatalf("import %v", out)
	}
	if code, out = e.importStatement("card.csv", "Date,Description,Amount\n11/01/2026,X,-1\n", nil); code != http.StatusUnprocessableEntity || out["field"] != "account_id" {
		t.Fatalf("CSV without account = %d %v", code, out)
	}
	if code, out = e.importStatement("x.csv", "nothing here", map[string]string{"account_id": "nope"}); code != http.StatusUnprocessableEntity || out["field"] != "account_id" {
		t.Fatalf("bad account id = %d %v", code, out)
	}

	e.invalid("POST", "/finance/accounts", map[string]any{"name": "Visa", "kind": "piggybank"}, "kind", "")
	visa := e.want("POST", "/finance/accounts", map[string]any{"name": "Visa", "kind": "credit", "mask": "0921"}, http.StatusCreated)
	if visa["from_ofx"] != false || visa["balance_cents"] != nil {
		t.Fatalf("account %v", visa)
	}
	code, out = e.importStatement("card.csv", "Trans. Date,Description,Amount\n11/05/26,SQ *POWELLS BOOKS,34.18\n", map[string]string{"account_id": visa["id"].(string), "flip_sign": "true"})
	if code != http.StatusCreated || out["needs_review"].(float64) != 1 {
		t.Fatalf("card import = %d %v", code, out)
	}
	bal := e.want("PUT", "/finance/accounts/"+visa["id"].(string)+"/balance", map[string]any{"balance_cents": -3418, "as_of": "2026-11-09"}, http.StatusOK)
	if bal["balance_cents"].(float64) != -3418 || bal["balance_as_of"] != "2026-11-09" {
		t.Fatalf("balance %v", bal)
	}
	e.want("PUT", "/finance/accounts/"+uuid.NewString()+"/balance", map[string]any{"balance_cents": 1, "as_of": "2026-11-09"}, http.StatusNotFound)

	ov := e.want("GET", "/finance", nil, http.StatusOK)
	review := ov["review"].([]any)
	if ov["review_count"].(float64) != 2 || len(review) != 2 || ov["net_worth_cents"].(float64) != 900000-3418 || ov["category_month"] != "2026-10-01" {
		t.Fatalf("overview %v", ov)
	}
	var powells map[string]any
	for _, r := range review {
		if m := r.(map[string]any); strings.Contains(m["description"].(string), "POWELLS") {
			powells = m
		}
	}
	if powells == nil || powells["account_name"] != "Visa" || powells["amount_cents"].(float64) != -3418 || powells["posted_on"] != "2026-11-05" {
		t.Fatalf("review %v", review)
	}
	res := e.want("PUT", "/finance/transactions/"+powells["id"].(string)+"/category", map[string]any{"category": "Shopping"}, http.StatusOK)
	if res["category"] != "Shopping" || res["pattern"] != "SQ POWELLS" || res["updated"].(float64) != 0 {
		t.Fatalf("categorize %v", res)
	}
	e.invalid("PUT", "/finance/transactions/"+powells["id"].(string)+"/category", map[string]any{"category": "Snacks"}, "category", "must be one of")
	e.want("PUT", "/finance/transactions/"+uuid.NewString()+"/category", map[string]any{"category": "Other"}, http.StatusNotFound)
	months := e.want("GET", "/finance", nil, http.StatusOK)["months"].([]any)
	if oct := months[10].(map[string]any); oct["inflow_cents"].(float64) != 680000 || oct["outflow_cents"].(float64) != 6240 || oct["month"] != "2026-10-01" {
		t.Fatalf("october %v", oct)
	}
}
