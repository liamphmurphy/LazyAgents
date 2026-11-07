package httpapi

// Clamp to saved run input: legacy turns may have cumulative counters and
// unmanaged usage may not have any per-turn cache accounting.
func cacheBreakdown(input, cached, reported int64) (int64, int64, int64) {
	reported = max(0, min(reported, input))
	cached = max(0, min(cached, reported))
	return cached, reported - cached, max(0, input-reported)
}
