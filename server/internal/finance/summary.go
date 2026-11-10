package finance

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Month is one calendar month of cash flow.
type Month struct {
	Month            time.Time // the 1st
	InflowCents      int64
	OutflowCents     int64
	TransferOutCents int64
	// NetWorthCents at the month's end (today for the current month); nil
	// until an account has a balance.
	NetWorthCents *int64
}

type CategorySpend struct {
	Category   string
	SpentCents int64
	// AverageCents over up to six months before, counting only months with
	// any spending (so a new import isn't averaged with empty months).
	AverageCents int64
}

type ReviewItem struct {
	store.FinanceReviewQueueRow
	Suggestions []string
}

type Overview struct {
	Accounts  []Account
	Months    []Month // oldest first, ending with the current month
	NetWorth  *int64
	NetChange *int64 // since the end of last month
	// SavingsRate over the months shown: (in − out) / in; nil with no income.
	SavingsRate      *float64
	CategoryMonth    time.Time
	Categories       []CategorySpend
	Review           []ReviewItem
	ReviewCount      int
	TransactionCount int
	Recurring        []Recurring
	LastImport       *store.FinanceImport
}

func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func day(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) }

// Overview is everything the dashboard shows, as of today.
func (s *Service) Overview(ctx context.Context, o Owner, today time.Time, months int) (Overview, error) {
	var out Overview
	today = day(today)
	accts, err := s.Accounts(ctx, o)
	if err != nil {
		return out, err
	}
	out.Accounts = accts
	if out.Months, err = s.cashflow(ctx, o, today, months); err != nil {
		return out, err
	}
	if err := s.netWorth(ctx, o, accts, today, out.Months); err != nil {
		return out, err
	}
	if n := len(out.Months); n > 0 && out.Months[n-1].NetWorthCents != nil {
		out.NetWorth = out.Months[n-1].NetWorthCents
		if n > 1 && out.Months[n-2].NetWorthCents != nil {
			d := *out.NetWorth - *out.Months[n-2].NetWorthCents
			out.NetChange = &d
		}
	}
	var in, spent int64
	for _, m := range out.Months {
		in += m.InflowCents
		spent += m.OutflowCents
	}
	if in > 0 {
		r := float64(in-spent) / float64(in)
		out.SavingsRate = &r
	}
	out.CategoryMonth, out.Categories, err = s.categorySpend(ctx, o, today, time.Time{})
	if err != nil {
		return out, err
	}
	q := s.q()
	counts, err := q.FinanceCounts(ctx, store.FinanceCountsParams{WorkspaceID: o.Workspace, UserID: o.User})
	if err != nil {
		return out, err
	}
	out.TransactionCount, out.ReviewCount = int(counts.Total), int(counts.Uncategorized)
	if out.Review, err = s.review(ctx, o, 25); err != nil {
		return out, err
	}
	if out.Recurring, err = s.Recurring(ctx, o, today); err != nil {
		return out, err
	}
	last, err := q.LastFinanceImport(ctx, store.LastFinanceImportParams{WorkspaceID: o.Workspace, UserID: o.User})
	if err == nil {
		out.LastImport = &last
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	return out, nil
}

// cashflow is the last n months of money in and out, oldest first, with
// empty months filled in.
func (s *Service) cashflow(ctx context.Context, o Owner, today time.Time, n int) ([]Month, error) {
	n = min(max(n, 1), 24)
	first := monthStart(today).AddDate(0, 1-n, 0)
	rows, err := s.q().FinanceMonthlyTotals(ctx, store.FinanceMonthlyTotalsParams{WorkspaceID: o.Workspace, UserID: o.User, FromDate: first})
	if err != nil {
		return nil, err
	}
	out := make([]Month, n)
	for i := range out {
		out[i].Month = first.AddDate(0, i, 0)
		if j := slices.IndexFunc(rows, func(r store.FinanceMonthlyTotalsRow) bool { return monthStart(r.Month).Equal(out[i].Month) }); j >= 0 {
			out[i].InflowCents, out[i].OutflowCents, out[i].TransferOutCents = rows[j].InflowCents, rows[j].OutflowCents, rows[j].TransferOutCents
		}
	}
	return out, nil
}

// netWorth fills each month's net worth: every account with a balance,
// rolled from its newest balance to the month's end by the transactions in
// between. Accounts with no balance yet are left out.
func (s *Service) netWorth(ctx context.Context, o Owner, accts []Account, today time.Time, months []Month) error {
	anchored := map[uuid.UUID]Balance{}
	for _, a := range accts {
		if a.Balance != nil {
			anchored[a.ID] = *a.Balance
		}
	}
	if len(anchored) == 0 {
		return nil
	}
	rows, err := s.q().FinanceAccountDailyNet(ctx, store.FinanceAccountDailyNetParams{WorkspaceID: o.Workspace, UserID: o.User})
	if err != nil {
		return err
	}
	for i := range months {
		end := months[i].Month.AddDate(0, 1, -1)
		if end.After(today) {
			end = today
		}
		var total int64
		for id, b := range anchored {
			total += balanceOn(b, end, rows, id)
		}
		months[i].NetWorthCents = &total
	}
	return nil
}

// balanceOn rolls an account's anchor balance to another day.
func balanceOn(anchor Balance, on time.Time, daily []store.FinanceAccountDailyNetRow, account uuid.UUID) int64 {
	bal := anchor.Cents
	for _, d := range daily {
		if d.AccountID != account {
			continue
		}
		switch {
		case on.After(anchor.AsOf) && d.PostedOn.After(anchor.AsOf) && !d.PostedOn.After(on):
			bal += d.NetCents
		case on.Before(anchor.AsOf) && d.PostedOn.After(on) && !d.PostedOn.After(anchor.AsOf):
			bal -= d.NetCents
		}
	}
	return bal
}

// categorySpend is spending by category in a month (by default the last full
// month with data, else this one) against the months before it.
func (s *Service) categorySpend(ctx context.Context, o Owner, today, month time.Time) (time.Time, []CategorySpend, error) {
	this := monthStart(today)
	from := this.AddDate(0, -7, 0)
	if !month.IsZero() {
		from = monthStart(month).AddDate(0, -6, 0)
	}
	rows, err := s.q().FinanceCategoryMonthly(ctx, store.FinanceCategoryMonthlyParams{WorkspaceID: o.Workspace, UserID: o.User, FromDate: from})
	if err != nil {
		return month, nil, err
	}
	if month.IsZero() {
		month = this
		last := this.AddDate(0, -1, 0)
		if slices.ContainsFunc(rows, func(r store.FinanceCategoryMonthlyRow) bool { return monthStart(r.Month).Equal(last) }) {
			month = last
		}
	}
	month = monthStart(month)
	spent := map[string]int64{}
	prior := map[string]int64{}
	priorMonths := map[time.Time]bool{}
	for _, r := range rows {
		m := monthStart(r.Month)
		switch {
		case m.Equal(month):
			spent[r.Category] += r.SpentCents
		case m.Before(month) && !m.Before(month.AddDate(0, -6, 0)):
			prior[r.Category] += r.SpentCents
			priorMonths[m] = true
		}
	}
	var out []CategorySpend
	for c, v := range spent {
		cs := CategorySpend{Category: c, SpentCents: v}
		if n := int64(len(priorMonths)); n > 0 {
			cs.AverageCents = prior[c] / n
		}
		out = append(out, cs)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SpentCents != out[j].SpentCents {
			return out[i].SpentCents > out[j].SpentCents
		}
		return out[i].Category < out[j].Category
	})
	return month, out, nil
}

