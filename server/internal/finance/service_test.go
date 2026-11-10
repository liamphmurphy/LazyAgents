package finance

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
)

func newService(t *testing.T) (*Service, Owner) {
	t.Helper()
	pool := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, seed.Options{UserName: "Liam", UserEmail: "liam@example.com"}); err != nil {
		t.Fatal(err)
	}
	me, err := store.New(pool).GetLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return &Service{Pool: pool}, Owner{Workspace: me.WorkspaceID, User: me.ID}
}

const checkingOFX = `<OFX><BANKMSGSRSV1><STMTTRNRS><STMTRS>
<BANKACCTFROM><ACCTID>000123454417<ACCTTYPE>CHECKING</BANKACCTFROM><BANKTRANLIST>
<STMTTRN><DTPOSTED>20261031<TRNAMT>6800.00<FITID>c1<NAME>PAYROLL ACME INC</STMTTRN>
<STMTTRN><DTPOSTED>20261102<TRNAMT>-1200.00<FITID>c2<NAME>CHASE CREDIT CRD AUTOPAY</STMTTRN>
<STMTTRN><DTPOSTED>20261106<TRNAMT>-2.50<FITID>c3<NAME>ZIPPY CARTS 4471</STMTTRN>
<STMTTRN><DTPOSTED>20261107<TRNAMT>-40.00<FITID>c4<NAME>VENMO PAYMENT 1023</STMTTRN>
</BANKTRANLIST><LEDGERBAL><BALAMT>10000.00<DTASOF>20261109</LEDGERBAL></STMTRS></STMTTRNRS></BANKMSGSRSV1></OFX>`

const cardCSV = "Transaction Date,Description,Amount\n" +
	"11/05/2026,SQ *POWELLS BOOKS,-34.18\n" +
	"11/04/2026,PAYMENT THANK YOU,1200.00\n" +
	"10/22/2026,SQ *POWELLS BOOKS,-12.00\n" +
	"10/12/2026,NETFLIX.COM,-17.49\n" +
	"09/12/2026,NETFLIX.COM,-15.49\n" +
	"08/12/2026,NETFLIX.COM,-15.49\n"

