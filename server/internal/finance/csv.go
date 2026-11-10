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

// CSVMapping says how to read one bank's CSV layout. It is data, not code:
// whoever proposes it (header detection, the assistant, the person in the
// import dialog), the same parser applies it, and a mapping that doesn't
// read every row of the file is refused rather than imported half right.
type CSVMapping struct {
	// Delimiter is ",", ";", "\t" or "|".
	Delimiter string `json:"delimiter"`
	// HeaderRow is the 1-based row (blank lines aren't rows) that holds the column names; rows up to
	// it (account details some banks put first) are skipped. 0: no header.
	HeaderRow int `json:"header_row"`
	// Columns are 0-based.
	Date       int    `json:"date"`
	DateFormat string `json:"date_format"`
	// Description joins these columns' text, e.g. a payee and a memo.
	Description []int `json:"description"`
	// Amounts come one of three ways: a signed Amount; an unsigned Amount
	// with a Direction column whose DebitValues mean money out; or separate
	// Debit (money out) and Credit (money in) columns.
	Amount      *int     `json:"amount,omitempty"`
	Direction   *int     `json:"direction,omitempty"`
	DebitValues []string `json:"debit_values,omitempty"`
	Debit       *int     `json:"debit,omitempty"`
	Credit      *int     `json:"credit,omitempty"`
	// Balance is a running balance column; its newest value becomes the account's balance.
	Balance *int `json:"balance,omitempty"`
	// DecimalComma reads 1.234,56 as one thousand two hundred thirty-four.
	DecimalComma bool `json:"decimal_comma,omitempty"`
	// FlipSign is for card exports that show purchases as positive.
	FlipSign bool `json:"flip_sign,omitempty"`
}

// DateFormats are the date layouts a mapping can name, each with the Go
// layouts it accepts (with or without leading zeros).
var DateFormats = map[string][]string{
	"MM/DD/YYYY":  {"01/02/2006", "1/2/2006", "01-02-2006", "1-2-2006"},
	"DD/MM/YYYY":  {"02/01/2006", "2/1/2006", "02-01-2006", "2-1-2006"},
	"YYYY-MM-DD":  {"2006-01-02", "2006/01/02", "2006-1-2", "2006/1/2"},
	"MM/DD/YY":    {"01/02/06", "1/2/06"},
	"DD/MM/YY":    {"02/01/06", "2/1/06"},
	"DD.MM.YYYY":  {"02.01.2006", "2.1.2006"},
	"YYYYMMDD":    {"20060102"},
	"MMM D, YYYY": {"Jan 2, 2006", "January 2, 2006", "Jan 2 2006"},
	"D MMM YYYY":  {"2 Jan 2006", "02 Jan 2006", "2-Jan-2006", "02-Jan-2006", "2 January 2006"},
}

// dateFormatOrder is how detection tries them: unambiguous first, then US
// month-first ahead of day-first (it only wins when no day is over 12).
var dateFormatOrder = []string{"YYYY-MM-DD", "YYYYMMDD", "MM/DD/YYYY", "DD/MM/YYYY", "MM/DD/YY", "DD/MM/YY", "DD.MM.YYYY", "MMM D, YYYY", "D MMM YYYY"}

var delimiters = []string{",", ";", "\t", "|"}

// DateFormatNames lists DateFormats in the order pickers show them.
func DateFormatNames() []string { return slices.Clone(dateFormatOrder) }

func parseDate(s, format string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	// Some exports add a time: 2026-11-05 13:02:11 or 2026-11-05T13:02:11Z.
	candidates := []string{s}
	if i := strings.IndexAny(s, " T"); i > 0 && strings.ContainsAny(s[i:], ":") {
		candidates = append(candidates, s[:i])
	}
	for _, c := range candidates {
		for _, layout := range DateFormats[format] {
			if at, err := time.Parse(layout, c); err == nil {
				return at, true
			}
		}
	}
	return time.Time{}, false
}

