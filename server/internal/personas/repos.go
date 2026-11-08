package personas

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/memory"
)

// MaxRepos is how many repositories one persona may list.
const MaxRepos = 10

const maxRepoURLLen = 500

var (
	repoHostRe  = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$`)
	repoPortRe  = regexp.MustCompile(`^[0-9]{1,5}$`)
	repoUserRe  = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)
	repoPathRe  = regexp.MustCompile(`^[A-Za-z0-9._~/-]+$`)
	urlSchemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
)

// NormalizeRepoURL validates a repository URL a coding persona may clone and
// returns its canonical form (trimmed, no trailing slash, lower-case scheme
// and host). The URL ends up in a `git clone` command line and, for a coding
// chat, is cloned with the workspace's git credentials, so only plain https,
// ssh and scp-style remotes are accepted: nothing git would read as an option
// (leading `-`), a remote helper (`ext::`), a local path or file: URL, and no
// credentials in the URL (those belong in Settings secrets).
func NormalizeRepoURL(raw string) (string, error) {
	bad := func(format string, a ...any) (string, error) { return "", apperr.Invalid("repos", format, a...) }
	s := strings.TrimSpace(raw)
	switch {
	case s == "":
		return bad("a repository URL can't be empty")
	case len(s) > maxRepoURLLen:
		return bad("a repository URL must be at most %d characters", maxRepoURLLen)
	case strings.HasPrefix(s, "-"):
		return bad("%q isn't a repository URL", s)
	case strings.ContainsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }):
		return bad("a repository URL can't contain spaces or control characters")
	case strings.Contains(s, "::"):
		return bad("%q isn't a supported repository URL", s)
	case strings.ContainsAny(s, "?#"):
		return bad("a repository URL can't have a query or fragment")
	}
	s = strings.TrimRight(s, "/")

	var scheme, user, host, port, path string
	switch {
	case strings.Contains(s, "://"):
		sch, rest, _ := strings.Cut(s, "://")
		scheme = strings.ToLower(sch)
		if scheme != "https" && scheme != "ssh" {
			return bad("use an https://, ssh:// or git@host:owner/repo repository URL")
		}
		authority, p, ok := strings.Cut(rest, "/")
		if !ok {
			return bad("a repository URL needs an owner and repository name")
		}
		path = p
		if u, h, hasUser := strings.Cut(authority, "@"); hasUser {
			if scheme == "https" {
				return bad("don't put credentials in the URL: store a git token under Settings secrets instead")
			}
			user, authority = u, h
		} else if scheme == "ssh" {
			return bad("an ssh:// URL needs a user, like ssh://git@host/owner/repo")
		}
		host, port, _ = strings.Cut(authority, ":")
		if strings.Contains(authority, ":") && !repoPortRe.MatchString(port) {
			return bad("%q isn't a valid port", port)
		}
	case urlSchemeRe.MatchString(s): // host:path, mailto:, … (an scp remote has user@ first)
		return bad("use an https://, ssh:// or git@host:owner/repo repository URL")
	default:
		// scp form user@host:path
		at := strings.Index(s, "@")
		colon := strings.Index(s, ":")
		if at < 1 || colon < at {
			return bad("use an https://, ssh:// or git@host:owner/repo repository URL")
		}
		scheme, user, host, path = "scp", s[:at], s[at+1:colon], s[colon+1:]
	}
	if scheme != "https" && !repoUserRe.MatchString(user) {
		return bad("%q isn't a valid ssh user", user)
	}
	if !repoHostRe.MatchString(host) {
		return bad("%q isn't a valid host", host)
	}
	if !repoPathRe.MatchString(path) {
		return bad("the repository path may only use letters, digits and . _ ~ / -")
	}
	segs := strings.Split(path, "/")
	if len(segs) < 2 {
		return bad("a repository URL needs an owner and repository name")
	}
	for _, seg := range segs {
		if seg == "" || seg == "." || seg == ".." || strings.HasPrefix(seg, "-") {
			return bad("the repository path has an invalid segment %q", seg)
		}
	}
	if strings.TrimSuffix(segs[len(segs)-1], ".git") == "" {
		return bad("a repository URL needs a repository name")
	}
	if scheme != "https" && (strings.ToLower(host) != "github.com" || (port != "" && port != "22")) {
		return bad("SSH repositories are supported on github.com only; use an https URL for other hosts (public repositories only)")
	}
	if kind := memory.DetectSecret(s); kind != "" {
		return bad("looks like it contains a %s; remove it", kind)
	}
	host = strings.ToLower(host)
	switch scheme {
	case "https":
		if port != "" {
			host += ":" + port
		}
		return "https://" + host + "/" + path, nil
	case "ssh":
		if port != "" {
			host += ":" + port
		}
		return "ssh://" + user + "@" + host + "/" + path, nil
	}
	return user + "@" + host + ":" + path, nil
}

// NormalizeRepos normalizes a persona's repository list, dropping duplicates
// (after normalization) and keeping the first-given order, which makes the
// first the default for new chats. It is never nil.
func NormalizeRepos(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, raw := range in {
		u, err := NormalizeRepoURL(raw)
		if err != nil {
			return nil, err
		}
		dup := false
		for _, o := range out {
			dup = dup || o == u
		}
		if !dup {
			out = append(out, u)
		}
	}
	if len(out) > MaxRepos {
		return nil, apperr.Invalid("repos", "a persona can list at most %d repositories", MaxRepos)
	}
	return out, nil
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slug lower-cases a persona's name to [a-z0-9-] for branch names and git
// author emails: runs of other characters collapse to one dash, at most 30
// characters, "persona" when nothing is left.
func Slug(name string) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(s) > 30 {
		s = strings.Trim(s[:30], "-")
	}
	if s == "" {
		return "persona"
	}
	return s
}

// WorkBranch names the branch a persona's coding chat pushes to. It ends in
// the random tail of the chat id: ids are UUIDv7, whose leading characters
// are the creation time and repeat across chats made within about a minute.
func WorkBranch(personaName string, chatID uuid.UUID) string {
	hex := strings.ReplaceAll(chatID.String(), "-", "")
	return "vesper/" + Slug(personaName) + "-" + hex[len(hex)-8:]
}