// The import path end to end: OFX makes its own account, CSV goes into a
// chosen one, re-imports add nothing, card payments pair up as transfers,
// a person's pick becomes a rule for past and future matches, and the
// overview adds it all up.
func TestImportCategorizeOverview(t *testing.T) {
	s, o := newService(t)
	ctx := context.Background()

	var ve *apperr.ValidationError
	if _, err := s.Import(ctx, o, ImportInput{Filename: "card.csv", Data: []byte(cardCSV)}); !errors.As(err, &ve) || ve.Field != "account_id" {
		t.Fatalf("CSV without an account: %v", err)
	}

	chk, err := s.Import(ctx, o, ImportInput{Filename: "chase/checking.ofx", Data: []byte(checkingOFX)})
	if err != nil {
		t.Fatal(err)
	}
	if !chk.AccountCreated || chk.Account.Name != "Checking ••4417" || chk.Account.Kind != "checking" || chk.Import.Filename != "checking.ofx" ||
		chk.Import.Inserted != 4 || chk.Import.Transfers != 0 || chk.NeedsReview != 3 {
		t.Fatalf("checking import %+v", chk)
	}
	// Again: the same account, nothing new.
	again, err := s.Import(ctx, o, ImportInput{Filename: "checking.qfx", Data: []byte(checkingOFX)})
	if err != nil || again.AccountCreated || again.Account.ID != chk.Account.ID || again.Import.Inserted != 0 || again.Import.Duplicates != 4 {
		t.Fatalf("re-import %+v %v", again, err)
	}

	card, err := s.CreateAccount(ctx, o, NewAccount{Name: " Visa  card ", Kind: "Credit", Mask: "0921"})
	if err != nil || card.Name != "Visa card" || card.Kind != "credit" {
		t.Fatalf("account %+v %v", card, err)
	}
	if _, err := s.CreateAccount(ctx, o, NewAccount{Name: "Visa card", Kind: "credit"}); !errors.As(err, &ve) || ve.Field != "name" {
		t.Fatalf("duplicate name: %v", err)
	}
	if _, err := s.CreateAccount(ctx, o, NewAccount{Name: "X", Kind: "piggybank"}); !errors.As(err, &ve) || ve.Field != "kind" {
		t.Fatalf("bad kind: %v", err)
	}
	cc, err := s.Import(ctx, o, ImportInput{Filename: "card.csv", Data: []byte(cardCSV), Account: &card.ID})
	if err != nil {
		t.Fatal(err)
	}
	// The $1,200 card payment pairs with checking's autopay.
	if cc.Import.Inserted != 6 || cc.Import.Transfers != 1 || cc.NeedsReview != 2 {
		t.Fatalf("card import %+v", cc.Import)
	}
	// An OFX names its account: putting it in another one is refused.
	if _, err := s.Import(ctx, o, ImportInput{Filename: "x.ofx", Data: []byte(checkingOFX), Account: &card.ID}); !errors.As(err, &ve) ||
		ve.Field != "account_id" || !strings.Contains(ve.Msg, "Checking ••4417, not Visa card") {
		t.Fatalf("OFX into the wrong account: %v", err)
	}

	review, err := s.review(ctx, o, 10)
	if err != nil {
		t.Fatal(err)
	}
	var powells, venmo ReviewItem
	for _, r := range review {
		switch r.Merchant {
		case "SQ POWELLS BOOKS":
			if r.AmountCents == -3418 {
				powells = r
			}
		case "VENMO PAYMENT":
			venmo = r
		}
	}
	if powells.ID == uuid.Nil || venmo.ID == uuid.Nil || len(review) != 4 {
		t.Fatalf("review %+v", review)
	}

	// A pick with a rule recategorizes the older Powell's charge too.
	res, err := s.Categorize(ctx, o, powells.ID, "shopping", true)
	if err != nil || res.Pattern != "SQ POWELLS" || res.Updated != 1 || *res.Transaction.Category != "Shopping" {
		t.Fatalf("categorize %+v %v", res, err)
	}
	if _, err := s.Categorize(ctx, o, venmo.ID, "Transfer", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Categorize(ctx, o, venmo.ID, "Snacks", false); !errors.As(err, &ve) || ve.Field != "category" {
		t.Fatalf("bad category: %v", err)
	}
	if _, err := s.Categorize(ctx, o, uuid.New(), "Other", false); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("someone else's: %v", err)
	}
	// A new statement with the same merchant follows the rule.
	more := "Transaction Date,Description,Amount\n11/08/2026,SQ *POWELLS BOOKS #2,-8.00\n"
	m, err := s.Import(ctx, o, ImportInput{Filename: "more.csv", Data: []byte(more), Account: &card.ID})
	if err != nil || m.Import.Categorized != 1 || m.NeedsReview != 0 {
		t.Fatalf("rule on import %+v %v", m.Import, err)
	}

	if err := s.SetBalance(ctx, o, card.ID, -45467, date("2026-11-08")); err != nil {
		t.Fatal(err)
	}
	if err := s.SetBalance(ctx, o, uuid.New(), 1, date("2026-11-08")); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("unknown account balance: %v", err)
	}

	ov, err := s.Overview(ctx, o, time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC), 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(ov.Months) != 12 || !ov.Months[11].Month.Equal(date("2026-11-01")) || !ov.Months[0].Month.Equal(date("2025-12-01")) {
		t.Fatalf("months %v … %v", ov.Months[0].Month, ov.Months[len(ov.Months)-1].Month)
	}
	oct, nov := ov.Months[10], ov.Months[11]
	if oct.InflowCents != 680000 || oct.OutflowCents != 1200+1749 || oct.TransferOutCents != 0 {
		t.Errorf("october %+v", oct)
	}
	// November: the autopay is a transfer, so out of spending; Venmo too.
	if nov.InflowCents != 0 || nov.OutflowCents != 3418+250+800 || nov.TransferOutCents != 120000+4000 {
		t.Errorf("november %+v", nov)
	}
	// Net worth: checking 10,000.00 on Nov 9 and the card −454.67 on Nov 8.
	if ov.NetWorth == nil || *ov.NetWorth != 1000000-45467 {
		t.Errorf("net worth %v", ov.NetWorth)
	}
	// End of October: undo checking's November activity (−1,242.50) and the
	// card's (+1,200 − 34.18 − 8.00).
	if got := ov.Months[10].NetWorthCents; got == nil || *got != (1000000+124250)+(-45467-120000+3418+800) {
		t.Errorf("october net worth %v", got)
	}
	if ov.SavingsRate == nil || *ov.SavingsRate < 0.98 {
		t.Errorf("savings rate %v", ov.SavingsRate)
	}
	if !ov.CategoryMonth.Equal(date("2026-10-01")) || len(ov.Categories) != 2 || ov.Categories[0].Category != "Subscriptions" ||
		ov.Categories[0].SpentCents != 1749 || ov.Categories[0].AverageCents != 1549 {
		t.Errorf("categories %s %+v", ov.CategoryMonth, ov.Categories)
	}
	if ov.ReviewCount != 1 || len(ov.Review) != 1 || ov.Review[0].Merchant != "ZIPPY CARTS" || ov.Review[0].Suggestions[0] != "Shopping" {
		t.Errorf("review %d %+v", ov.ReviewCount, ov.Review)
	}
	if ov.LastImport == nil || ov.LastImport.Filename != "more.csv" || len(ov.Accounts) != 2 || ov.Accounts[1].Balance == nil {
		t.Errorf("last import %+v accounts %+v", ov.LastImport, ov.Accounts)
	}
	if len(ov.Recurring) != 1 || ov.Recurring[0].Label != "Netflix Com" || ov.Recurring[0].PreviousCents == nil {
		t.Errorf("recurring %+v", ov.Recurring)
	}

	// Breaking up a matched transfer sends its other side back to review.
	var autopay store.FinanceTransaction
	if err := s.Pool.QueryRow(ctx, `SELECT id FROM finance_transactions WHERE merchant = 'CHASE CREDIT CRD AUTOPAY'`).Scan(&autopay.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Categorize(ctx, o, autopay.ID, "Other", false); err != nil {
		t.Fatal(err)
	}
	q, err := s.Query(ctx, o, QueryInput{Uncategorized: true})
	if err != nil || q.Count != 2 || q.SumCents != 120000-250 {
		t.Fatalf("after unlinking: %+v %v", q, err)
	}

	// Query filters and escaping.
	q, err = s.Query(ctx, o, QueryInput{Search: "powell", Direction: "out", Limit: 1})
	if err != nil || q.Count != 3 || len(q.Rows) != 1 || q.SumCents != -(3418+1200+800) {
		t.Fatalf("search %+v %v", q, err)
	}
	if q, err = s.Query(ctx, o, QueryInput{Search: "100%"}); err != nil || q.Count != 0 {
		t.Fatalf("escaped search %+v %v", q, err)
	}
	from, to := date("2026-10-01"), date("2026-10-31")
	if q, err = s.Query(ctx, o, QueryInput{From: &from, To: &to, Category: "subscriptions"}); err != nil || q.Count != 1 {
		t.Fatalf("dated category %+v %v", q, err)
	}
	if _, err = s.Query(ctx, o, QueryInput{Category: "Snacks"}); !errors.As(err, &ve) {
		t.Fatalf("bad category: %v", err)
	}
	if !strings.HasPrefix(ov.Accounts[0].Name, "Checking") {
		t.Errorf("accounts in the order added: %+v", ov.Accounts)
	}
}

