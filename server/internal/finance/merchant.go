package finance

import (
	"slices"
	"strings"
	"unicode"
)

// noisePrefixes are words banks put ahead of the merchant; dropping them lets
// a rule made from one card's export match another's.
var noisePrefixes = map[string]bool{"POS": true, "DEBIT": true, "CARD": true, "PURCHASE": true, "CHECKCARD": true, "ACH": true, "VISA": true,
	"DDA": true, "WITHDRAWAL": true, "PREAUTHORIZED": true, "RECURRING": true, "TST": true}

// noisePhrases are bank prefixes whose first word alone isn't noise: every
// Fidelity cash-management debit reads "DIRECT DEBIT <payee>", so without
// dropping it one rule caught them all, while "DIRECT DEPOSIT" stays because
// it's how income is recognized.
var noisePhrases = [][]string{{"DIRECT", "DEBIT"}}

// foreignCurrencies follow the foreign amount a card puts ahead of the
// merchant on a charge abroad ("4.55 POUND STERLING CAFFE NERO"). They're
// only dropped after that amount, so a shop named after one keeps its name.
var foreignCurrencies = [][]string{{"POUND", "STERLING"}}

// Merchant normalizes a bank description for matching: upper case, words of
// letters only (store numbers, dates and reference codes vary between
// charges), bank prefixes and a leading foreign amount's currency dropped.
func Merchant(description string) string {
	fields := strings.FieldsFunc(strings.ToUpper(description), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '&'
	})
	hasDigit := func(f string) bool { return strings.IndexFunc(f, unicode.IsDigit) >= 0 }
	amountFirst := len(fields) > 0 && hasDigit(fields[0])
	words := make([]string, 0, len(fields))
	for _, f := range fields {
		if !hasDigit(f) {
			words = append(words, f)
		}
	}
	for {
		if len(words) > 0 && noisePrefixes[words[0]] {
			words = words[1:]
		} else if n := leadingPhrase(words, noisePhrases); n > 0 {
			words = words[n:]
		} else {
			break
		}
	}
	if amountFirst {
		words = words[leadingPhrase(words, foreignCurrencies):]
	}
	return strings.Join(words, " ")
}

// leadingPhrase is the length of the phrase words starts with, or 0.
func leadingPhrase(words []string, phrases [][]string) int {
	for _, p := range phrases {
		if len(words) >= len(p) && slices.Equal(words[:len(p)], p) {
			return len(p)
		}
	}
	return 0
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
