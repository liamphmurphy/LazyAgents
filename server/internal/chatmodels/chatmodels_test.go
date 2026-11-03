package chatmodels

import (
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
)

func TestModelsHaveOneDefault(t *testing.T) {
	for _, h := range agentdef.Harnesses {
		n := 0
		for _, m := range models(h) {
			if m.Default {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%s has %d default models", h, n)
		}
		if KeySecret[h] == "" {
			t.Errorf("%s has no key secret", h)
		}
	}
}
