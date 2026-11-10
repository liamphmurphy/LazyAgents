package agentapi

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/finance"
)

// The finance_* tools answer from the person's imported transactions, only
// in their Finance space: a chat elsewhere (or a shared context) never reads
// their money.

func financeFor(ctx context.Context, d Deps, c agents.Caller, tool string) (*finance.Service, finance.Owner, error) {
	var o finance.Owner
	s, err := chatScopeFor(ctx, d, c)
	if err != nil {
		return nil, o, err
	}
	notHere := apperr.Conflict("%s only works in the person's Finance space: they can open it from Finance in the main menu and use Ask", tool)
	if s == nil || c.RunID == nil {
		return nil, o, notHere
	}
	chat, err := d.Q.ChatSessionForRun(ctx, c.RunID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, o, notHere
	}
	if err != nil {
		return nil, o, err
	}
	if chat.SpaceID == nil {
		return nil, o, notHere
	}
	sp, err := d.Q.GetChatSpaceByID(ctx, *chat.SpaceID)
	if err != nil {
		return nil, o, err
	}
	if !finance.IsFinanceSpace(sp) || sp.UserID != s.UserID {
		return nil, o, notHere
	}
	return &finance.Service{Pool: d.Runs.Pool}, finance.Owner{Workspace: c.Agent.WorkspaceID, User: s.UserID}, nil
}

func dollars(cents int64) float64 { return math.Round(float64(cents)) / 100 }

func optDollars(cents *int64) *float64 {
	if cents == nil {
		return nil
	}
	v := dollars(*cents)
	return &v
}

func ym(t time.Time) string { return t.Format("2006-01") }

func parseMonth(field, s string) (time.Time, error) {
	t, err := time.Parse("2006-01", strings.TrimSpace(s))
	if err != nil {
		return t, apperr.Invalid(field, "use YYYY-MM, e.g. 2026-10")
	}
	return t, nil
}

func parseDay(field, s string) (*time.Time, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	t, err := time.Parse(time.DateOnly, strings.TrimSpace(s))
	if err != nil {
		return nil, apperr.Invalid(field, "use YYYY-MM-DD")
	}
	return &t, nil
}

type FinanceSummaryIn struct {
	Months int    `json:"months,omitempty" jsonschema:"Months of cash flow ending with this one, 1–24 (default 6)"`
	Month  string `json:"month,omitempty" jsonschema:"YYYY-MM for spending by category; default the last full month with data"`
}

func financeSummary(ctx context.Context, d Deps, c agents.Caller, in FinanceSummaryIn) (any, error) {
	svc, o, err := financeFor(ctx, d, c, "finance_summary")
	if err != nil {
		return nil, err
	}
	if in.Months == 0 {
		in.Months = 6
	}
	if in.Months < 1 || in.Months > 24 {
		return nil, apperr.Invalid("months", "1–24")
	}
	var month time.Time
	if in.Month != "" {
		if month, err = parseMonth("month", in.Month); err != nil {
			return nil, err
		}
	}
	s, err := svc.Summary(ctx, o, finance.Now(), in.Months, month)
	if err != nil {
		return nil, err
	}
	months := make([]map[string]any, len(s.Months))
	for i, m := range s.Months {
		months[i] = map[string]any{"month": ym(m.Month), "in": dollars(m.InflowCents), "out": dollars(m.OutflowCents), "net": dollars(m.InflowCents - m.OutflowCents),
			"transfers_out": dollars(m.TransferOutCents)}
		if m.NetWorthCents != nil {
			months[i]["net_worth_at_end"] = dollars(*m.NetWorthCents)
		}
	}
	months[len(months)-1]["to_date"] = true
	cats := make([]map[string]any, len(s.Categories))
	for i, cs := range s.Categories {
		cats[i] = map[string]any{"category": cs.Category, "spent": dollars(cs.SpentCents), "average_before": dollars(cs.AverageCents)}
	}
	accts := make([]map[string]any, len(s.Accounts))
	for i, a := range s.Accounts {
		accts[i] = map[string]any{"id": a.ID, "name": a.Name, "kind": a.Kind, "transactions": a.TransactionCount}
		if a.Balance != nil {
			accts[i]["balance"], accts[i]["balance_as_of"] = dollars(a.Balance.Cents), a.Balance.AsOf.Format(time.DateOnly)
		}
	}
	out := map[string]any{"months": months, "spending_by_category": map[string]any{"month": ym(s.CategoryMonth), "categories": cats},
		"uncategorized_transactions": s.Uncategorized, "accounts": accts, "net_worth": optDollars(s.NetWorth)}
	if s.SavingsRate != nil {
		out["savings_rate_percent"] = math.Round(*s.SavingsRate*1000) / 10
	}
	if len(s.Accounts) == 0 {
		out["note"] = "Nothing is imported yet: the person can import a CSV or OFX export from their bank with Import on the Finance page."
	}
	return out, nil
}

