package config

import (
	"slices"
	"testing"
)

func TestPortListAcceptsRangesAndLists(t *testing.T) {
	for in, want := range map[string][]int{
		"":               nil,
		"8090-8092":      {8090, 8091, 8092},
		"8090, 8095":     {8090, 8095},
		"8090-8091,8091": {8090, 8091},
	} {
		got, err := portList(in)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("portList(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"80a", "8092-8090", "0", "65536", "8000-8100", "1-5,10-14,20-24,30-34,40-44"} {
		if _, err := portList(in); err == nil {
			t.Errorf("portList(%q) accepted", in)
		}
	}
}

// No sign-in is for a disposable local instance: anywhere else, anyone who
// reaches the API would act as the person.
func TestNoSignInOnlyOnLoopback(t *testing.T) {
	t.Setenv("VESPER_NO_SIGNIN", "1")
	for url, ok := range map[string]bool{
		"http://localhost:8080":         true,
		"http://127.0.0.1:3000":         true,
		"http://[::1]:8000":             true,
		"https://vesper.example.com":    false,
		"http://192.168.1.20:8080":      false,
		"http://localhost.example.com/": false,
	} {
		t.Setenv("PUBLIC_URL", url)
		c, err := Load()
		if ok && (err != nil || !c.NoSignIn) {
			t.Errorf("PUBLIC_URL=%s: %v, NoSignIn=%v; want it on", url, err, c.NoSignIn)
		}
		if !ok && err == nil {
			t.Errorf("PUBLIC_URL=%s: accepted", url)
		}
	}
	t.Setenv("VESPER_NO_SIGNIN", "")
	t.Setenv("PUBLIC_URL", "https://vesper.example.com")
	if c, err := Load(); err != nil || c.NoSignIn {
		t.Errorf("unset: %v, NoSignIn=%v", err, c.NoSignIn)
	}
}
