package projects

import "testing"

func TestRepoURLValidation(t *testing.T) {
	ok := []string{"git@github.com:liamphmurphy/LazyAgents.git", "git@github.com:a/b", "ssh://git@github.com/a/b.git", "  git@github.com:a/b.git/ ", ""}
	bad := []string{"https://github.com/a/b", "http://github.com/a/b.git", "git@gitlab.com:a/b.git", "git@github.com:a/b.git?x=1", "git@github.com:a/..", "git@github.com:a", "file:///tmp/x"}
	for _, u := range ok {
		if err := normalize(&Config{Name: "N", Color: "#aabbcc", RepoURL: u}); err != nil {
			t.Errorf("%q rejected: %v", u, err)
		}
	}
	for _, u := range bad {
		if err := normalize(&Config{Name: "N", Color: "#aabbcc", RepoURL: u}); err == nil {
			t.Errorf("%q accepted", u)
		}
	}
}
