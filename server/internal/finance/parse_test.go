package finance

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
)

func date(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestParseCents(t *testing.T) {
	for in, want := range map[string]int64{"12.34": 1234, "-12.34": -1234, "1,234.5": 123450, "(12.00)": -1200, "$5": 500, "-$5.10": -510,
		"12.00-": -1200, "+3.1": 310, ".99": 99, "1.005": 101, "-1.004": -100} {
		got, err := ParseCents(in)
		if err != nil || got != want {
			t.Errorf("ParseCents(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "abc", "1.2.3"} {
		if _, err := ParseCents(in); err == nil {
			t.Errorf("ParseCents(%q) accepted", in)
		}
	}
}

func TestMerchantAndRulePattern(t *testing.T) {
	for desc, want := range map[string][2]string{
		"TRIMET TICKET 4471":           {"TRIMET TICKET", "TRIMET TICKET"},
		"SQ *POWELLS BOOKS":            {"SQ POWELLS BOOKS", "SQ POWELLS"},
		"POS DEBIT NEW SEASONS MKT #7": {"NEW SEASONS MKT", "NEW SEASONS"},
		"Netflix.com 866-579-7172 CA":  {"NETFLIX COM CA", "NETFLIX COM"},
		"T-Mobile":                     {"T MOBILE", "T MOBILE"},
		"12345":                        {"", ""},
	} {
		if m := Merchant(desc); m != want[0] || RulePattern(m) != want[1] {
			t.Errorf("%q: merchant %q pattern %q, want %v", desc, m, RulePattern(m), want)
		}
	}
	if !ruleMatches("NEW SEASONS", "NEW SEASONS MKT") || ruleMatches("NEW SEASONS", "NEW SEASONSX") || !ruleMatches("UBER", "UBER") {
		t.Error("rules match whole-word prefixes only")
	}
}

func TestDefaultCategoryAndSuggestions(t *testing.T) {
	for _, c := range []struct {
		merchant string
		cents    int64
		want     string
	}{
		{"PAYROLL ACME INC", 680000, Income},
		{"PAYROLL ACME INC", -100, ""}, // income words only categorize money in
		{"NETFLIX COM CA", -1549, "Subscriptions"},
		{"CHASE CREDIT CRD AUTOPAY", -120000, ""}, // only a match or the person makes a transfer
		{"PAYMENT THANK YOU", 120000, ""},
		{"UBER TRIP", -1800, "Transport"},
		{"UBER EATS", -2400, "Dining"},   // the more specific list comes first
		{"SUBURBAN HARDWARE", -2400, ""}, // UBER isn't a whole word here
	} {
		got, _ := DefaultCategory(c.merchant, c.cents)
		if got != c.want {
			t.Errorf("DefaultCategory(%q, %d) = %q, want %q", c.merchant, c.cents, got, c.want)
		}
	}
	got := Suggest("SQ POWELLS BOOKS", -3418, map[string][]string{"SQ": {"Dining", "Shopping"}})
	if !slices.Equal(got, []string{"Dining", "Shopping", "Other"}) {
		t.Errorf("learned first: %v", got)
	}
	if got := Suggest("VENMO CASHOUT", 4000, nil); !slices.Equal(got, []string{Income, Transfer, "Other"}) {
		t.Errorf("money in: %v", got)
	}
	if got := Suggest("TRIMET TICKET", -250, nil); got[0] != "Transport" || len(got) != 3 {
		t.Errorf("default first: %v", got)
	}
	if got := Suggest("CHASE CREDIT CRD AUTOPAY", -120000, nil); !slices.Equal(got, []string{Transfer, "Shopping", "Dining"}) {
		t.Errorf("transfer-like: %v", got)
	}
}

const sgmlOFX = `OFXHEADER:100
DATA:OFXSGML
VERSION:102

<OFX>
<SIGNONMSGSRSV1><SONRS><STATUS><CODE>0<SEVERITY>INFO</STATUS><DTSERVER>20261109120000</SONRS></SIGNONMSGSRSV1>
<BANKMSGSRSV1><STMTTRNRS><STMTRS><CURDEF>USD
<BANKACCTFROM><BANKID>123456789<ACCTID>000123454417<ACCTTYPE>CHECKING</BANKACCTFROM>
<BANKTRANLIST><DTSTART>20261001<DTEND>20261109
<STMTTRN><TRNTYPE>DEBIT<DTPOSTED>20261106120000[-8:PST]<TRNAMT>-2.50<FITID>2026110601<NAME>TRIMET TICKET 4471</STMTTRN>
<STMTTRN><TRNTYPE>CREDIT<DTPOSTED>20261031<TRNAMT>6800.00<FITID>2026103101<NAME>PAYROLL<MEMO>ACME INC</STMTTRN>
</BANKTRANLIST>
<LEDGERBAL><BALAMT>12450.17<DTASOF>20261109120000</LEDGERBAL>
</STMTRS></STMTTRNRS></BANKMSGSRSV1>
</OFX>`

func TestParseOFX(t *testing.T) {
	p, err := ParseOFX([]byte(sgmlOFX))
	if err != nil {
		t.Fatal(err)
	}
	if p.AccountID != "000123454417" || p.AccountType != "checking" || p.Credit || p.Balance == nil || p.Balance.Cents != 1245017 || !p.Balance.AsOf.Equal(date("2026-11-09")) {
		t.Fatalf("statement %+v balance %+v", p, p.Balance)
	}
	if len(p.Txns) != 2 {
		t.Fatalf("txns %+v", p.Txns)
	}
	if tx := p.Txns[0]; tx.AmountCents != -250 || !tx.PostedOn.Equal(date("2026-11-06")) || tx.Description != "TRIMET TICKET 4471" || tx.DedupeKey != "fitid:2026110601" {
		t.Errorf("first %+v", tx)
	}
	// A short NAME gets its MEMO.
	if tx := p.Txns[1]; tx.Description != "PAYROLL ACME INC" || tx.AmountCents != 680000 {
		t.Errorf("second %+v", tx)
	}

	xml := `<?xml version="1.0"?><?OFX OFXHEADER="200" VERSION="220"?><OFX><CREDITCARDMSGSRSV1><CCSTMTTRNRS><CCSTMTRS>
<CCACCTFROM><ACCTID>4111000000000921</ACCTID></CCACCTFROM><BANKTRANLIST>
<STMTTRN><TRNTYPE>DEBIT</TRNTYPE><DTPOSTED>20261105</DTPOSTED><TRNAMT>-34.18</TRNAMT><FITID>a1</FITID><NAME>SQ *POWELLS BOOKS</NAME></STMTTRN>
</BANKTRANLIST></CCSTMTRS></CCSTMTTRNRS></CREDITCARDMSGSRSV1></OFX>`
	p, err = ParseOFX([]byte(xml))
	if err != nil || !p.Credit || p.AccountID != "4111000000000921" || len(p.Txns) != 1 || p.Txns[0].AmountCents != -3418 || p.Balance != nil {
		t.Fatalf("xml: %+v %v", p, err)
	}
	if !LooksLikeOFX("x.qfx", nil) || !LooksLikeOFX("export", []byte(xml)) || LooksLikeOFX("export.csv", []byte("Date,Amount")) {
		t.Error("LooksLikeOFX")
	}

	two := strings.Replace(sgmlOFX, "</STMTRS>", "<BANKACCTFROM><ACCTID>999</BANKACCTFROM></STMTRS>", 1)
	var ve *apperr.ValidationError
	if _, err := ParseOFX([]byte(two)); !errors.As(err, &ve) || !strings.Contains(ve.Msg, "one account per file") {
		t.Errorf("two accounts: %v", err)
	}
	if _, err := ParseOFX([]byte("<html>nope</html>")); !errors.As(err, &ve) {
		t.Errorf("not OFX: %v", err)
	}
}

func TestParseCSV(t *testing.T) {
	// Chase checking: signed amounts, a running balance, newest first.
	chase := "Details,Posting Date,Description,Amount,Type,Balance,Check or Slip #\n" +
		"DEBIT,11/06/2026,\"TRIMET TICKET 4471\",-2.50,DEBIT_CARD,12450.17,,\n" +
		"DEBIT,11/06/2026,\"TRIMET TICKET 4471\",-2.50,DEBIT_CARD,12452.67,,\n" +
		"CREDIT,10/31/2026,\"PAYROLL ACME\",\"6,800.00\",ACH_CREDIT,12455.17,,\n"
	p, err := ParseCSV([]byte("\xef\xbb\xbf"+chase), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Txns) != 3 || p.Txns[2].AmountCents != 680000 || !p.Txns[0].PostedOn.Equal(date("2026-11-06")) {
		t.Fatalf("txns %+v", p.Txns)
	}
	// Two identical rows are two transactions, each with its own stable key.
	if p.Txns[0].DedupeKey == p.Txns[1].DedupeKey {
		t.Error("identical rows share a key")
	}
	again, _ := ParseCSV([]byte(chase), false)
	if again.Txns[1].DedupeKey != p.Txns[1].DedupeKey {
		t.Error("keys change between imports")
	}
	if p.Balance == nil || p.Balance.Cents != 1245017 || !p.Balance.AsOf.Equal(date("2026-11-06")) {
		t.Errorf("balance %+v", p.Balance)
	}

	// Debit and credit columns, account details above the header, a card
	// that shows purchases as positive.
	split := "Account,Visa 0921\n\nDate,Description,Debit,Credit\n2026-11-05,SQ *POWELLS BOOKS,34.18,\n2026-11-04,PAYMENT THANK YOU,,1200.00\n,,,\n"
	p, err = ParseCSV([]byte(split), false)
	if err != nil || len(p.Txns) != 2 || p.Txns[0].AmountCents != -3418 || p.Txns[1].AmountCents != 120000 || p.Balance != nil {
		t.Fatalf("split: %+v %v", p.Txns, err)
	}
	positive := "Trans. Date,Description,Amount\n11/05/26,SQ *POWELLS BOOKS,34.18\n11/04/26,PAYMENT THANK YOU,-1200.00\n11/03/26,AUTH HOLD,0.00\n"
	p, err = ParseCSV([]byte(positive), true)
	if err != nil || len(p.Txns) != 2 || p.Txns[0].AmountCents != -3418 || p.Txns[1].AmountCents != 120000 {
		t.Fatalf("flipped: %+v %v", p.Txns, err)
	}

	var ve *apperr.ValidationError
	if _, err := ParseCSV([]byte("Foo,Bar\n1,2\n"), false); !errors.As(err, &ve) || !strings.Contains(ve.Msg, "no header row") {
		t.Errorf("no header: %v", err)
	}
	// A mapping must read every row: the first one it can't is the error.
	m, ok := DetectCSV([]byte("Date,Description,Amount\n11/01/2026,X,1\n31/31/2026,Y,2\n"))
	if ok {
		t.Fatalf("a column with an impossible date isn't a date column: %+v", m)
	}
	amount := 2
	m = CSVMapping{Delimiter: ",", HeaderRow: 1, Date: 0, DateFormat: "MM/DD/YYYY", Description: []int{1}, Amount: &amount}
	m.Amount = ptr(2)
	if _, err := ParseCSVWith([]byte("Date,Description,Amount\n11/01/2026,X,1\n31/31/2026,Y,2\n"), m); !errors.As(err, &ve) || !strings.Contains(ve.Msg, "row 3") {
		t.Errorf("bad date: %v", err)
	}
}

func TestDetectRecurring(t *testing.T) {
	today := date("2026-11-10")
	var in []Outflow
	add := func(merchant, desc, on string, cents int64) {
		in = append(in, Outflow{Merchant: merchant, Description: desc, PostedOn: date(on), AmountCents: cents})
	}
	for _, m := range []string{"2026-07-12", "2026-08-12", "2026-09-12"} {
		add("NETFLIX COM", "NETFLIX.COM", m, -1549)
	}
	add("NETFLIX COM", "NETFLIX.COM", "2026-10-12", -1749) // price went up
	add("NETFLIX COM", "NETFLIX.COM", "2026-11-09", -1749)
	add("ADOBE CREATIVE", "ADOBE CREATIVE CLD", "2026-09-18", -2299) // new: two charges
	add("ADOBE CREATIVE", "ADOBE CREATIVE CLD", "2026-10-18", -2299)
	add("NEW SEASONS", "NEW SEASONS", "2026-10-01", -6240) // weekly, not monthly
	add("NEW SEASONS", "NEW SEASONS", "2026-10-08", -5110)
	add("NEW SEASONS", "NEW SEASONS", "2026-10-15", -7020)
	add("OLD GYM", "OLD GYM", "2026-06-03", -4900) // stopped
	add("OLD GYM", "OLD GYM", "2026-07-03", -4900)
	add("POWER CO", "POWER CO", "2026-08-18", -8120) // a bill that varies: recurring, but no price-change news
	add("POWER CO", "POWER CO", "2026-09-18", -7390)
	add("POWER CO", "POWER CO", "2026-10-18", -6603)
	add("BIG TICKET", "BIG TICKET", "2026-10-01", -1000) // too different to be one price
	add("BIG TICKET", "BIG TICKET", "2026-10-31", -9000)
	slices.SortStableFunc(in, func(a, b Outflow) int { return a.PostedOn.Compare(b.PostedOn) })

	got := DetectRecurring(in, today)
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	power, adobe, netflix := got[0], got[1], got[2] // largest first
	if power.PreviousCents != nil || power.Count != 3 {
		t.Errorf("power %+v", power)
	}
	if adobe.Label != "Adobe Creative" || adobe.AmountCents != 2299 || !adobe.New || adobe.PreviousCents != nil || adobe.DayOfMonth != 18 {
		t.Errorf("adobe %+v", adobe)
	}
	if netflix.AmountCents != 1749 || netflix.PreviousCents == nil || *netflix.PreviousCents != 1549 || !netflix.ChangedOn.Equal(date("2026-10-12")) || netflix.Count != 5 || netflix.New {
		t.Errorf("netflix %+v prev %v", netflix, netflix.PreviousCents)
	}
}