func (s *Service) review(ctx context.Context, o Owner, limit int) ([]ReviewItem, error) {
	q := s.q()
	rows, err := q.FinanceReviewQueue(ctx, store.FinanceReviewQueueParams{WorkspaceID: o.Workspace, UserID: o.User, RowLimit: int32(limit)})
	if err != nil {
		return nil, err
	}
	used, err := q.FinanceMerchantCategories(ctx, store.FinanceMerchantCategoriesParams{WorkspaceID: o.Workspace, UserID: o.User})
	if err != nil {
		return nil, err
	}
	learned := map[string][]string{}
	for _, u := range used { // most used first
		learned[u.Word] = append(learned[u.Word], u.Category)
	}
	out := make([]ReviewItem, len(rows))
	for i, r := range rows {
		out[i] = ReviewItem{FinanceReviewQueueRow: r, Suggestions: Suggest(r.Merchant, r.AmountCents, learned)}
	}
	return out, nil
}

// Recurring is the person's monthly charges, from the last 13 months.
func (s *Service) Recurring(ctx context.Context, o Owner, today time.Time) ([]Recurring, error) {
	rows, err := s.q().FinanceOutflowsSince(ctx, store.FinanceOutflowsSinceParams{WorkspaceID: o.Workspace, UserID: o.User, FromDate: monthStart(today).AddDate(0, -13, 0)})
	if err != nil {
		return nil, err
	}
	in := make([]Outflow, len(rows))
	for i, r := range rows {
		in[i] = Outflow{Merchant: r.Merchant, Description: r.Description, PostedOn: r.PostedOn, AmountCents: r.AmountCents}
	}
	return DetectRecurring(in, day(today)), nil
}

