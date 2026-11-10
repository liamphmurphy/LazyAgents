package finance

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
)

// MaxImportRows bounds one file: years of a busy account, not a data dump.
const MaxImportRows = 20000

// Parsed is one statement file: its transactions and, when the file says,
// which account it is and the balance at its end.
type Parsed struct {
	Format string // csv or ofx
	// OFX only: the bank's account id, its type and whether it's a card.
	AccountID   string
	AccountType string
	Credit      bool
	Balance     *Balance
	Txns        []Txn
}

type Balance struct {
	AsOf  time.Time
	Cents int64
}

type Txn struct {
	PostedOn    time.Time
	AmountCents int64
	Description string
	// DedupeKey is the same every time the bank exports this transaction.
	DedupeKey string
}

// LooksLikeOFX tells an OFX/QFX file from a CSV by its name or its header.
func LooksLikeOFX(filename string, data []byte) bool {
	name := strings.ToLower(filename)
	if strings.HasSuffix(name, ".ofx") || strings.HasSuffix(name, ".qfx") {
		return true
	}
	head := data[:min(len(data), 2048)]
	return bytes.Contains(bytes.ToUpper(head), []byte("<OFX>")) || bytes.Contains(head, []byte("OFXHEADER"))
}

// ParseCents reads a money amount as banks write it: "1,234.56", "-12",
// "(12.00)", "$5.10", "12.00-".
func ParseCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	neg := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg, s = true, s[1:len(s)-1]
	}
	if strings.HasSuffix(s, "-") {
		neg, s = true, strings.TrimSuffix(s, "-")
	}
	s = strings.NewReplacer("$", "", ",", "", " ", "", " ", "").Replace(s)
	if strings.HasPrefix(s, "-") {
		neg, s = !neg, s[1:]
	} else {
		s = strings.TrimPrefix(s, "+")
	}
	if s == "" {
		return 0, errors.New("empty amount")
	}
	whole, frac, _ := strings.Cut(s, ".")
	if len(frac) > 2 {
		// Some exports carry four decimals; round half away from zero.
		round := frac[2] >= '5'
		frac = frac[:2]
		c, err := centsOf(whole, frac)
		if err != nil {
			return 0, err
		}
		if round {
			c++
		}
		if neg {
			c = -c
		}
		return c, nil
	}
	c, err := centsOf(whole, frac)
	if err != nil {
		return 0, err
	}
	if neg {
		c = -c
	}
	return c, nil
}

func centsOf(whole, frac string) (int64, error) {
	for len(frac) < 2 {
		frac += "0"
	}
	if whole == "" {
		whole = "0"
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("amount %q: %w", whole+"."+frac, err)
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("amount %q: %w", whole+"."+frac, err)
	}
	return w*100 + f, nil
}

var ofxTag = regexp.MustCompile(`<(/?)([A-Za-z0-9.]+)>([^<]*)`)

// ParseOFX reads an OFX or QFX statement, SGML (1.x) or XML (2.x). It is
// deliberately forgiving: it reads the tags it needs and ignores the rest,
// since banks bend the spec in many small ways.
func ParseOFX(data []byte) (Parsed, error) {
	out := Parsed{Format: "ofx"}
	var (
		txn      map[string]string
		inLedger bool
		ledger   = map[string]string{}
		accounts []string
	)
	for _, m := range ofxTag.FindAllSubmatch(data, -1) {
		closing, tag, value := len(m[1]) > 0, strings.ToUpper(string(m[2])), strings.TrimSpace(string(m[3]))
		switch {
		case tag == "STMTTRN" && !closing:
			txn = map[string]string{}
		case tag == "STMTTRN" && closing:
			if txn != nil {
				t, err := ofxTxn(txn)
				if err != nil {
					return out, err
				}
				out.Txns = append(out.Txns, t)
				if len(out.Txns) > MaxImportRows {
					return out, apperr.Invalid("file", "has more than %d transactions; export a shorter date range", MaxImportRows)
				}
			}
			txn = nil
		case tag == "LEDGERBAL":
			inLedger = !closing
		case tag == "CCSTMTRS" && !closing:
			out.Credit = true
		case closing:
		case txn != nil:
			txn[tag] = value
		case inLedger:
			ledger[tag] = value
		case tag == "ACCTID":
			if !slices.Contains(accounts, value) {
				accounts = append(accounts, value)
			}
		case tag == "ACCTTYPE":
			out.AccountType = strings.ToLower(value)
		}
	}
	if len(accounts) > 1 {
		return out, apperr.Invalid("file", "holds %d accounts; export one account per file", len(accounts))
	}
	if len(accounts) == 1 {
		out.AccountID = accounts[0]
	}
	if out.AccountID == "" && len(out.Txns) == 0 {
		return out, apperr.Invalid("file", "isn't an OFX statement Vesper can read")
	}
	if ledger["BALAMT"] != "" && ledger["DTASOF"] != "" {
		cents, err := ParseCents(ledger["BALAMT"])
		at, derr := ofxDate(ledger["DTASOF"])
		if err == nil && derr == nil {
			out.Balance = &Balance{AsOf: at, Cents: cents}
		}
	}
	return out, nil
}