// parseAmount reads a cell as cents, with a decimal comma when asked.
func parseAmount(s string, decimalComma bool) (int64, error) {
	if decimalComma {
		s = strings.NewReplacer(".", "", " ", "", " ", "").Replace(s)
		s = strings.Replace(s, ",", ".", 1)
	}
	return ParseCents(s)
}

// Validate checks a mapping's shape against a file with ncols columns.
func (m CSVMapping) Validate(ncols int) error {
	if !slices.Contains(delimiters, m.Delimiter) {
		return apperr.Invalid("delimiter", `must be one of "," ";" "\t" "|"`)
	}
	if m.HeaderRow < 0 || m.HeaderRow > 20 {
		return apperr.Invalid("header_row", "must be 0 (no header) to 20")
	}
	if _, ok := DateFormats[m.DateFormat]; !ok {
		return apperr.Invalid("date_format", "must be one of %s", strings.Join(dateFormatOrder, ", "))
	}
	col := func(field string, i int) error {
		if i < 0 || i >= ncols {
			return apperr.Invalid(field, "column %d doesn't exist: the file has columns 0–%d", i, ncols-1)
		}
		return nil
	}
	if err := col("date", m.Date); err != nil {
		return err
	}
	if len(m.Description) == 0 || len(m.Description) > 3 {
		return apperr.Invalid("description", "name 1–3 columns")
	}
	for _, i := range m.Description {
		if err := col("description", i); err != nil {
			return err
		}
	}
	signed, split := m.Amount != nil, m.Debit != nil || m.Credit != nil
	switch {
	case signed == split:
		return apperr.Invalid("amount", "give either amount (with an optional direction) or debit and credit columns")
	case signed:
		if err := col("amount", *m.Amount); err != nil {
			return err
		}
		if m.Direction != nil {
			if err := col("direction", *m.Direction); err != nil {
				return err
			}
			if len(m.DebitValues) == 0 {
				return apperr.Invalid("debit_values", "list the direction column's values that mean money out, e.g. DR or Debit")
			}
		}
	default:
		if m.Debit == nil || m.Credit == nil {
			return apperr.Invalid("debit", "give both debit and credit columns")
		}
		if err := col("debit", *m.Debit); err != nil {
			return err
		}
		if err := col("credit", *m.Credit); err != nil {
			return err
		}
	}
	if m.Balance != nil {
		if err := col("balance", *m.Balance); err != nil {
			return err
		}
	}
	return nil
}

func csvReader(data []byte, delimiter string) *csv.Reader {
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))))
	r.Comma = []rune(delimiter)[0]
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	return r
}

// records reads up to limit records (all with limit < 0).
func records(data []byte, delimiter string, limit int) ([][]string, error) {
	r := csvReader(data, delimiter)
	var out [][]string
	for line := 1; limit < 0 || len(out) < limit; line++ {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return out, apperr.Invalid("file", "line %d isn't valid CSV", line)
		}
		out = append(out, rec)
	}
	return out, nil
}

// SniffDelimiter is the delimiter that splits the first lines most consistently into the most columns.
func SniffDelimiter(data []byte) string {
	best, bestScore := ",", -1
	for _, d := range delimiters {
		recs, _ := records(data, d, 8)
		counts := map[int]int{}
		for _, r := range recs {
			if !blank(r) {
				counts[len(r)]++
			}
		}
		// Score: the most common width, weighted by how many rows share it.
		score := 0
		for width, n := range counts {
			if width > 1 && width*n > score {
				score = width * n
			}
		}
		if score > bestScore {
			best, bestScore = d, score
		}
	}
	return best
}

