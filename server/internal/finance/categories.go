// Package finance is the Finance app: a person's accounts and transactions,
// imported from their bank's CSV or OFX exports, categorized by their rules,
// and summarized for the dashboard and the assistant's finance_* tools.
//
// Amounts are integer cents; negative is money out of an account. Transfers
// between the person's own accounts are categorized Transfer and left out of
// income and spending, so a credit-card payment isn't counted twice.
package finance

import (
	"slices"
	"strings"
)

const (
	Transfer      = "Transfer"
	Uncategorized = "Uncategorized"
	Income        = "Income"
)

// Categories is the fixed set, in the order pickers list them. A small fixed
// set keeps months comparable; rules give the person the detail.
var Categories = []string{"Income", "Housing", "Utilities", "Groceries", "Dining", "Transport", "Travel", "Shopping", "Entertainment",
	"Health", "Subscriptions", "Household", "Gifts", "Fees", "Other", Transfer}

// CanonicalCategory matches a category name case-insensitively.
func CanonicalCategory(s string) (string, bool) {
	s = strings.TrimSpace(s)
	i := slices.IndexFunc(Categories, func(c string) bool { return strings.EqualFold(c, s) })
	if i < 0 {
		return "", false
	}
	return Categories[i], true
}

// defaultKeywords categorize a transaction before the person has rules for
// it: whole words (or phrases) found in its merchant. Checked in order, so
// the more specific lists come first. Transfer is never a default: a card
// payment whose other side isn't imported is the only trace of that card's
// spending, so only a match or the person makes something a transfer.
var defaultKeywords = []struct {
	category string
	inflow   bool // only money in
	outflow  bool // only money out
	words    []string
}{
	{Income, true, false, []string{"PAYROLL", "DIRECT DEP", "DIRECT DEPOSIT", "SALARY", "INTEREST PAID", "INTEREST PAYMENT", "DIVIDEND"}},
	{"Subscriptions", false, true, []string{"NETFLIX", "SPOTIFY", "HULU", "DISNEY PLUS", "YOUTUBE PREMIUM", "ICLOUD", "APPLE COM BILL", "PATREON", "ADOBE", "OPENAI", "ANTHROPIC", "GITHUB", "FASTMAIL", "PROTON"}},
	{"Groceries", false, true, []string{"SAFEWAY", "KROGER", "FRED MEYER", "NEW SEASONS", "TRADER JOE", "WHOLE FOODS", "WHOLEFDS", "COSTCO", "ALBERTSONS", "WINCO", "GROCERY", "MARKET OF CHOICE"}},
	{"Dining", false, true, []string{"RESTAURANT", "CAFE", "COFFEE", "STARBUCKS", "DOORDASH", "GRUBHUB", "UBER EATS", "PIZZA", "TAQUERIA", "BAKERY", "BREWING", "CHIPOTLE"}},
	{"Transport", false, true, []string{"UBER", "LYFT", "TRIMET", "SHELL", "CHEVRON", "ARCO", "EXXON", "PARKING", "BIKETOWN", "TESLA SUPERCHARGER"}},
	{"Travel", false, true, []string{"AIRLINES", "AIRLINE", "AIRBNB", "HOTEL", "MARRIOTT", "HILTON", "HYATT", "EXPEDIA", "AMTRAK", "DELTA AIR", "ALASKA AIR", "UNITED AIR"}},
	{"Utilities", false, true, []string{"COMCAST", "XFINITY", "PGE", "PORTLAND GENERAL", "NW NATURAL", "WATER BUREAU", "VERIZON", "T MOBILE", "ATT"}},
	{"Health", false, true, []string{"PHARMACY", "CVS", "WALGREENS", "DENTAL", "CLINIC", "KAISER", "PROVIDENCE"}},
	{"Shopping", false, true, []string{"AMAZON", "AMZN", "TARGET", "BEST BUY", "REI", "IKEA", "ETSY", "EBAY"}},
	{"Entertainment", false, true, []string{"STEAM", "STEAMGAMES", "NINTENDO", "PLAYSTATION", "CINEMA", "THEATER", "TICKETMASTER"}},
	{"Housing", false, true, []string{"RENT", "MORTGAGE", "HOA"}},
	{"Fees", false, true, []string{"FEE", "INTEREST CHARGE", "OVERDRAFT"}},
}

// DefaultCategory is the built-in guess for a merchant, if any.
func DefaultCategory(merchant string, amountCents int64) (string, bool) {
	padded := " " + merchant + " "
	for _, k := range defaultKeywords {
		if (k.inflow && amountCents < 0) || (k.outflow && amountCents > 0) {
			continue
		}
		for _, w := range k.words {
			if strings.Contains(padded, " "+w+" ") {
				return k.category, true
			}
		}
	}
	return "", false
}

// transferish is whether a merchant reads like money moving between accounts.
// Transfer matching needs it on one side, so a $40 dinner doesn't pair with
// an unrelated $40 deposit.
func transferish(merchant string) bool {
	padded := " " + merchant + " "
	for _, w := range []string{"PAYMENT", "PMT", "TRANSFER", "XFER", "AUTOPAY", "ONLINE BANKING"} {
		if strings.Contains(padded, " "+w+" ") {
			return true
		}
	}
	return false
}

// Suggest is up to three categories for a transaction in review: how the
// person categorized merchants with the same first word, then the built-in
// guess, then common fallbacks for its direction.
func Suggest(merchant string, amountCents int64, learned map[string][]string) []string {
	var out []string
	add := func(c string) {
		if c != "" && len(out) < 3 && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	first, _, _ := strings.Cut(merchant, " ")
	for _, c := range learned[first] {
		add(c)
	}
	if transferish(merchant) {
		add(Transfer)
	}
	if c, ok := DefaultCategory(merchant, amountCents); ok {
		add(c)
	}
	fallback := []string{"Shopping", "Dining", "Other"}
	if amountCents > 0 {
		fallback = []string{Income, Transfer, "Other"}
	}
	for _, c := range fallback {
		add(c)
	}
	return out
}
