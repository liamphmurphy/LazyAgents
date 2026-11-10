package finance

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// AppName marks the Finance app's space in chat_spaces.app.
const AppName = "finance"

// MaxImportBytes bounds an uploaded statement.
const MaxImportBytes = 5 << 20

// SpaceInstructions reach every chat in the Finance space: the Ask drawer.
const SpaceInstructions = `This chat is Ask in the person's Finance app. Answer questions about their money from their imported transactions, using the finance_* tools: finance_summary for cash flow, spending by category, net worth and savings rate; finance_query for the transactions behind a number; finance_recurring for subscriptions and other monthly charges. Never estimate or invent figures: if the tools don't have the data, say what to import.
Amounts are in dollars; negative is money out. Transfer is money moving between their own accounts (such as a card payment) and is left out of income and spending.
When they say how a transaction should be categorized, use finance_categorize; by default it saves a rule for that merchant, so tell them it will apply to future imports too.`

// Now is the clock the dashboard and tools read "today" from; tests pin it.
var Now = func() time.Time { return time.Now().UTC() }

// Owner is the person whose finances these are.
type Owner struct {
	Workspace uuid.UUID
	User      uuid.UUID
}

type Service struct {
	Pool *pgxpool.Pool
}

func (s *Service) tx(ctx context.Context, fn func(q *store.Queries) error) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error { return fn(store.New(tx)) })
}

func (s *Service) q() *store.Queries { return store.New(s.Pool) }

// EnsureSpace is the person's Finance space, made the first time it's needed.
func (s *Service) EnsureSpace(ctx context.Context, o Owner) (store.ChatSpace, error) {
	q := s.q()
	sp, err := q.GetAppSpace(ctx, store.GetAppSpaceParams{WorkspaceID: o.Workspace, UserID: o.User, AppName: AppName})
	if !errors.Is(err, pgx.ErrNoRows) {
		return sp, err
	}
	return q.CreateAppSpace(ctx, store.CreateAppSpaceParams{WorkspaceID: o.Workspace, UserID: o.User, Name: "Finance", Color: "green",
		Description: "Ask about your money. Answers come from your imported transactions.", Instructions: SpaceInstructions, AppName: AppName})
}

// IsFinanceSpace reports whether a space is someone's Finance space.
func IsFinanceSpace(sp store.ChatSpace) bool { return sp.App != nil && *sp.App == AppName }

// Account is one of the person's accounts with its newest balance.
type Account struct {
	store.FinanceAccount
	TransactionCount int
	Balance          *Balance
}

var AccountKinds = []string{"checking", "savings", "credit", "brokerage", "other"}

func (s *Service) Accounts(ctx context.Context, o Owner) ([]Account, error) {
	q := s.q()
	rows, err := q.ListFinanceAccounts(ctx, store.ListFinanceAccountsParams{WorkspaceID: o.Workspace, UserID: o.User})
	if err != nil {
		return nil, err
	}
	bals, err := q.LatestFinanceBalances(ctx, store.LatestFinanceBalancesParams{WorkspaceID: o.Workspace, UserID: o.User})
	if err != nil {
		return nil, err
	}
	out := make([]Account, len(rows))
	for i, r := range rows {
		out[i] = Account{FinanceAccount: store.FinanceAccount{ID: r.ID, WorkspaceID: r.WorkspaceID, UserID: r.UserID, Name: r.Name, Kind: r.Kind, Mask: r.Mask,
			OfxAccountID: r.OfxAccountID, CreatedAt: r.CreatedAt}, TransactionCount: int(r.TransactionCount)}
		if j := slices.IndexFunc(bals, func(b store.FinanceBalance) bool { return b.AccountID == r.ID }); j >= 0 {
			out[i].Balance = &Balance{AsOf: bals[j].AsOf, Cents: bals[j].BalanceCents}
		}
	}
	return out, nil
}

// NewAccount is an account the person adds by hand, for CSV imports.
type NewAccount struct {
	Name string
	Kind string
	Mask string
}

func (s *Service) CreateAccount(ctx context.Context, o Owner, in NewAccount) (store.FinanceAccount, error) {
	return createAccount(ctx, s.q(), o, in, nil)
}

