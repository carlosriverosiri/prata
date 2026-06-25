package failover

import "testing"

// TestHintsOnceAtThreshold: the hint fires exactly at the threshold and not
// again within the same streak.
func TestHintsOnceAtThreshold(t *testing.T) {
	n := New(2)
	if n.RecordFailure(true) {
		t.Fatal("hinted after 1 failure; threshold is 2")
	}
	if !n.RecordFailure(true) {
		t.Fatal("did not hint at the threshold (2nd failure)")
	}
	if n.RecordFailure(true) {
		t.Fatal("hinted again within the same streak")
	}
}

// TestNoHintWhenSwitchNotSuggested: on the cloud backend (suggestSwitch=false)
// we never hint, however many times it fails.
func TestNoHintWhenSwitchNotSuggested(t *testing.T) {
	n := New(1)
	if n.RecordFailure(false) || n.RecordFailure(false) {
		t.Fatal("hinted when suggestSwitch is false")
	}
}

// TestSuccessResetsStreak: after a success, a new outage hints again.
func TestSuccessResetsStreak(t *testing.T) {
	n := New(2)
	n.RecordFailure(true)
	if !n.RecordFailure(true) {
		t.Fatal("expected hint at threshold")
	}
	n.RecordSuccess()
	if n.RecordFailure(true) {
		t.Fatal("hinted on the first failure of a new streak")
	}
	if !n.RecordFailure(true) {
		t.Fatal("did not hint at threshold after reset")
	}
}

// TestThresholdClamped: New(0) clamps to a threshold of 1.
func TestThresholdClamped(t *testing.T) {
	n := New(0)
	if !n.RecordFailure(true) {
		t.Fatal("threshold 0 should clamp to 1 and hint on the first failure")
	}
}

// TestMixedBackendFailuresStillHint: failures on the cloud backend (false)
// advance the count but never hint; once a qualifying local failure (true)
// reaches the threshold, it hints.
func TestMixedBackendFailuresStillHint(t *testing.T) {
	n := New(2)
	if n.RecordFailure(false) {
		t.Fatal("unexpected hint on a cloud-backend failure")
	}
	if !n.RecordFailure(true) {
		t.Fatal("expected hint once the threshold is reached on a local backend")
	}
}
