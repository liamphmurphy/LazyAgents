package harness

import "testing"

func TestCodexCacheCoverageAcrossMissingCounter(t *testing.T) {
	turn := func(base Usage, summary string) Usage {
		p := (Codex{Baseline: base}).NewParser("test")
		p.Line([]byte(`{"type":"thread.started","thread_id":"same"}`))
		p.Line([]byte(`{"type":"turn.completed","usage":` + summary + `}`))
		return p.Result().Usage
	}
	first := turn(Usage{}, `{"input_tokens":1000,"output_tokens":100,"cached_input_tokens":900}`)
	if first.CacheReportedInputTokens != 1000 || first.CachedTokens != 900 {
		t.Fatal(first)
	}
	missing := turn(first, `{"input_tokens":2000,"output_tokens":200}`)
	if missing.InputTokens != 1000 || missing.OutputTokens != 100 || missing.CacheReportedInputTokens != 0 {
		t.Fatalf("missing cache reset session totals: %+v", missing)
	}
	restored := turn(missing, `{"input_tokens":3000,"output_tokens":300,"cached_input_tokens":2700}`)
	if restored.InputTokens != 1000 || restored.CacheReportedInputTokens != 0 || !restored.SessionCacheKnown {
		t.Fatalf("gap was presented as known: %+v", restored)
	}
	next := turn(restored, `{"input_tokens":4000,"output_tokens":400,"cached_input_tokens":3600}`)
	if next.InputTokens != 1000 || next.CachedTokens != 900 || next.CacheReportedInputTokens != 1000 {
		t.Fatal(next)
	}
}

func TestClaudeCacheCoverageIncludesCreationAsUncached(t *testing.T) {
	p := (ClaudeCode{}).NewParser("test")
	p.Line([]byte(`{"type":"result","usage":{"input_tokens":20,"output_tokens":10,"cache_creation_input_tokens":30,"cache_read_input_tokens":950}}`))
	u := p.Result().Usage
	if u.InputTokens != 1000 || u.CacheReportedInputTokens != 1000 || u.CachedTokens != 950 {
		t.Fatal(u)
	}
	p = (ClaudeCode{}).NewParser("test")
	p.Line([]byte(`{"type":"result","usage":{"input_tokens":100,"output_tokens":10}}`))
	if p.Result().Usage.CacheReportedInputTokens != 0 {
		t.Fatal("missing cache counter treated as zero hits")
	}
}

func TestCodexDoesNotPriceMissingDiscountedCacheCounter(t *testing.T) {
	p := (Codex{}).NewParser("gpt-5")
	p.Line([]byte(`{"type":"turn.completed","usage":{"input_tokens":1000,"output_tokens":100}}`))
	if p.Result().Usage.CostSource != "unknown" {
		t.Fatalf("guessed discounted-cache cost: %+v", p.Result().Usage)
	}
}
