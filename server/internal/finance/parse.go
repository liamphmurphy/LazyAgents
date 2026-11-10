package finance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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
