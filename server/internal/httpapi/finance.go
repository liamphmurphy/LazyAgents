package httpapi

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/finance"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// The Finance app: one person's accounts, imports and categories. Every
// lookup is scoped to the caller; someone else's is a 404.

func (s *Server) finance() *finance.Service { return &finance.Service{Pool: s.Pool} }

func financeOwner(ctx context.Context) finance.Owner {
	u, _, ws := me(ctx)
	return finance.Owner{Workspace: ws, User: u.ID}
}

func apiDate(t time.Time) openapi_types.Date { return openapi_types.Date{Time: t} }

func optDate(t time.Time) *openapi_types.Date {
	if t.IsZero() {
		return nil
	}
	d := apiDate(t)
	return &d
}

func toFinanceAccount(a finance.Account) api.FinanceAccount {
	out := api.FinanceAccount{Id: a.ID, Name: a.Name, Kind: api.FinanceAccountKind(a.Kind), Mask: a.Mask, TransactionCount: a.TransactionCount, FromOfx: a.OfxAccountID != nil}
	if a.Balance != nil {
		out.BalanceCents, out.BalanceAsOf = &a.Balance.Cents, optDate(a.Balance.AsOf)
	}
	return out
}

func toFinanceImport(i store.FinanceImport) api.FinanceImportSummary {
	return api.FinanceImportSummary{Id: i.ID, AccountId: i.AccountID, Filename: i.Filename, Format: api.FinanceImportSummaryFormat(i.Format), Parsed: int(i.Parsed),
		Inserted: int(i.Inserted), Duplicates: int(i.Duplicates), Transfers: int(i.Transfers), Categorized: int(i.Categorized), CreatedAt: i.CreatedAt}
}

func (s *Server) GetFinanceOverview(ctx context.Context, req api.GetFinanceOverviewRequestObject) (api.GetFinanceOverviewResponseObject, error) {
	o := financeOwner(ctx)
	months := 12
	if req.Params.Months != nil {
		if *req.Params.Months < 1 || *req.Params.Months > 24 {
			return nil, apperr.Invalid("months", "must be 1–24")
		}
		months = *req.Params.Months
	}
	svc := s.finance()
	sp, err := svc.EnsureSpace(ctx, o)
	if err != nil {
		return nil, err
	}
	ov, err := svc.Overview(ctx, o, finance.Now(), months)
	if err != nil {
		return nil, err
	}
	out := api.FinanceOverview{SpaceId: sp.ID, CategoriesAll: finance.Categories, Accounts: make([]api.FinanceAccount, len(ov.Accounts)),
		Months: make([]api.FinanceMonth, len(ov.Months)), NetWorthCents: ov.NetWorth, NetWorthChangeCents: ov.NetChange, SavingsRate: ov.SavingsRate,
		CategoryMonth: apiDate(ov.CategoryMonth), Categories: make([]api.FinanceCategorySpend, len(ov.Categories)), Review: make([]api.FinanceReviewItem, len(ov.Review)),
		ReviewCount: ov.ReviewCount, TransactionCount: ov.TransactionCount, Recurring: make([]api.FinanceRecurring, len(ov.Recurring))}
	for i, a := range ov.Accounts {
		out.Accounts[i] = toFinanceAccount(a)
	}
	for i, m := range ov.Months {
		out.Months[i] = api.FinanceMonth{Month: apiDate(m.Month), InflowCents: m.InflowCents, OutflowCents: m.OutflowCents, TransferOutCents: m.TransferOutCents, NetWorthCents: m.NetWorthCents}
	}
	for i, c := range ov.Categories {
		out.Categories[i] = api.FinanceCategorySpend{Category: c.Category, SpentCents: c.SpentCents, AverageCents: c.AverageCents}
	}
	for i, r := range ov.Review {
		out.Review[i] = api.FinanceReviewItem{Id: r.ID, PostedOn: apiDate(r.PostedOn), Description: r.Description, AmountCents: r.AmountCents,
			AccountName: r.AccountName, AccountMask: r.AccountMask, Suggestions: r.Suggestions}
	}
	for i, r := range ov.Recurring {
		item := api.FinanceRecurring{Label: r.Label, AmountCents: r.AmountCents, PreviousCents: r.PreviousCents, DayOfMonth: r.DayOfMonth,
			FirstSeen: apiDate(r.FirstSeen), LastSeen: apiDate(r.LastSeen), Count: r.Count, New: r.New}
		if r.ChangedOn != nil {
			item.ChangedOn = optDate(*r.ChangedOn)
		}
		out.Recurring[i] = item
	}
	if ov.LastImport != nil {
		li := toFinanceImport(*ov.LastImport)
		out.LastImport = &li
	}
	return api.GetFinanceOverview200JSONResponse(out), nil
}