func ofxTxn(f map[string]string) (Txn, error) {
	at, err := ofxDate(f["DTPOSTED"])
	if err != nil {
		return Txn{}, apperr.Invalid("file", "a transaction's date %q can't be read", f["DTPOSTED"])
	}
	cents, err := ParseCents(f["TRNAMT"])
	if err != nil {
		return Txn{}, apperr.Invalid("file", "a transaction's amount %q can't be read", f["TRNAMT"])
	}
	desc := f["NAME"]
	if memo := f["MEMO"]; desc == "" || (memo != "" && !strings.Contains(desc, memo) && len(desc) < 12) {
		desc = strings.TrimSpace(desc + " " + memo)
	}
	t := Txn{PostedOn: at, AmountCents: cents, Description: desc}
	if id := f["FITID"]; id != "" {
		t.DedupeKey = "fitid:" + id
	} else {
		t.DedupeKey = hashKey(at, cents, desc, 0)
	}
	return t, nil
}

// ofxDate reads the date part of an OFX timestamp (YYYYMMDD…).
func ofxDate(s string) (time.Time, error) {
	if len(s) < 8 {
		return time.Time{}, errors.New("short date")
	}
	return time.Parse("20060102", s[:8])
}

func hashKey(at time.Time, cents int64, desc string, n int) string {
	h := sha256.Sum256(fmt.Appendf(nil, "%s|%d|%s|%d", at.Format(time.DateOnly), cents, strings.ToUpper(strings.Join(strings.Fields(desc), " ")), n))
	return "row:" + hex.EncodeToString(h[:12])
}

// CSV column names, most specific first.
var (
	dateHeaders    = []string{"posted date", "posting date", "post date", "date posted", "transaction date", "trans. date", "trans date", "date"}
	descHeaders    = []string{"description", "original description", "payee", "merchant", "name", "transaction description", "details", "memo"}
	amountHeaders  = []string{"amount", "transaction amount", "amount (usd)"}
	debitHeaders   = []string{"debit", "debits", "withdrawal", "withdrawals", "money out", "debit amount"}
	creditHeaders  = []string{"credit", "credits", "deposit", "deposits", "money in", "credit amount"}
	balanceHeaders = []string{"balance", "running balance", "running bal."}
)

var dateLayouts = []string{"01/02/2006", "1/2/2006", "2006-01-02", "01/02/06", "1/2/06", "2006/01/02", "Jan 2, 2006", "02 Jan 2006", "01-02-2006"}