type FinanceQueryIn struct {
	Month         string `json:"month,omitempty" jsonschema:"YYYY-MM: just that month (instead of from/to)"`
	From          string `json:"from,omitempty" jsonschema:"YYYY-MM-DD, inclusive"`
	To            string `json:"to,omitempty" jsonschema:"YYYY-MM-DD, inclusive"`
	Category      string `json:"category,omitempty" jsonschema:"One category, or Uncategorized"`
	Search        string `json:"search,omitempty" jsonschema:"Words in the bank's description, e.g. a merchant"`
	Direction     string `json:"direction,omitempty" jsonschema:"in (money in), out (money out) or empty for both"`
	AccountID     string `json:"account_id,omitempty" jsonschema:"One account's id, from finance_summary"`
	Limit         int    `json:"limit,omitempty" jsonschema:"Transactions to return, up to 50 (default 20); count and total cover every match"`
	Uncategorized bool   `json:"uncategorized,omitempty" jsonschema:"Only transactions waiting for a category"`
}

func financeQuery(ctx context.Context, d Deps, c agents.Caller, in FinanceQueryIn) (any, error) {
	svc, o, err := financeFor(ctx, d, c, "finance_query")
	if err != nil {
		return nil, err
	}
	q := finance.QueryInput{Category: in.Category, Search: in.Search, Direction: in.Direction, Limit: in.Limit, Uncategorized: in.Uncategorized}
	if in.Month != "" {
		if in.From != "" || in.To != "" {
			return nil, apperr.Invalid("month", "give month or from/to, not both")
		}
		m, err := parseMonth("month", in.Month)
		if err != nil {
			return nil, err
		}
		end := m.AddDate(0, 1, -1)
		q.From, q.To = &m, &end
	} else {
		if q.From, err = parseDay("from", in.From); err != nil {
			return nil, err
		}
		if q.To, err = parseDay("to", in.To); err != nil {
			return nil, err
		}
	}
	if in.AccountID != "" {
		id, err := uuid.Parse(strings.TrimSpace(in.AccountID))
		if err != nil {
			return nil, apperr.Invalid("account_id", "use an account id from finance_summary")
		}
		q.Account = &id
	}
	res, err := svc.Query(ctx, o, q)
	if err != nil {
		return nil, err
	}
	rows := make([]map[string]any, len(res.Rows))
	for i, r := range res.Rows {
		row := map[string]any{"id": r.ID, "date": r.PostedOn.Format(time.DateOnly), "description": r.Description, "amount": dollars(r.AmountCents), "account": r.AccountName}
		if r.Category != nil {
			row["category"] = *r.Category
		}
		rows[i] = row
	}
	return map[string]any{"count": res.Count, "total": dollars(res.SumCents), "transactions": rows}, nil
}

func financeRecurring(ctx context.Context, d Deps, c agents.Caller, _ Empty) (any, error) {
	svc, o, err := financeFor(ctx, d, c, "finance_recurring")
	if err != nil {
		return nil, err
	}
	rs, err := svc.Recurring(ctx, o, finance.Now())
	if err != nil {
		return nil, err
	}
	var monthly int64
	items := make([]map[string]any, len(rs))
	for i, r := range rs {
		monthly += r.AmountCents
		item := map[string]any{"name": r.Label, "amount": dollars(r.AmountCents), "day_of_month": r.DayOfMonth, "since": r.FirstSeen.Format(time.DateOnly),
			"last_charged": r.LastSeen.Format(time.DateOnly), "charges": r.Count, "new": r.New}
		if r.PreviousCents != nil {
			item["previous_amount"] = dollars(*r.PreviousCents)
			item["price_changed_on"] = r.ChangedOn.Format(time.DateOnly)
		}
		items[i] = item
	}
	return map[string]any{"recurring": items, "monthly_total": dollars(monthly),
		"note": "Charges from one merchant 26–35 days apart at a similar price, still charged in the last 45 days."}, nil
}

type FinanceCategorizeIn struct {
	TransactionID string `json:"transaction_id" jsonschema:"From finance_query"`
	Category      string `json:"category" jsonschema:"One of Income, Housing, Utilities, Groceries, Dining, Transport, Travel, Shopping, Entertainment, Health, Subscriptions, Household, Gifts, Fees, Other, Transfer"`
	SaveRule      *bool  `json:"save_rule,omitempty" jsonschema:"Also categorize this merchant's past and future transactions (default true)"`
}