func (s *Server) CreateFinanceAccount(ctx context.Context, req api.CreateFinanceAccountRequestObject) (api.CreateFinanceAccountResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("body", "required")
	}
	a, err := s.finance().CreateAccount(ctx, financeOwner(ctx), finance.NewAccount{Name: req.Body.Name, Kind: string(req.Body.Kind), Mask: deref(req.Body.Mask)})
	if err != nil {
		return nil, err
	}
	return api.CreateFinanceAccount201JSONResponse(toFinanceAccount(finance.Account{FinanceAccount: a})), nil
}

func (s *Server) SetFinanceAccountBalance(ctx context.Context, req api.SetFinanceAccountBalanceRequestObject) (api.SetFinanceAccountBalanceResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("body", "required")
	}
	o, svc := financeOwner(ctx), s.finance()
	if err := svc.SetBalance(ctx, o, req.AccountId, req.Body.BalanceCents, req.Body.AsOf.Time); err != nil {
		return nil, err
	}
	accts, err := svc.Accounts(ctx, o)
	if err != nil {
		return nil, err
	}
	for _, a := range accts {
		if a.ID == req.AccountId {
			return api.SetFinanceAccountBalance200JSONResponse(toFinanceAccount(a)), nil
		}
	}
	return nil, apperr.ErrNotFound
}

func (s *Server) ImportFinanceStatement(ctx context.Context, req api.ImportFinanceStatementRequestObject) (api.ImportFinanceStatementResponseObject, error) {
	f, err := readForm(req.Body)
	if err != nil {
		return nil, err
	}
	defer f.close()
	if f.file == nil {
		return nil, apperr.Invalid("file", "required")
	}
	if f.file.Size > finance.MaxImportBytes {
		return nil, apperr.Invalid("file", "statements are limited to 5 MB; export a shorter date range")
	}
	data, err := io.ReadAll(f.tmp)
	if err != nil {
		return nil, err
	}
	in := finance.ImportInput{Filename: f.file.Name, Data: data}
	if v := f.str("account_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return nil, apperr.Invalid("account_id", "must be an account id")
		}
		in.Account = &id
	}
	switch f.str("flip_sign") {
	case "", "false":
	case "true":
		in.FlipSign = true
	default:
		return nil, apperr.Invalid("flip_sign", "true or false")
	}
	o, svc := financeOwner(ctx), s.finance()
	res, err := svc.Import(ctx, o, in)
	if err != nil {
		return nil, err
	}
	out := api.FinanceImportResult{Import: toFinanceImport(res.Import), AccountCreated: res.AccountCreated, NeedsReview: res.NeedsReview,
		FirstDate: optDate(res.FirstDate), LastDate: optDate(res.LastDate), Account: toFinanceAccount(finance.Account{FinanceAccount: res.Account})}
	// The account as the dashboard shows it, with its new balance and count.
	if accts, err := svc.Accounts(ctx, o); err == nil {
		for _, a := range accts {
			if a.ID == res.Account.ID {
				out.Account = toFinanceAccount(a)
			}
		}
	}
	return api.ImportFinanceStatement201JSONResponse(out), nil
}

func (s *Server) CategorizeFinanceTransaction(ctx context.Context, req api.CategorizeFinanceTransactionRequestObject) (api.CategorizeFinanceTransactionResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("body", "required")
	}
	saveRule := req.Body.SaveRule == nil || *req.Body.SaveRule
	res, err := s.finance().Categorize(ctx, financeOwner(ctx), req.TransactionId, req.Body.Category, saveRule)
	if err != nil {
		return nil, err
	}
	out := api.FinanceCategorizeResult{Id: res.Transaction.ID, Category: deref(res.Transaction.Category), Updated: res.Updated}
	if res.Pattern != "" {
		out.Pattern = &res.Pattern
	}
	return api.CategorizeFinanceTransaction200JSONResponse(out), nil
}
