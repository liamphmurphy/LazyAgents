package finance

import (
	"slices"
	"strings"
	"time"
)

// Outflow is one charge, as recurring detection reads it.
type Outflow struct {
	Merchant    string
	Description string
	PostedOn    time.Time
	AmountCents int64 // negative
}

// Recurring is a charge that repeats about monthly.
type Recurring struct {
	Merchant    string
	Label       string
	AmountCents int64 // the latest charge, positive
	// PreviousCents is the price before the latest change, if it changed;
	// ChangedOn is the first charge at the new price.
	PreviousCents *int64
	ChangedOn     *time.Time
	DayOfMonth    int
	FirstSeen     time.Time
	LastSeen      time.Time
	Count         int
	// New: first charged within the last two months.
	New bool
}

const (
	minInterval = 26 // days between monthly charges: February to a 31-day month
	maxInterval = 35
	// A charge not seen for this long has stopped.
	staleAfter = 45 * 24 * time.Hour
	// More than this change between charges is a different purchase, not a price change.
	maxPriceChange = 0.35
)

// DetectRecurring finds monthly charges: at least two charges from one
// merchant, each 26–35 days after the last, still charged in the last 45
// days, at a similar price. Weekly groceries and one-off purchases don't fit.
// outflows must be oldest first. The result is largest first.
func DetectRecurring(outflows []Outflow, today time.Time) []Recurring {
	byMerchant := map[string][]Outflow{}
	var order []string
	for _, o := range outflows {
		if o.Merchant == "" {
			continue
		}
		if _, ok := byMerchant[o.Merchant]; !ok {
			order = append(order, o.Merchant)
		}
		byMerchant[o.Merchant] = append(byMerchant[o.Merchant], o)
	}
	var out []Recurring
	for _, m := range order {
		charges := byMerchant[m]
		if len(charges) < 2 {
			continue
		}
		last := charges[len(charges)-1]
		if today.Sub(last.PostedOn) > staleAfter {
			continue
		}
		// The run of monthly charges ending with the latest one.
		start := len(charges) - 1
		for start > 0 {
			prev, cur := charges[start-1], charges[start]
			days := int(cur.PostedOn.Sub(prev.PostedOn).Hours() / 24)
			if days < minInterval || days > maxInterval || !similar(prev.AmountCents, cur.AmountCents) {
				break
			}
			start--
		}
		run := charges[start:]
		if len(run) < 2 {
			continue
		}
		r := Recurring{Merchant: m, Label: label(last.Description, m), AmountCents: -last.AmountCents, DayOfMonth: last.PostedOn.Day(),
			FirstSeen: run[0].PostedOn, LastSeen: last.PostedOn, Count: len(run)}
		// A price change is news for a fixed-price charge (a subscription
		// going up), not for a bill that varies every month: report it only
		// when the old price held for at least two charges.
		for i := len(run) - 2; i >= 1; i-- {
			if run[i].AmountCents != last.AmountCents {
				if run[i-1].AmountCents == run[i].AmountCents {
					p, at := -run[i].AmountCents, run[i+1].PostedOn
					r.PreviousCents, r.ChangedOn = &p, &at
				}
				break
			}
		}
		r.New = today.Sub(r.FirstSeen) <= 62*24*time.Hour
		out = append(out, r)
	}
	slices.SortStableFunc(out, func(a, b Recurring) int {
		switch {
		case a.AmountCents > b.AmountCents:
			return -1
		case a.AmountCents < b.AmountCents:
			return 1
		}
		return 0
	})
	return out
}

func similar(a, b int64) bool {
	if a == b {
		return true
	}
	lo, hi := min(-a, -b), max(-a, -b)
	return lo > 0 && float64(hi-lo)/float64(lo) <= maxPriceChange
}

// label is a readable name for a merchant: its rule pattern in title case.
func label(description, merchant string) string {
	p := RulePattern(merchant)
	if p == "" {
		return description
	}
	words := strings.Fields(strings.ToLower(p))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
