package finance

import (
	"errors"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
)

// Layouts header detection now knows beyond the basics: semicolons with
// decimal commas and day-first dates, and an unsigned amount beside a
// DR/CR column.
func TestDetectCSVLayouts(t *testing.T) {
	euro := "Buchungstag;Date;Description;Amount\n" + // header detection finds "date", "description", "amount"
		"x;05.11.2026;REWE MARKT;-12,50\nx;31.10.2026;GEHALT;2.400,00\n"
	m, ok := DetectCSV([]byte(euro))
	if !ok || m.Delimiter != ";" || m.DateFormat != "DD.MM.YYYY" || !m.DecimalComma || m.Date != 1 {
		t.Fatalf("euro %+v %v", m, ok)
	}
	p, err := ParseCSVWith([]byte(euro), m)
	if err != nil || p.Txns[0].AmountCents != -1250 || p.Txns[1].AmountCents != 240000 || !p.Txns[1].PostedOn.Equal(date("2026-10-31")) {
		t.Fatalf("euro parse %+v %v", p.Txns, err)
	}

	drcr := "Date,Description,Amount,Type\n2026-11-05,COFFEE,4.50,DR\n2026-11-01,PAYROLL,3400.00,CR\n"
	m, ok = DetectCSV([]byte(drcr))
	if !ok || m.Direction == nil || *m.Direction != 3 || len(m.DebitValues) != 1 || m.DebitValues[0] != "DR" {
		t.Fatalf("drcr %+v %v", m, ok)
	}
	p, err = ParseCSVWith([]byte(drcr), m)
	if err != nil || p.Txns[0].AmountCents != -450 || p.Txns[1].AmountCents != 340000 {
		t.Fatalf("drcr parse %+v %v", p.Txns, err)
	}

	// Day-first is only chosen when a day is over 12.
	if f := guessDateFormat([]string{"05/11/2026", "11/05/2026"}); f != "MM/DD/YYYY" {
		t.Errorf("ambiguous: %s", f)
	}
	if f := guessDateFormat([]string{"05/11/2026", "25/10/2026"}); f != "DD/MM/YYYY" {
		t.Errorf("day-first: %s", f)
	}
	if f := guessDateFormat([]string{"20261105", "2026-11-05 13:02:11"}); f != "" {
		t.Errorf("mixed formats: %s", f)
	}
	if _, ok := parseDate("2026-11-05T13:02:11Z", "YYYY-MM-DD"); !ok {
		t.Error("a timestamp's date")
	}
}

// A layout detection doesn't know: the guess prefills the mapping step, and
// a correct mapping (what the assistant proposes) reads it.
func TestUnknownLayoutGuessAndMapping(t *testing.T) {
	odd := "Statement for account ****0921\n\nTxn Dt|Payee/Memo|Ref|Value|DR/CR\n" +
		"20261105|SQ *POWELLS BOOKS|A1|34.18|D\n20261104|PAYMENT RECEIVED|A2|1200.00|C\n"
	if _, ok := DetectCSV([]byte(odd)); ok {
		t.Fatal("unknown headers shouldn't be detected")
	}
	if _, err := ParseCSV([]byte(odd), false); err == nil {
		t.Fatal("ParseCSV should refuse an unknown layout")
	}
	g := GuessCSV([]byte(odd))
	if g.Delimiter != "|" || g.HeaderRow != 2 || g.Date != 0 || g.DateFormat != "YYYYMMDD" || g.Amount == nil || *g.Amount != 3 || len(g.Description) != 1 || g.Description[0] != 1 {
		t.Fatalf("guess %+v amount %v", g, g.Amount)
	}
	// The D/C column is recognized as the direction, so the guess already reads signs right.
	if g.Direction == nil || *g.Direction != 4 || len(g.DebitValues) != 1 || g.DebitValues[0] != "D" {
		t.Fatalf("guess direction %v %v", g.Direction, g.DebitValues)
	}
	p, err := ParseCSVWith([]byte(odd), g)
	if err != nil || len(p.Txns) != 2 || p.Txns[0].AmountCents != -3418 || p.Txns[1].AmountCents != 120000 || p.Txns[0].Description != "SQ *POWELLS BOOKS" {
		t.Fatalf("mapped %+v %v", p.Txns, err)
	}
	s, err := SampleCSV([]byte(odd), "|", 5)
	if err != nil || len(s.Rows) != 4 || s.Columns != 5 {
		t.Fatalf("sample %+v %v", s, err)
	}
	if CSVFingerprint([]byte(odd)) != CSVFingerprint([]byte(odd+"20261101|X|A3|1.00|D\n")) || CSVFingerprint([]byte(odd)) == CSVFingerprint([]byte("Date,Description,Amount\n")) {
		t.Error("fingerprint follows the first line only")
	}
}

func TestMappingValidation(t *testing.T) {
	base := func() CSVMapping {
		return CSVMapping{Delimiter: ",", HeaderRow: 1, Date: 0, DateFormat: "YYYY-MM-DD", Description: []int{1}, Amount: ptr(2)}
	}
	var ve *apperr.ValidationError
	for name, c := range map[string]struct {
		edit  func(*CSVMapping)
		field string
	}{
		"delimiter":   {func(m *CSVMapping) { m.Delimiter = "#" }, "delimiter"},
		"date format": {func(m *CSVMapping) { m.DateFormat = "whenever" }, "date_format"},
		"column":      {func(m *CSVMapping) { m.Date = 9 }, "date"},
		"no desc":     {func(m *CSVMapping) { m.Description = nil }, "description"},
		"both modes":  {func(m *CSVMapping) { m.Debit, m.Credit = ptr(1), ptr(2) }, "amount"},
		"half split":  {func(m *CSVMapping) { m.Amount, m.Debit = nil, ptr(2) }, "debit"},
		"direction":   {func(m *CSVMapping) { m.Direction = ptr(1) }, "debit_values"},
	} {
		m := base()
		c.edit(&m)
		if err := m.Validate(3); !errors.As(err, &ve) || ve.Field != c.field {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := base().Validate(3); err != nil {
		t.Error(err)
	}
	if _, err := ParseCSVWith([]byte("a,b,c\n"), base()); err == nil || !strings.Contains(err.Error(), "no transactions") {
		t.Errorf("header only: %v", err)
	}
}
