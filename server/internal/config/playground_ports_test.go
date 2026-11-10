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
