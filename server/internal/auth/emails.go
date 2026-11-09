package auth

import (
	"net/mail"
	"strings"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// A person's email addresses: users.email is the primary (and the unique
// account address), users.extra_emails the rest in the order they listed them.

const (
	MaxEmails      = 10
	maxEmailLength = 254
)

// Emails lists every address of u, primary first.
func Emails(u store.User) []string {
	return append([]string{u.Email}, u.ExtraEmails...)
}

// OwnEmail returns u's address matching addr, ignoring case, in the form u
// saved it.
func OwnEmail(u store.User, addr string) (string, bool) {
	addr = strings.TrimSpace(addr)
	for _, e := range Emails(u) {
		if strings.EqualFold(e, addr) {
			return e, true
		}
	}
	return "", false
}

// NormalizeEmails trims a replacement address list and checks each is a bare
// address (no display name) appearing once, ignoring case.
func NormalizeEmails(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, apperr.Invalid("emails", "add at least one email")
	}
	if len(in) > MaxEmails {
		return nil, apperr.Invalid("emails", "at most %d emails", MaxEmails)
	}
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, e := range in {
		e = strings.TrimSpace(e)
		if len(e) > maxEmailLength {
			return nil, apperr.Invalid("emails", "%q is longer than %d characters", e, maxEmailLength)
		}
		if a, err := mail.ParseAddress(e); err != nil || a.Address != e || a.Name != "" {
			return nil, apperr.Invalid("emails", "%q isn't an email address", e)
		}
		key := strings.ToLower(e)
		if seen[key] {
			return nil, apperr.Invalid("emails", "%s is listed twice", e)
		}
		seen[key] = true
		out = append(out, e)
	}
	return out, nil
}
