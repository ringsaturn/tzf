package tzf

// FuzzyMissForTest reports whether f's FUZZY fast path cannot answer at
// (lng, lat). Benchmarks use it to build a pool of slow-path queries.
func FuzzyMissForTest(f F, lng, lat float64) bool {
	df, ok := f.(*defaultFinder)
	if !ok {
		return true
	}
	return df.fuzzy.getTimezoneName(lng, lat) == ""
}
