package finance

import (
	"strings"
	"unicode"
)

// noisePrefixes are words banks put ahead of the merchant; dropping them lets
// a rule made from one card's export match another's.
var noisePrefixes = map[string]bool{"POS": true, "DEBIT": true, "CARD": true, "PURCHASE": true, "CHECKCARD": true, "ACH": true, "VISA": true,
	"DDA": true, "WITHDRAWAL": true, "PREAUTHORIZED": true, "RECURRING": true, "TST": true}

// Merchant normalizes a bank description for matching: upper case, words of
// letters only (store numbers, dates and reference codes vary between
// charges), bank prefixes dropped.
func Merchant(description string) string {
	fields := strings.FieldsFunc(strings.ToUpper(description), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '&'
	})
	words := make([]string, 0, len(fields))
	for _, f := range fields {
		if strings.IndexFunc(f, unicode.IsDigit) >= 0 {
			continue
		}
		if len(words) == 0 && noisePrefixes[f] {
			continue
		}
		words = append(words, f)
	}
	return strings.Join(words, " ")
}

// RulePattern is the start of a merchant that a rule made from it matches:
// its first two words, enough to tell "NEW SEASONS" from "NEW YORK TIMES"
// without tying the rule to one store's suffix.
func RulePattern(merchant string) string {
	words := strings.Fields(merchant)
	if len(words) > 2 {
		words = words[:2]
	}
	return strings.Join(words, " ")
}

// ruleMatches is whether a rule's pattern covers a merchant: a whole-word prefix.
func ruleMatches(pattern, merchant string) bool {
	return merchant == pattern || strings.HasPrefix(merchant, pattern+" ")
}