// The Finance space is made once per person and then found again.
func TestEnsureSpace(t *testing.T) {
	s, o := newService(t)
	ctx := context.Background()
	a, err := s.EnsureSpace(ctx, o)
	if err != nil || !IsFinanceSpace(a) || a.Name != "Finance" || a.Instructions != SpaceInstructions {
		t.Fatalf("space %+v %v", a, err)
	}
	b, err := s.EnsureSpace(ctx, o)
	if err != nil || b.ID != a.ID {
		t.Fatalf("second %+v %v", b, err)
	}
	if _, err := s.Pool.Exec(ctx, `DELETE FROM chat_spaces WHERE id = $1`, a.ID); err != nil {
		t.Fatal(err)
	}
	c, err := s.EnsureSpace(ctx, o)
	if err != nil || c.ID == a.ID {
		t.Fatalf("after delete %+v %v", c, err)
	}
}

const oddCSV = "Statement for account ****0921\n\nTxn Dt|Payee/Memo|Ref|Value|DR/CR\n" +
	"20261105|SQ *POWELLS BOOKS|A1|34.18|D\n20261104|PAYMENT RECEIVED|A2|1200.00|C\n"

// A layout Vesper doesn't know waits as a pending import; a mapping that
// misreads a row is refused with the row; the confirmed one imports and is
// saved, so the bank's next export imports straight away.
func TestPendingCSVMapping(t *testing.T) {
	s, o := newService(t)
	ctx := context.Background()
	card, err := s.CreateAccount(ctx, o, NewAccount{Name: "Visa", Kind: "credit"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Import(ctx, o, ImportInput{Filename: "odd.csv", Data: []byte(oddCSV), Account: &card.ID})
	if err != nil || res.Pending == nil || res.Import.ID != uuid.Nil {
		t.Fatalf("unknown layout: %+v %v", res, err)
	}
	p := res.Pending
	if p.Prefill.Delimiter != "|" || p.Prefill.HeaderRow != 2 || len(p.Sample.Rows) != 4 || p.AccountID != card.ID {
		t.Fatalf("pending %+v", p)
	}
	prompt := MappingPrompt(*p, "credit")
	for _, want := range []string{p.ID.String(), "finance_map_csv", `3: ["20261105","SQ *POWELLS BOOKS","A1","34.18","D"]`, "Txn Dt|Payee/Memo"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, prompt)
		}
	}

	var ve *apperr.ValidationError
	wrong := p.Prefill
	wrong.DateFormat = "MM/DD/YYYY"
	if _, err := s.ProposeMapping(ctx, o, p.ID, wrong, ""); !errors.As(err, &ve) || !strings.Contains(ve.Msg, "row 3") {
		t.Fatalf("misread mapping: %v", err)
	}
	right := p.Prefill
	right.Direction, right.DebitValues = ptr(4), []string{"D"}
	pv, err := s.ProposeMapping(ctx, o, p.ID, right, "  Pipe-separated;   D/C marks direction. ")
	if err != nil || pv.Count != 2 || pv.OutCents != 3418 || pv.InCents != 120000 {
		t.Fatalf("proposed %+v %v", pv, err)
	}
	got, err := s.GetPending(ctx, o, p.ID)
	if err != nil || got.Mapping == nil || got.MappingNote != "Pipe-separated; D/C marks direction." {
		t.Fatalf("stored %+v %v", got, err)
	}
	if _, err := s.GetPending(ctx, Owner{Workspace: o.Workspace, User: uuid.New()}, p.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("someone else's pending import: %v", err)
	}

	done, err := s.CommitPending(ctx, o, p.ID, *got.Mapping)
	if err != nil || done.Import.Inserted != 2 || done.Account.ID != card.ID {
		t.Fatalf("commit %+v %v", done, err)
	}
	if _, err := s.GetPending(ctx, o, p.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("pending import kept after commit: %v", err)
	}
	// The next export from that bank: same layout, new rows.
	next := oddCSV + "20261108|NEW SEASONS MKT|A3|62.40|D\n"
	again, err := s.Import(ctx, o, ImportInput{Filename: "odd-2.csv", Data: []byte(next), Account: &card.ID})
	if err != nil || again.Pending != nil || again.Import.Inserted != 1 || again.Import.Duplicates != 2 {
		t.Fatalf("saved mapping: %+v %v", again, err)
	}

	res, err = s.Import(ctx, o, ImportInput{Filename: "x.csv", Data: []byte("a;b\nc;d\n"), Account: &card.ID})
	if err != nil || res.Pending == nil {
		t.Fatalf("unreadable: %+v %v", res, err)
	}
	if err := s.CancelPending(ctx, o, res.Pending.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.CancelPending(ctx, o, res.Pending.ID); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("cancel twice: %v", err)
	}
}
