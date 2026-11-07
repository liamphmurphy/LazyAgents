package memory

import (
	"regexp"
	"strings"
	"unicode"
)

// secretPatterns match credentials that must never be written to memory. They
// err on the side of refusing: a false positive costs the assistant a rephrase,
// a false negative leaks a credential into a store the person may export.
var secretPatterns = []struct {
	kind string
	re   *regexp.Regexp
}{
	{"private key", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"AWS access key", regexp.MustCompile(`\b(AKIA|ASIA)[0-9A-Z]{16}\b`)},
	{"GitHub token", regexp.MustCompile(`\b(gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{40,})\b`)},
	{"GitLab token", regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}\b`)},
	{"Slack token", regexp.MustCompile(`\bxox[abposr]-[A-Za-z0-9-]{10,}\b`)},
	{"API key", regexp.MustCompile(`\bsk-(ant-|proj-|live_|test_)?[A-Za-z0-9_-]{20,}\b`)},
	{"Stripe key", regexp.MustCompile(`\b[rp]k_(live|test)_[A-Za-z0-9]{16,}\b`)},
	{"Google API key", regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`)},
	{"LazyAgents token", regexp.MustCompile(`\bla_[A-Za-z0-9_-]{20,}\b`)},
	{"JSON web token", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`)},
	{"bearer token", regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/-]{20,}=*`)},
	{"credentials in a URL", regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s/:@]+:[^\s/@]+@`)},
	{"password or secret", regexp.MustCompile(`(?i)\b` + secretWords + `\b\s*[=:]\s*["']?[^\s"']{6,}`)},
}

const secretWords = `(password|passwd|pwd|passphrase|secret|api[_-]?key|access[_-]?key|auth[_-]?token|client[_-]?secret|private[_-]?key|token)`

// secretIs catches "my password is hunter22". Only values that look like a
// credential count, so "my API key is rotated monthly" or "the password is
// stored in 1Password" can still be remembered.
var secretIs = regexp.MustCompile(`(?i)\b` + secretWords + `\s+is\s+["']?([^\s"']+)`)

// DetectSecret names the kind of credential the text contains, or "" if none.
func DetectSecret(texts ...string) string {
	for _, t := range texts {
		for _, p := range secretPatterns {
			if p.re.MatchString(t) {
				return p.kind
			}
		}
		for _, m := range secretIs.FindAllStringSubmatch(t, -1) {
			if looksLikeCredential(strings.TrimRight(m[2], ".,;!?")) {
				return "password or secret"
			}
		}
	}
	return ""
}

// looksLikeCredential: 8+ characters with a digit or a symbol, unlike a word.
func looksLikeCredential(v string) bool {
	if len(v) < 8 {
		return false
	}
	return strings.ContainsFunc(v, func(r rune) bool { return unicode.IsDigit(r) || (unicode.IsPunct(r) || unicode.IsSymbol(r)) })
}
