package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/liamphmurphy/vesper/server/internal/finance"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
)

// 00112 undoes what the DIRECT DEBIT and POUND STERLING rules recategorized,
// drops those rules, and leaves every stored merchant equal to what an import
// computes now, so rules made afterwards match old and new rows alike.
func TestFinanceMerchantMigration(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	sqldb := stdlib.OpenDBFromPool(pool)
	defer sqldb.Close()
	if err := goose.DownToContext(ctx, sqldb, ".", 111); err != nil {
		t.Fatal(err)
	}
	var ws, user, acct uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces(name,slug) VALUES('W','w') RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(workspace_id,name,email,initials,password_hash) VALUES($1,'U','u@example.com','U','x') RETURNING id`, ws).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO finance_accounts(workspace_id,user_id,name,kind) VALUES($1,$2,'Fidelity','checking') RETURNING id`, ws, user).Scan(&acct); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO finance_rules(workspace_id,user_id,pattern,category) VALUES
		($1,$2,'DIRECT DEBIT','Transfer'), ($1,$2,'POUND STERLING','Dining'), ($1,$2,'NETFLIX COM','Subscriptions')`, ws, user); err != nil {
		t.Fatal(err)
	}
	// Merchants as the code before 00112 stored them.
	rows := []struct {
		description, oldMerchant string
		category, source         *string
		wantCategory             *string
	}{
		{"DIRECT DEBIT VENMO PAYMENT (Cash)", "DIRECT DEBIT VENMO PAYMENT CASH", ptr("Transfer"), ptr("rule"), nil},
		{"DIRECT DEBIT ELAN WEB PYMT (Cash)", "DIRECT DEBIT ELAN WEB PYMT CASH", ptr("Transfer"), ptr("person"), ptr("Transfer")},
		{"DIRECT DEBIT ACH OPB Membership8002418123", "DIRECT DEBIT ACH OPB", nil, nil, nil},
		{"DIRECT DEPOSIT RVO HEALTH PAYROLL (Cash)", "DIRECT DEPOSIT RVO HEALTH PAYROLL CASH", ptr("Income"), ptr("default"), ptr("Income")},
		{"4.55 POUND STERLING CAFFE NERO OXFORD BLACKWE", "POUND STERLING CAFFE NERO OXFORD BLACKWE", ptr("Dining"), ptr("rule"), nil},
		{"POUND STERLING SHOP", "POUND STERLING SHOP", nil, nil, nil},
		{"NETFLIX.COM", "NETFLIX COM", ptr("Subscriptions"), ptr("rule"), ptr("Subscriptions")},
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		if err := pool.QueryRow(ctx, `INSERT INTO finance_transactions(account_id,posted_on,amount_cents,description,merchant,category,category_source,dedupe_key)
			VALUES($1,'2026-10-20',-100,$2,$3,$4,$5,$6) RETURNING id`, acct, r.description, r.oldMerchant, r.category, r.source, uuid.NewString()).Scan(&ids[i]); err != nil {
			t.Fatal(err)
		}
	}

	if err := goose.UpContext(ctx, sqldb, "."); err != nil {
		t.Fatal(err)
	}
	for i, r := range rows {
		var merchant string
		var category *string
		if err := pool.QueryRow(ctx, `SELECT merchant, category FROM finance_transactions WHERE id = $1`, ids[i]).Scan(&merchant, &category); err != nil {
			t.Fatal(err)
		}
		if want := finance.Merchant(r.description); merchant != want {
			t.Errorf("%q: merchant %q, want %q as an import computes it", r.description, merchant, want)
		}
		if deref(category) != deref(r.wantCategory) {
			t.Errorf("%q: category %q, want %q", r.description, deref(category), deref(r.wantCategory))
		}
	}
	var patterns []string
	if err := pool.QueryRow(ctx, `SELECT array_agg(pattern ORDER BY pattern) FROM finance_rules`).Scan(&patterns); err != nil || len(patterns) != 1 || patterns[0] != "NETFLIX COM" {
		t.Fatalf("rules after migrating %v %v, want only NETFLIX COM", patterns, err)
	}
}

func ptr(s string) *string { return &s }

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