func createAccount(ctx context.Context, q *store.Queries, o Owner, in NewAccount, ofxID *string) (store.FinanceAccount, error) {
	name := strings.Join(strings.Fields(in.Name), " ")
	if name == "" || len([]rune(name)) > 80 {
		return store.FinanceAccount{}, apperr.Invalid("name", "must be 1–80 characters")
	}
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	if !slices.Contains(AccountKinds, kind) {
		return store.FinanceAccount{}, apperr.Invalid("kind", "must be one of %s", strings.Join(AccountKinds, ", "))
	}
	mask := strings.TrimSpace(in.Mask)
	if len(mask) > 8 {
		return store.FinanceAccount{}, apperr.Invalid("mask", "is the last few digits of the account, up to 8 characters")
	}
	a, err := q.CreateFinanceAccount(ctx, store.CreateFinanceAccountParams{WorkspaceID: o.Workspace, UserID: o.User, Name: name, Kind: kind, Mask: mask, OfxAccountID: ofxID})
	if isUnique(err) {
		return a, apperr.Invalid("name", "you already have an account called %s", name)
	}
	return a, err
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 23505")
}

func (s *Service) account(ctx context.Context, q *store.Queries, o Owner, id uuid.UUID) (store.FinanceAccount, error) {
	a, err := q.GetFinanceAccount(ctx, store.GetFinanceAccountParams{ID: id, WorkspaceID: o.Workspace, UserID: o.User})
	if errors.Is(err, pgx.ErrNoRows) {
		return a, apperr.ErrNotFound
	}
	return a, err
}

// SetBalance records an account's balance on a day, for net worth.
func (s *Service) SetBalance(ctx context.Context, o Owner, account uuid.UUID, cents int64, asOf time.Time) error {
	q := s.q()
	if _, err := s.account(ctx, q, o, account); err != nil {
		return err
	}
	return q.UpsertFinanceBalance(ctx, store.UpsertFinanceBalanceParams{AccountID: account, AsOf: asOf, BalanceCents: cents})
}

// ImportInput is one uploaded statement file.
type ImportInput struct {
	Filename string
	Data     []byte
	// Account is required for a CSV; an OFX names its own account.
	Account *uuid.UUID
	// FlipSign: a card's CSV that shows purchases as positive.
	FlipSign bool
}

type ImportResult struct {
	Import  store.FinanceImport
	Account store.FinanceAccount
	// AccountCreated: an OFX file for an account Vesper hadn't seen.
	AccountCreated bool
	NeedsReview    int
	FirstDate      time.Time
	LastDate       time.Time
	// Pending: a CSV whose layout needs mapping first; nothing was imported.
	Pending *Pending
}

// Import adds a statement's new transactions: duplicates of earlier imports
// are skipped, the person's rules (then built-in keywords) categorize the
// rest, and transfers between their accounts are matched up.
func (s *Service) Import(ctx context.Context, o Owner, in ImportInput) (ImportResult, error) {
	var out ImportResult
	if len(in.Data) == 0 {
		return out, apperr.Invalid("file", "is empty")
	}
	if len(in.Data) > MaxImportBytes {
		return out, apperr.Invalid("file", "statements are limited to 5 MB; export a shorter date range")
	}
	if LooksLikeOFX(in.Filename, in.Data) {
		p, err := ParseOFX(in.Data)
		if err != nil {
			return out, err
		}
		return s.write(ctx, o, p, in.Filename, in.Account)
	}
	// A CSV goes into the account the person picked. Its layout is read with
	// the mapping saved for it, else header detection; a layout neither
	// knows waits as a pending import for the mapping step.
	if in.Account == nil {
		return out, apperr.Invalid("account_id", "pick the account this CSV is from")
	}
	if _, err := s.account(ctx, s.q(), o, *in.Account); errors.Is(err, apperr.ErrNotFound) {
		return out, apperr.Invalid("account_id", "unknown account")
	} else if err != nil {
		return out, err
	}
	fp := CSVFingerprint(in.Data)
	saved, hasSaved, err := s.savedMapping(ctx, o, fp)
	if err != nil {
		return out, err
	}
	if hasSaved {
		if p, err := ParseCSVWith(in.Data, saved); err == nil {
			return s.write(ctx, o, p, in.Filename, in.Account)
		}
	}
	prefill := GuessCSV(in.Data)
	if m, ok := DetectCSV(in.Data); ok {
		m.FlipSign = in.FlipSign
		prefill = m
		p, err := ParseCSVWith(in.Data, m)
		if err == nil {
			return s.write(ctx, o, p, in.Filename, in.Account)
		}
		var ve *apperr.ValidationError
		if !errors.As(err, &ve) {
			return out, err
		}
	}
	if hasSaved {
		prefill = saved
	}
	prefill.FlipSign = prefill.FlipSign || in.FlipSign
	pending, err := s.createPending(ctx, o, *in.Account, in.Filename, in.Data, prefill)
	out.Pending = &pending
	return out, err
}