// Summary is what finance_summary answers with: cash flow for n months and
// spending by category for one month.
type Summary struct {
	Months        []Month
	NetWorth      *int64
	SavingsRate   *float64
	CategoryMonth time.Time
	Categories    []CategorySpend
	Uncategorized int
	Accounts      []Account
}

func (s *Service) Summary(ctx context.Context, o Owner, today time.Time, months int, month time.Time) (Summary, error) {
	var out Summary
	today = day(today)
	accts, err := s.Accounts(ctx, o)
	if err != nil {
		return out, err
	}
	out.Accounts = accts
	if out.Months, err = s.cashflow(ctx, o, today, months); err != nil {
		return out, err
	}
	if err := s.netWorth(ctx, o, accts, today, out.Months); err != nil {
		return out, err
	}
	if n := len(out.Months); n > 0 {
		out.NetWorth = out.Months[n-1].NetWorthCents
	}
	var in, spent int64
	for _, m := range out.Months {
		in += m.InflowCents
		spent += m.OutflowCents
	}
	if in > 0 {
		r := float64(in-spent) / float64(in)
		out.SavingsRate = &r
	}
	if out.CategoryMonth, out.Categories, err = s.categorySpend(ctx, o, today, month); err != nil {
		return out, err
	}
	counts, err := s.q().FinanceCounts(ctx, store.FinanceCountsParams{WorkspaceID: o.Workspace, UserID: o.User})
	out.Uncategorized = int(counts.Uncategorized)
	return out, err
}

// QueryInput filters transactions; zero values don't filter.
type QueryInput struct {
	From, To      *time.Time
	Category      string
	Uncategorized bool
	Account       *uuid.UUID
	Search        string
	Direction     string // in, out or empty
	Limit         int
}

type QueryResult struct {
	Rows     []store.QueryFinanceTransactionsRow
	Count    int
	SumCents int64
}

func (s *Service) Query(ctx context.Context, o Owner, in QueryInput) (QueryResult, error) {
	var out QueryResult
	arg := store.QueryFinanceTransactionsParams{WorkspaceID: o.Workspace, UserID: o.User, FromDate: in.From, ToDate: in.To, AccountID: in.Account,
		Uncategorized: in.Uncategorized, RowLimit: int32(min(max(in.Limit, 1), 50))}
	if in.Limit <= 0 {
		arg.RowLimit = 20
	}
	if c := strings.TrimSpace(in.Category); c != "" {
		canon, ok := CanonicalCategory(c)
		if !ok && !strings.EqualFold(c, Uncategorized) {
			return out, apperr.Invalid("category", "must be one of %s, or Uncategorized", strings.Join(Categories, ", "))
		}
		if canon == "" {
			arg.Uncategorized = true
		} else {
			arg.Category = &canon
		}
	}
	if q := strings.TrimSpace(in.Search); q != "" {
		esc := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q)
		arg.Search = &esc
	}
	switch in.Direction {
	case "":
	case "in", "out":
		arg.Direction = &in.Direction
	default:
		return out, apperr.Invalid("direction", "in, out or empty")
	}
	rows, err := s.q().QueryFinanceTransactions(ctx, arg)
	if err != nil {
		return out, err
	}
	out.Rows = rows
	if len(rows) > 0 {
		out.Count, out.SumCents = int(rows[0].MatchCount), rows[0].MatchSumCents
	}
	return out, nil
}
