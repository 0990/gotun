package stats

type Counter interface {
	// Value is the current value of the counter.
	Value() int64
	// Add adds a value to the current counter value, and returns the previous value.
	Add(int64) int64
}
