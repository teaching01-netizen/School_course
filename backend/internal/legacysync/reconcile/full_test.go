package reconcile

import "testing"

// TestCourseWorkers pins the pool-sizing helper: the bounded pool never
// exceeds the requested concurrency, never exceeds the item count, and 0/1
// report the caller's serial flag back unchanged.
func TestCourseWorkers(t *testing.T) {
	cases := []struct {
		concurrency, n, want int
	}{
		{0, 10, 0},
		{1, 10, 1},
		{16, 5, 5},
		{16, 0, 0},
		{3, 30, 3},
	}
	for _, tc := range cases {
		if got := courseWorkers(tc.concurrency, tc.n); got != tc.want {
			t.Fatalf("courseWorkers(%d, %d) = %d, want %d", tc.concurrency, tc.n, got, tc.want)
		}
	}
}
