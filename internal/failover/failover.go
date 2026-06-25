// Package failover decides when to surface a one-time hint that the active
// transcription backend is repeatedly failing, so the user can switch to the
// cloud fallback. It NEVER switches backends itself: Prata has no silent
// failover — the switch is always a deliberate user action in the tray menu,
// which keeps patient audio from ever being auto-routed to the cloud. This
// package only decides *when to hint*, and at most once per outage streak.
package failover

// Notifier tracks consecutive backend failures and decides when to hint. Use
// New to build one. It is intended for a single goroutine (the daemon's
// processor) and holds no locks.
type Notifier struct {
	threshold int
	failures  int
	hinted    bool
}

// New returns a Notifier that hints after threshold consecutive failures.
// threshold is clamped to a minimum of 1.
func New(threshold int) *Notifier {
	if threshold < 1 {
		threshold = 1
	}
	return &Notifier{threshold: threshold}
}

// RecordFailure registers one transcription failure on the active backend and
// reports whether the caller should show the switch hint now. suggestSwitch
// must be true only when switching makes sense — the active backend is a
// local/keyless GPU, not the cloud backend itself. The hint fires at most once
// per outage streak: when the consecutive-failure count reaches the threshold,
// suggestSwitch is true, and no hint has been shown since the last success.
func (n *Notifier) RecordFailure(suggestSwitch bool) bool {
	n.failures++
	if !suggestSwitch || n.hinted || n.failures < n.threshold {
		return false
	}
	n.hinted = true
	return true
}

// RecordSuccess clears the streak after the backend responds, so a later
// outage can hint again. A response that yields empty or degenerate text still
// counts as success here: it proves the backend is reachable, which is what
// the hint is about.
func (n *Notifier) RecordSuccess() {
	n.failures = 0
	n.hinted = false
}
