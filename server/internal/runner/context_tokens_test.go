package runner

import (
	"testing"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/harness"
)

func TestContextParserThrottlesWrites(t *testing.T) {
	var writes []int64
	cp := &contextParser{Parser: harness.ClaudeCode{}.NewParser(""), write: func(n int64) { writes = append(writes, n) }}
	line := func(n string) {
		cp.Line([]byte(`{"type":"assistant","message":{"usage":{"input_tokens":` + n + `},"content":[]}}`))
	}
	line("100")
	line("200") // within 15s of the first write: held for the turn-end write
	if len(writes) != 1 || writes[0] != 100 {
		t.Fatalf("writes %v, want [100]", writes)
	}
	cp.written = time.Now().Add(-contextWriteEvery)
	line("300")
	if len(writes) != 2 || writes[1] != 300 {
		t.Fatalf("writes %v, want [100 300]", writes)
	}
}