func financeCategorize(ctx context.Context, d Deps, c agents.Caller, in FinanceCategorizeIn) (any, error) {
	svc, o, err := financeFor(ctx, d, c, "finance_categorize")
	if err != nil {
		return nil, err
	}
	id, err := parseUUID("transaction_id", in.TransactionID)
	if err != nil {
		return nil, err
	}
	res, err := svc.Categorize(ctx, o, id, in.Category, in.SaveRule == nil || *in.SaveRule)
	if err != nil {
		return nil, err
	}
	out := ok(map[string]any{"category": *res.Transaction.Category})
	if res.Pattern != "" {
		out["rule"] = "merchants starting " + res.Pattern
		out["also_updated"] = res.Updated
	}
	return out, nil
}

type FinanceMapCSVIn struct {
	ImportID     string   `json:"import_id" jsonschema:"The pending import named in your instructions"`
	Delimiter    string   `json:"delimiter" jsonschema:"One character: , ; | or a tab"`
	HeaderRow    int      `json:"header_row" jsonschema:"1-based row holding the column names (blank lines aren't rows); 0 if there is none"`
	Date         int      `json:"date" jsonschema:"0-based date column"`
	DateFormat   string   `json:"date_format" jsonschema:"MM/DD/YYYY, DD/MM/YYYY, YYYY-MM-DD, MM/DD/YY, DD/MM/YY, DD.MM.YYYY, YYYYMMDD, MMM D, YYYY or D MMM YYYY"`
	Description  []int    `json:"description" jsonschema:"1–3 columns joined into the description"`
	Amount       *int     `json:"amount,omitempty" jsonschema:"Signed amount column, or unsigned with direction"`
	Direction    *int     `json:"direction,omitempty" jsonschema:"Column saying which way the money went, with an unsigned amount"`
	DebitValues  []string `json:"debit_values,omitempty" jsonschema:"The direction column's values that mean money out, e.g. DR"`
	Debit        *int     `json:"debit,omitempty" jsonschema:"Money-out column, with credit, instead of amount"`
	Credit       *int     `json:"credit,omitempty" jsonschema:"Money-in column, with debit, instead of amount"`
	Balance      *int     `json:"balance,omitempty" jsonschema:"Running balance column, if any"`
	DecimalComma bool     `json:"decimal_comma,omitempty" jsonschema:"Amounts are written 1.234,56"`
	FlipSign     bool     `json:"flip_sign,omitempty" jsonschema:"Purchases show as positive numbers"`
	Note         string   `json:"note,omitempty" jsonschema:"One sentence on how you read the file, shown to the person"`
}

// financeMapCSV takes the assistant's mapping for a pending CSV import. It is
// kept only if it reads every row; the error otherwise names the row, so the
// assistant can fix it. The person still confirms it in the import dialog.
func financeMapCSV(ctx context.Context, d Deps, c agents.Caller, in FinanceMapCSVIn) (any, error) {
	svc, o, err := financeFor(ctx, d, c, "finance_map_csv")
	if err != nil {
		return nil, err
	}
	id, err := parseUUID("import_id", in.ImportID)
	if err != nil {
		return nil, err
	}
	m := finance.CSVMapping{Delimiter: in.Delimiter, HeaderRow: in.HeaderRow, Date: in.Date, DateFormat: strings.TrimSpace(in.DateFormat), Description: in.Description,
		Amount: in.Amount, Direction: in.Direction, DebitValues: in.DebitValues, Debit: in.Debit, Credit: in.Credit, Balance: in.Balance,
		DecimalComma: in.DecimalComma, FlipSign: in.FlipSign}
	pv, err := svc.ProposeMapping(ctx, o, id, m, in.Note)
	if errors.Is(err, apperr.ErrNotFound) {
		return nil, apperr.Invalid("import_id", "no pending import %s: it was imported or cancelled", in.ImportID)
	}
	if err != nil {
		return nil, err
	}
	rows := make([]map[string]any, len(pv.Rows))
	for i, r := range pv.Rows {
		rows[i] = map[string]any{"date": r.PostedOn.Format(time.DateOnly), "description": r.Description, "amount": dollars(r.AmountCents)}
	}
	return ok(map[string]any{"transactions": pv.Count, "from": pv.FirstDate.Format(time.DateOnly), "to": pv.LastDate.Format(time.DateOnly),
		"money_in": dollars(pv.InCents), "money_out": dollars(pv.OutCents), "first_rows": rows,
		"message": "Accepted: every row reads. Check the first rows look right (purchases negative); if not, call again. The person confirms it in the import dialog."}), nil
}
