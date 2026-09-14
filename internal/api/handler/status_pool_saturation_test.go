package handler

import "testing"

// TestDeriveSiteStatusSeesASaturatedPool is the /status half of the 2026-09-14 outage.
//
// For 54 minutes the page read "All systems operational" while nginx answered 504 to 14,007
// requests. Three separate blindnesses produced that, and this covers the one the page can
// actually fix from inside the process:
//
//   - The 5xx fraction counts only responses THIS process produced (observability's request
//     window). A request stuck waiting for a pooled connection never completes in Fiber, so
//     nginx's 504 is invisible to it — structurally, not by oversight.
//   - Latency is not measured at all; the per-minute bucket has no field for it.
//   - The database probe is pool.Ping, which succeeds happily while every connection is busy.
//     A pool with zero free connections and a queue of waiters pings in microseconds.
//
// The pool's own saturation is the signal that was there for the taking: pgxpool.Stat reports
// it from memory, with no query and no I/O. An exhausted pool IS the outage's shape.
func TestDeriveSiteStatusSeesASaturatedPool(t *testing.T) {
	// Below the traffic floor on purpose: the point is that saturation reads through even
	// when the error fraction is untrustworthy, which is exactly the case during the outage
	// (few requests COMPLETED, so few were counted).
	const quiet = int64(0)

	cases := []struct {
		name      string
		acquired  int32
		maxConns  int32
		want      providerStatus
		exercises string
	}{
		{
			name: "idle pool is untouched", acquired: 0, maxConns: 10, want: statusOperational,
			exercises: "an ordinary quiet moment must not read as degraded",
		},
		{
			name: "busy but not saturated", acquired: 8, maxConns: 10, want: statusOperational,
			exercises: "a pool doing work is not a pool in trouble",
		},
		{
			name: "one connection short of the threshold", acquired: 8, maxConns: 10, want: statusOperational,
			exercises: "the boundary from below",
		},
		{
			name: "at the saturation threshold", acquired: 9, maxConns: 10, want: statusDegraded,
			exercises: "the boundary from above — 90% of the pool held",
		},
		{
			name: "fully exhausted", acquired: 10, maxConns: 10, want: statusDegraded,
			exercises: "the outage's own shape: every connection held, nothing free",
		},
		{
			name: "a pool that reports no capacity is not a verdict", acquired: 0, maxConns: 0, want: statusOperational,
			exercises: "guards the division: an unconfigured or closed pool must not read as degraded",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveSiteStatus(true, 0, quiet, poolPressure(tc.acquired, tc.maxConns))
			if got != tc.want {
				t.Errorf("deriveSiteStatus(acquired=%d, max=%d) = %q, want %q — %s",
					tc.acquired, tc.maxConns, got, tc.want, tc.exercises)
			}
		})
	}
}

// TestSaturationNeverOutranksTheWorseVerdicts keeps the new signal in its place. Degraded is
// the most it may ever say: a saturated pool means requests are queuing, which is not the same
// claim as "the database is unreachable" or "half of all responses are errors". Letting it
// overwrite either would make the page less truthful, not more.
func TestSaturationNeverOutranksTheWorseVerdicts(t *testing.T) {
	saturated := poolPressure(10, 10)

	if got := deriveSiteStatus(false, 0, 0, saturated); got != statusDown {
		t.Errorf("unreachable database with a saturated pool = %q, want %q — the pool signal must "+
			"not soften a database that does not answer", got, statusDown)
	}

	// Past siteDownErrorRate, with enough traffic to trust the fraction.
	if got := deriveSiteStatus(true, 0.9, minSiteRequestsForSignal, saturated); got != statusDown {
		t.Errorf("90%% error rate with a saturated pool = %q, want %q", got, statusDown)
	}
}
