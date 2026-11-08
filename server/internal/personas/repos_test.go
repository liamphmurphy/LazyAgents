package personas

import (
	"errors"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
)

func TestNormalizeRepoURL(t *testing.T) {
	ok := map[string]string{
		"https://github.com/a/b":             "https://github.com/a/b",
		"  HTTPS://GitHub.com/A/B.git/ ":     "https://github.com/A/B.git",
		"https://github.com:8443/a/b.git":    "https://github.com:8443/a/b.git",
		"https://gitlab.example.com/g/sub/r": "https://gitlab.example.com/g/sub/r",
		"git@github.com:a/b.git":             "git@github.com:a/b.git",
		"git@GitHub.com:a/b":                 "git@github.com:a/b",
		"ssh://git@github.com:22/a/b":        "ssh://git@github.com:22/a/b",
		"SSH://git@github.com/a/b.git":       "ssh://git@github.com/a/b.git",
		"https://github.com/a/b..c/d_e~f.g":  "https://github.com/a/b..c/d_e~f.g",
	}
	for in, want := range ok {
		got, err := NormalizeRepoURL(in)
		if err != nil || got != want {
			t.Errorf("NormalizeRepoURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{
		"", "   ",
		"-uhttps://x", "--upload-pack=touch", "-oProxyCommand=x@y:a/b",
		"ext::sh -c touch", "ext::sh", "fd::3/a", "https://github.com/a::b",
		"file:///etc", "file:///etc/passwd/x", "git://github.com/a/b", "http://github.com/a/b",
		"/tmp/repo", "./repo", "../repo", "~/repo", "repo", "a/b",
		"github.com:a/b", "ftp://github.com/a/b",
		"https://u:t@github.com/a/b", "https://user@github.com/a/b",
		"https://github.com/a/b?x=1", "https://github.com/a/b#frag",
		"https://github.com/a", "https://github.com", "https://github.com/", "https://github.com//b",
		"https://github.com/a/../b", "https://github.com/./b", "https://github.com/a/.git",
		"https://github.com/-a/b", "https://github.com/a/b c", "https://github.com/a\n/b", "https://github.com/a/\tb",
		"https://github.com/a/b%2e", "https://github.com/a/b;rm", "https://github.com/a/$(x)",
		"https://-evil.com/a/b", "https://exa mple.com/a/b", "https://host:abc/a/b", "https://host:/a/b",
		"ssh://github.com/a/b", "ssh://-oProxy@github.com/a/b", "ssh://git@/a/b",
		"git@gitlab.com:a/b", "ssh://git@example.com/a/b", "git@github.com.evil.io:a/b", "ssh://git@github.com:2222/a/b",
		"git@github.com:a", "git@github.com:/a/b", "git@github.com:a/../b", "@github.com:a/b", "git@:a/b",
		"https://github.com/" + strings.Repeat("a", 500) + "/b",
		"https://github.com/a/ghp_abcdefghijklmnopqrstuvwxyz0123456789",
	}
	for _, in := range bad {
		got, err := NormalizeRepoURL(in)
		var ve *apperr.ValidationError
		if err == nil || !errors.As(err, &ve) || ve.Field != "repos" {
			t.Errorf("NormalizeRepoURL(%q) = %q, %v; want a repos validation error", in, got, err)
		}
	}
	if _, err := NormalizeRepoURL("git@gitlab.com:a/b"); err == nil || !strings.Contains(err.Error(), "SSH repositories are supported on github.com only; use an https URL for other hosts (public repositories only)") {
		t.Errorf("non-github ssh error: %v", err)
	}
	if _, err := NormalizeRepoURL("https://u:t@github.com/a/b"); err == nil || !strings.Contains(err.Error(), "Settings secrets") {
		t.Errorf("credentials error should point at Settings secrets: %v", err)
	}
}

func TestNormalizeRepos(t *testing.T) {
	got, err := NormalizeRepos([]string{"https://GitHub.com/a/b", "https://github.com/a/b/", "git@github.com:a/c"})
	if err != nil || len(got) != 2 || got[0] != "https://github.com/a/b" || got[1] != "git@github.com:a/c" {
		t.Fatalf("dedupe: %v %v", got, err)
	}
	if got, err := NormalizeRepos(nil); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("nil must give an empty non-nil list: %#v %v", got, err)
	}
	var many []string
	for i := 0; i < 11; i++ {
		many = append(many, "https://github.com/a/r"+string(rune('a'+i)))
	}
	if _, err := NormalizeRepos(many); err == nil {
		t.Fatal("11 repos accepted")
	}
	if got, err := NormalizeRepos(many[:10]); err != nil || len(got) != 10 {
		t.Fatalf("10 repos: %v %v", got, err)
	}
}