// ParseCSVWith reads a CSV with a mapping. Every non-blank row after the
// header must read; the first one that doesn't is the error, with its line.
func ParseCSVWith(data []byte, m CSVMapping) (Parsed, error) {
	out := Parsed{Format: "csv"}
	recs, err := records(data, m.Delimiter, -1)
	if err != nil {
		return out, err
	}
	ncols := 0
	for _, r := range recs[min(m.HeaderRow, len(recs)):] {
		ncols = max(ncols, len(r))
	}
	if m.HeaderRow > 0 && m.HeaderRow <= len(recs) {
		ncols = max(ncols, len(recs[m.HeaderRow-1]))
	}
	if err := m.Validate(ncols); err != nil {
		return out, err
	}
	debits := map[string]bool{}
	for _, v := range m.DebitValues {
		debits[strings.ToLower(strings.TrimSpace(v))] = true
	}
	seen := map[string]int{}
	var newest *Balance
	for i := m.HeaderRow; i < len(recs); i++ {
		rec, row := recs[i], i+1
		if blank(rec) {
			continue
		}
		t, bal, err := m.txn(rec, debits)
		if err != nil {
			return out, apperr.Invalid("file", "row %d: %s", row, err)
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
	if len(out.Txns) == 0 {
		return out, apperr.Invalid("file", "has no transactions after the header")
	}
	out.Balance = newest
	return out, nil
}

func (m CSVMapping) txn(rec []string, debits map[string]bool) (Txn, *int64, error) {
	var t Txn
	ds := cell(rec, m.Date)
	at, ok := parseDate(ds, m.DateFormat)
	if !ok {
		return t, nil, fmt.Errorf("date %q isn't %s", ds, m.DateFormat)
	}
	t.PostedOn = at
	var parts []string
	for _, i := range m.Description {
		if v := strings.Join(strings.Fields(cell(rec, i)), " "); v != "" && !slices.Contains(parts, v) {
			parts = append(parts, v)
		}
	}
	t.Description = strings.Join(parts, " ")
	if t.Description == "" {
		return t, nil, errors.New("has no description")
	}
	amount := func(field string, i int) (int64, error) {
		v := cell(rec, i)
		if v == "" {
			return 0, nil
		}
		c, err := parseAmount(v, m.DecimalComma)
		if err != nil {
			return 0, fmt.Errorf("%s %q isn't an amount", field, v)
		}
		return c, nil
	}
	if m.Amount != nil {
		c, err := amount("amount", *m.Amount)
		if err != nil {
			return t, nil, err
		}
		if m.Direction != nil {
			if c < 0 {
				c = -c
			}
			if debits[strings.ToLower(cell(rec, *m.Direction))] {
				c = -c
			}
		}
		t.AmountCents = c
	} else {
		// Separate columns hold positive numbers: debits leave, credits arrive.
		out, err := amount("debit", *m.Debit)
		if err != nil {
			return t, nil, err
		}
		in, err := amount("credit", *m.Credit)
		if err != nil {
			return t, nil, err
		}
		t.AmountCents = abs(in) - abs(out)
	}
	if m.FlipSign {
		t.AmountCents = -t.AmountCents
	}
	var bal *int64
	if m.Balance != nil {
		if v := cell(rec, *m.Balance); v != "" {
			if c, err := parseAmount(v, m.DecimalComma); err == nil {
				bal = &c
			}
		}
	}
	return t, bal, nil
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func blank(rec []string) bool {
	return !slices.ContainsFunc(rec, func(s string) bool { return strings.TrimSpace(s) != "" })
}

func cell(rec []string, i int) string {
	if i < 0 || i >= len(rec) {
		return ""
	}
	return strings.TrimSpace(rec[i])
}

// CSV column names header detection knows, most specific first.
var (
	dateHeaders      = []string{"posted date", "posting date", "post date", "date posted", "transaction date", "trans. date", "trans date", "date"}
	descHeaders      = []string{"description", "original description", "payee", "merchant", "name", "transaction description", "details", "memo"}
	amountHeaders    = []string{"amount", "transaction amount", "amount (usd)"}
	debitHeaders     = []string{"debit", "debits", "withdrawal", "withdrawals", "money out", "debit amount"}
	creditHeaders    = []string{"credit", "credits", "deposit", "deposits", "money in", "credit amount"}
	balanceHeaders   = []string{"balance", "running balance", "running bal."}
	directionHeaders = []string{"credit/debit", "debit/credit", "dr/cr", "cr/dr", "transaction type", "type"}
	debitWords       = []string{"debit", "dr", "d", "withdrawal", "purchase", "sale"}
	creditWords      = []string{"credit", "cr", "c", "deposit", "payment", "refund", "return"}
)

// DetectCSV is the mapping for a CSV whose header names Vesper knows. ok is
// false when it doesn't recognize the layout: then the person (with the
// assistant's help) maps it.
func DetectCSV(data []byte) (CSVMapping, bool) {
	m := CSVMapping{Delimiter: SniffDelimiter(data)}
	recs, err := records(data, m.Delimiter, 60)
	if err != nil {
		return m, false
	}
	// Some banks put a few lines of account details above the header.
	for i, rec := range recs[:min(len(recs), 10)] {
		norm := make([]string, len(rec))
		for j, h := range rec {
			norm[j] = strings.ToLower(strings.TrimSpace(h))
		}
		find := func(names []string) *int {
			for _, n := range names {
				if j := slices.Index(norm, n); j >= 0 {
					return &j
				}
			}
			return nil
		}
		date, desc := find(dateHeaders), find(descHeaders)
		amount, debit, credit := find(amountHeaders), find(debitHeaders), find(creditHeaders)
		if date == nil || desc == nil || (amount == nil && (debit == nil || credit == nil)) {
			continue
		}
		m.HeaderRow, m.Date, m.Description, m.Balance = i+1, *date, []int{*desc}, find(balanceHeaders)
		if amount != nil {
			m.Amount = amount
		} else {
			m.Debit, m.Credit = debit, credit
		}
		body := recs[i+1:]
		m.DateFormat = guessDateFormat(column(body, m.Date))
		if m.DateFormat == "" {
			return m, false
		}
		amounts := column(body, *firstNonNil(m.Amount, m.Debit))
		m.DecimalComma = decimalComma(amounts)
		// An unsigned amount next to a debit/credit indicator column.
		if m.Amount != nil && allNonNegative(amounts, m.DecimalComma) {
			if dir := find(directionHeaders); dir != nil && *dir != *m.Amount {
				values := distinct(column(body, *dir))
				var debitValues []string
				for _, v := range values {
					if slices.Contains(debitWords, strings.ToLower(v)) {
						debitValues = append(debitValues, v)
					}
				}
				if len(debitValues) > 0 {
					m.Direction, m.DebitValues = dir, debitValues
				}
			}
		}
		return m, true
	}
	return m, false
}

func firstNonNil(ps ...*int) *int {
	for _, p := range ps {
		if p != nil {
			return p
		}
	}
	return nil
}

// column is one column's non-empty values.
func column(recs [][]string, i int) []string {
	var out []string
	for _, r := range recs {
		if v := cell(r, i); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func distinct(values []string) []string {
	var out []string
	for _, v := range values {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// guessDateFormat is the first format every value parses in, or "".
func guessDateFormat(values []string) string {
	if len(values) == 0 {
		return ""
	}
	for _, f := range dateFormatOrder {
		ok := true
		for _, v := range values {
			if _, good := parseDate(v, f); !good {
				ok = false
				break
			}
		}
		if ok {
			return f
		}
	}
	return ""
}

var commaDecimal = regexp.MustCompile(`^[-+(]?[^,]*,\d{2}\)?-?$`)

// decimalComma is whether amounts are written 1.234,56: most have a comma
// before exactly two final digits and no dot after it.
func decimalComma(values []string) bool {
	n := 0
	for _, v := range values {
		if commaDecimal.MatchString(strings.TrimSpace(v)) && !strings.Contains(v[strings.LastIndex(v, ","):], ".") {
			n++
		}
	}
	return len(values) > 0 && n*2 > len(values)
}

func allNonNegative(values []string, decimalComma bool) bool {
	for _, v := range values {
		if c, err := parseAmount(v, decimalComma); err != nil || c < 0 {
			return false
		}
	}
	return len(values) > 0
}

// ParseCSV reads a CSV whose header Vesper recognizes (see DetectCSV).
// flipSign is for card exports that show purchases as positive amounts.
func ParseCSV(data []byte, flipSign bool) (Parsed, error) {
	m, ok := DetectCSV(data)
	if !ok {
		return Parsed{Format: "csv"}, apperr.Invalid("file", "has no header row with a date, a description and an amount (or debit and credit) column")
	}
	m.FlipSign = flipSign
	return ParseCSVWith(data, m)
}

// CSVSample is what the mapping step shows (and the assistant reads): the
// header's cells and the first rows, as the file has them.
type CSVSample struct {
	Delimiter string
	Rows      [][]string // the first records of the file, header included
	Columns   int
}

func SampleCSV(data []byte, delimiter string, n int) (CSVSample, error) {
	recs, err := records(data, delimiter, n)
	if err != nil {
		return CSVSample{}, err
	}
	s := CSVSample{Delimiter: delimiter, Rows: recs}
	for _, r := range recs {
		s.Columns = max(s.Columns, len(r))
	}
	return s, nil
}

// GuessCSV is a best-effort mapping for a layout DetectCSV didn't know, to
// prefill the mapping step: the header is the first row followed by a row
// with a date; the date column is the one that parses as dates; amounts are
// money-looking columns; the description is the column with the most text.
// It may well be wrong: the person or the assistant fixes it.
func GuessCSV(data []byte) CSVMapping {
	m := CSVMapping{Delimiter: SniffDelimiter(data), DateFormat: "MM/DD/YYYY", Description: []int{}}
	recs, _ := records(data, m.Delimiter, 40)
	if len(recs) == 0 {
		return m
	}
	width := 0
	for _, r := range recs {
		width = max(width, len(r))
	}
	// The first row with a date in some column starts the data.
	start := -1
	for i, r := range recs {
		for j := range r {
			if guessDateFormat([]string{cell(r, j)}) != "" {
				start = i
				break
			}
		}
		if start >= 0 {
			break
		}
	}
	if start < 0 {
		return m
	}
	m.HeaderRow = start
	body := recs[start:]
	textLen := map[int]int{}
	dateCol := -1
	for j := 0; j < width; j++ {
		vals := column(body, j)
		if len(vals) == 0 {
			continue
		}
		if f := guessDateFormat(vals); f != "" {
			if dateCol < 0 {
				dateCol, m.Date, m.DateFormat = j, j, f
			}
			continue
		}
		comma := decimalComma(vals)
		money := 0
		for _, v := range vals {
			if _, err := parseAmount(v, comma); err == nil {
				money++
			}
		}
		if money == len(vals) {
			if m.Amount == nil {
				jj := j
				m.Amount, m.DecimalComma = &jj, comma
			}
			continue
		}
		for _, v := range vals {
			textLen[j] += len(v)
		}
	}
	// An unsigned amount beside a column of D/C-style markers: read it as the direction.
	if m.Amount != nil && allNonNegative(column(body, *m.Amount), m.DecimalComma) {
		for j := 0; j < width; j++ {
			vals := distinct(column(body, j))
			if j == *m.Amount || len(vals) == 0 || len(vals) > 4 {
				continue
			}
			var debitValues []string
			marker := true
			for _, v := range vals {
				lv := strings.ToLower(v)
				switch {
				case slices.Contains(debitWords, lv):
					debitValues = append(debitValues, v)
				case !slices.Contains(creditWords, lv):
					marker = false
				}
			}
			if marker && len(debitValues) > 0 {
				jj := j
				m.Direction, m.DebitValues = &jj, debitValues
				delete(textLen, j)
				break
			}
		}
	}
	best, bestLen := -1, 0
	for j, n := range textLen {
		if n > bestLen || (n == bestLen && j < best) {
			best, bestLen = j, n
		}
	}
	if best >= 0 {
		m.Description = []int{best}
	}
	return m
}

// CSVFingerprint names a CSV layout before any mapping exists: its sniffed
// delimiter and its first row, which for most exports is the header (and
// for the rest, account details that are just as stable for one bank). A
// mapping saved under it is reused for the next export from the same bank.
func CSVFingerprint(data []byte) string {
	d := SniffDelimiter(data)
	recs, _ := records(data, d, 1)
	key := d + "\x1e"
	if len(recs) > 0 {
		for _, h := range recs[0] {
			key += strings.ToLower(strings.TrimSpace(h)) + "\x1f"
		}
	}
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:16])
}