// write adds a parsed statement's new transactions to an account.
func (s *Service) write(ctx context.Context, o Owner, p Parsed, filename string, account *uuid.UUID) (ImportResult, error) {
	var out ImportResult
	if len(p.Txns) == 0 {
		return out, apperr.Invalid("file", "has no transactions")
	}
	err := s.tx(ctx, func(q *store.Queries) error {
		acct, created, err := s.importAccount(ctx, q, o, p, account)
		if err != nil {
			return err
		}
		out.Account, out.AccountCreated = acct, created
		imp, err := q.CreateFinanceImport(ctx, store.CreateFinanceImportParams{AccountID: acct.ID, Filename: importName(filename), Format: p.Format, Parsed: int32(len(p.Txns))})
		if err != nil {
			return err
		}
		rules, err := q.ListFinanceRules(ctx, store.ListFinanceRulesParams{WorkspaceID: o.Workspace, UserID: o.User})
		if err != nil {
			return err
		}
		var inserted []store.FinanceTransaction
		duplicates := 0
		for _, t := range p.Txns {
			if out.FirstDate.IsZero() || t.PostedOn.Before(out.FirstDate) {
				out.FirstDate = t.PostedOn
			}
			if t.PostedOn.After(out.LastDate) {
				out.LastDate = t.PostedOn
			}
			merchant := Merchant(t.Description)
			arg := store.InsertFinanceTransactionParams{AccountID: acct.ID, ImportID: &imp.ID, PostedOn: t.PostedOn, AmountCents: t.AmountCents,
				Description: t.Description, Merchant: merchant, DedupeKey: t.DedupeKey}
			arg.Category, arg.CategorySource = categorize(rules, merchant, t.AmountCents)
			row, err := q.InsertFinanceTransaction(ctx, arg)
			if errors.Is(err, pgx.ErrNoRows) {
				duplicates++
				continue
			}
			if err != nil {
				return err
			}
			inserted = append(inserted, row)
		}
		transfers := 0
		matched := map[uuid.UUID]bool{}
		for _, t := range inserted {
			if matched[t.ID] || t.CategorySource != nil && *t.CategorySource == "person" {
				continue
			}
			other, err := q.FindFinanceTransferMatch(ctx, store.FindFinanceTransferMatchParams{WorkspaceID: o.Workspace, UserID: o.User, AccountID: acct.ID,
				AmountCents: t.AmountCents, Earliest: t.PostedOn.AddDate(0, 0, -5), Latest: t.PostedOn.AddDate(0, 0, 5), PostedOn: t.PostedOn})
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			if !transferish(t.Merchant) && !transferish(other.Merchant) {
				continue
			}
			if err := q.LinkFinanceTransfer(ctx, store.LinkFinanceTransferParams{FirstID: t.ID, SecondID: other.ID}); err != nil {
				return err
			}
			matched[t.ID], matched[other.ID] = true, true
			transfers++
		}
		categorized := 0
		for _, t := range inserted {
			if t.Category != nil || matched[t.ID] {
				categorized++
			}
		}
		out.NeedsReview = len(inserted) - categorized
		if p.Balance != nil {
			if err := q.UpsertFinanceBalance(ctx, store.UpsertFinanceBalanceParams{AccountID: acct.ID, AsOf: p.Balance.AsOf, BalanceCents: p.Balance.Cents}); err != nil {
				return err
			}
		}
		out.Import, err = q.FinishFinanceImport(ctx, store.FinishFinanceImportParams{ID: imp.ID, Inserted: int32(len(inserted)), Duplicates: int32(duplicates),
			Transfers: int32(transfers), Categorized: int32(categorized)})
		return err
	})
	return out, err
}

func importName(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	if name == "" {
		return "statement"
	}
	if r := []rune(name); len(r) > 120 {
		name = string(r[:120])
	}
	return name
}