// ParseCSV reads a bank's CSV export, finding its date, description and
// amount (or debit and credit) columns by their headers. flipSign is for
// card exports that show purchases as positive amounts.
func ParseCSV(data []byte, flipSign bool) (Parsed, error) {
	out := Parsed{Format: "csv"}
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	var cols *csvColumns
	seen := map[string]int{}
	var newest *Balance
	for line := 1; ; line++ {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return out, apperr.Invalid("file", "line %d isn't valid CSV", line)
		}
		if cols == nil {
			// Some banks put a few lines of account details above the header.
			if c, ok := findColumns(rec); ok {
				cols = &c
			} else if line > 10 {
				break
			}
			continue
		}
		if blank(rec) {
			continue
		}
		t, bal, err := cols.txn(rec, flipSign)
		if err != nil {
			return out, apperr.Invalid("file", "line %d: %s", line, err)
		}
		if t.AmountCents == 0 {
			continue // pending holds and $0 authorizations
		}
		k := t.PostedOn.Format(time.DateOnly) + "|" + strconv.FormatInt(t.AmountCents, 10) + "|" + t.Description
		t.DedupeKey = hashKey(t.PostedOn, t.AmountCents, t.Description, seen[k])
		seen[k]++
		out.Txns = append(out.Txns, t)
		if len(out.Txns) > MaxImportRows {
			return out, apperr.Invalid("file", "has more than %d transactions; export a shorter date range", MaxImportRows)
		}
		// Files list newest first or oldest first; the balance on the latest
		// date (its first row in the file) is the account's balance then.
		if bal != nil && (newest == nil || t.PostedOn.After(newest.AsOf)) {
			newest = &Balance{AsOf: t.PostedOn, Cents: *bal}
		}
	}
	if cols == nil {
		return out, apperr.Invalid("file", "has no header row with a date, a description and an amount (or debit and credit) column")
	}
	out.Balance = newest
	return out, nil
}

type csvColumns struct {
	date, desc, amount, debit, credit, balance int
}

func blank(rec []string) bool {
	return !slices.ContainsFunc(rec, func(s string) bool { return strings.TrimSpace(s) != "" })
}

func findColumns(header []string) (csvColumns, bool) {
	norm := make([]string, len(header))
	for i, h := range header {
		norm[i] = strings.ToLower(strings.TrimSpace(h))
	}
	find := func(names []string) int {
		for _, n := range names {
			if i := slices.Index(norm, n); i >= 0 {
				return i
			}
		}
		return -1
	}
	c := csvColumns{date: find(dateHeaders), desc: find(descHeaders), amount: find(amountHeaders), debit: find(debitHeaders), credit: find(creditHeaders), balance: find(balanceHeaders)}
	ok := c.date >= 0 && c.desc >= 0 && (c.amount >= 0 || (c.debit >= 0 && c.credit >= 0))
	return c, ok
}

func cell(rec []string, i int) string {
	if i < 0 || i >= len(rec) {
		return ""
	}
	return strings.TrimSpace(rec[i])
}

func (c csvColumns) txn(rec []string, flipSign bool) (Txn, *int64, error) {
	var t Txn
	ds := cell(rec, c.date)
	for _, layout := range dateLayouts {
		if at, err := time.Parse(layout, ds); err == nil {
			t.PostedOn = at
			break
		}
	}
	if t.PostedOn.IsZero() {
		return t, nil, fmt.Errorf("date %q can't be read", ds)
	}
	t.Description = strings.Join(strings.Fields(cell(rec, c.desc)), " ")
	if t.Description == "" {
		return t, nil, errors.New("has no description")
	}
	if c.amount >= 0 && cell(rec, c.amount) != "" {
		cents, err := ParseCents(cell(rec, c.amount))
		if err != nil {
			return t, nil, fmt.Errorf("amount %q can't be read", cell(rec, c.amount))
		}
		t.AmountCents = cents
	} else {
		// Separate columns hold positive numbers: debits leave, credits arrive.
		for _, side := range []struct {
			i    int
			sign int64
		}{{c.debit, -1}, {c.credit, 1}} {
			if v := cell(rec, side.i); v != "" {
				cents, err := ParseCents(v)
				if err != nil {
					return t, nil, fmt.Errorf("amount %q can't be read", v)
				}
				if cents < 0 {
					cents = -cents
				}
				t.AmountCents += side.sign * cents
			}
		}
	}
	if flipSign {
		t.AmountCents = -t.AmountCents
	}
	var bal *int64
	if v := cell(rec, c.balance); v != "" {
		if cents, err := ParseCents(v); err == nil {
			bal = &cents
		}
	}
	return t, bal, nil
}
