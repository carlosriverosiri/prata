package tray

import "testing"

// TestTooltipText covers the tooltip composition: base name, the optional
// active-backend suffix, and the persistent degraded suffix — and that the two
// suffixes compose in that order. tooltipText is pure string logic (no Win32
// calls), so it is safe to exercise on a struct literal without a running tray.
func TestTooltipText(t *testing.T) {
	cases := []struct {
		name     string
		tooltip  string
		backends []string
		active   int
		degraded string
		want     string
	}{
		{
			name:    "base only",
			tooltip: "Prata dev",
			want:    "Prata dev",
		},
		{
			name:     "with backend",
			tooltip:  "Prata dev",
			backends: []string{"LAN GPU-server"},
			active:   0,
			want:     "Prata dev — LAN GPU-server",
		},
		{
			name:     "degraded only",
			tooltip:  "Prata dev",
			degraded: "F1 UPPTAGEN",
			want:     "Prata dev — F1 UPPTAGEN",
		},
		{
			name:     "backend and degraded compose",
			tooltip:  "Prata dev",
			backends: []string{"Rngv GPU-server (Tailscale)", "LAN GPU-server"},
			active:   1,
			degraded: "SVARAR INTE",
			want:     "Prata dev — LAN GPU-server — SVARAR INTE",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tr := &Tray{
				tooltip:        c.tooltip,
				backendNames:   c.backends,
				activeBackend:  c.active,
				degradedReason: c.degraded,
			}
			if got := tr.tooltipText(); got != c.want {
				t.Errorf("tooltipText() = %q, want %q", got, c.want)
			}
		})
	}
}