// importAccount is the account a statement goes into: the one picked, else
// the one its OFX account id was seen in before, else (OFX only) a new one.
func (s *Service) importAccount(ctx context.Context, q *store.Queries, o Owner, p Parsed, picked *uuid.UUID) (store.FinanceAccount, bool, error) {
	if picked != nil {
		a, err := s.account(ctx, q, o, *picked)
		if errors.Is(err, apperr.ErrNotFound) {
			return a, false, apperr.Invalid("account_id", "unknown account")
		}
		if err != nil {
			return a, false, err
		}
		if p.AccountID != "" {
			if a.OfxAccountID != nil && *a.OfxAccountID != p.AccountID {
				return a, false, apperr.Invalid("account_id", "this file is for a different account than %s", a.Name)
			}
			owner, err := q.GetFinanceAccountByOFX(ctx, store.GetFinanceAccountByOFXParams{WorkspaceID: o.Workspace, UserID: o.User, OfxAccountID: &p.AccountID})
			if err == nil && owner.ID != a.ID {
				return a, false, apperr.Invalid("account_id", "this file is for %s, not %s", owner.Name, a.Name)
			}
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return a, false, err
			}
			if err := q.SetFinanceAccountOFX(ctx, store.SetFinanceAccountOFXParams{ID: a.ID, OfxAccountID: &p.AccountID}); err != nil {
				return a, false, err
			}
		}
		return a, false, nil
	}
	if p.AccountID == "" {
		return store.FinanceAccount{}, false, apperr.Invalid("account_id", "pick the account this CSV is from")
	}
	a, err := q.GetFinanceAccountByOFX(ctx, store.GetFinanceAccountByOFXParams{WorkspaceID: o.Workspace, UserID: o.User, OfxAccountID: &p.AccountID})
	if err == nil {
		return a, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return a, false, err
	}
	kind := "checking"
	switch {
	case p.Credit:
		kind = "credit"
	case p.AccountType == "savings" || p.AccountType == "moneymrkt":
		kind = "savings"
	}
	mask := p.AccountID
	if len(mask) > 4 {
		mask = mask[len(mask)-4:]
	}
	base := strings.ToUpper(kind[:1]) + kind[1:] + " ••" + mask
	for n := 1; ; n++ {
		name := base
		if n > 1 {
			name = fmt.Sprintf("%s (%d)", base, n)
		}
		a, err = createAccount(ctx, q, o, NewAccount{Name: name, Kind: kind, Mask: mask}, &p.AccountID)
		var ve *apperr.ValidationError
		if errors.As(err, &ve) && ve.Field == "name" && n < 10 {
			continue
		}
		return a, err == nil, err
	}
}

// categorize is a new transaction's category: the person's longest matching
// rule, else a built-in keyword, else none (it goes to review).
func categorize(rules []store.FinanceRule, merchant string, cents int64) (*string, *string) {
	for _, r := range rules { // longest pattern first
		if ruleMatches(r.Pattern, merchant) {
			c, src := r.Category, "rule"
			return &c, &src
		}
	}
	if c, ok := DefaultCategory(merchant, cents); ok {
		src := "default"
		return &c, &src
	}
	return nil, nil
}

type CategorizeResult struct {
	Transaction store.FinanceTransaction
	// Pattern is the rule saved, if any; Updated how many other transactions it recategorized.
	Pattern string
	Updated int
}

// Categorize sets a transaction's category and, with saveRule, makes a rule
// for its merchant that also recategorizes past and future matches.
func (s *Service) Categorize(ctx context.Context, o Owner, id uuid.UUID, category string, saveRule bool) (CategorizeResult, error) {
	var out CategorizeResult
	cat, ok := CanonicalCategory(category)
	if !ok {
		return out, apperr.Invalid("category", "must be one of %s", strings.Join(Categories, ", "))
	}
	err := s.tx(ctx, func(q *store.Queries) error {
		t, err := q.GetFinanceTransaction(ctx, store.GetFinanceTransactionParams{ID: id, WorkspaceID: o.Workspace, UserID: o.User})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		}
		if err != nil {
			return err
		}
		if t.TransferID != nil && cat != Transfer {
			if err := q.UnlinkFinanceTransfer(ctx, &t.ID); err != nil {
				return err
			}
		}
		if err := q.SetFinanceTransactionCategory(ctx, store.SetFinanceTransactionCategoryParams{ID: t.ID, Category: &cat, CategorySource: ptr("person")}); err != nil {
			return err
		}
		t.Category, t.CategorySource = &cat, ptr("person")
		if cat != Transfer {
			t.TransferID = nil
		}
		out.Transaction = t
		if !saveRule {
			return nil
		}
		pattern := RulePattern(t.Merchant)
		if pattern == "" {
			return nil
		}
		if _, err := q.UpsertFinanceRule(ctx, store.UpsertFinanceRuleParams{WorkspaceID: o.Workspace, UserID: o.User, Pattern: pattern, Category: cat}); err != nil {
			return err
		}
		n, err := q.ApplyFinanceRule(ctx, store.ApplyFinanceRuleParams{Category: cat, WorkspaceID: o.Workspace, UserID: o.User, Pattern: pattern, ExceptID: t.ID})
		out.Pattern, out.Updated = pattern, int(n)
		return err
	})
	return out, err
}

func ptr[T any](v T) *T { return &v }
