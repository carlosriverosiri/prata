package audio

import "testing"

// le16 appends one little-endian int16 sample to b.
func le16(b []byte, v int16) []byte {
	u := uint16(v)
	return append(b, byte(u), byte(u>>8))
}

func TestPeak(t *testing.T) {
	cases := map[string]struct {
		samples []int16
		want    int16
	}{
		"empty is zero":        {samples: nil, want: 0},
		"pure silence is zero": {samples: []int16{0, 0, 0, 0}, want: 0},
		"positive peak":        {samples: []int16{10, 200, 50, -30}, want: 200},
		"negative peak":        {samples: []int16{10, -5000, 50, 30}, want: 5000},
		"min int16 clamps":     {samples: []int16{-32768, 1}, want: 32767},
		"near-silence floor":   {samples: []int16{-40, 12, -3, 7}, want: 40},
		"real-speech level":    {samples: []int16{120, -8000, 6000}, want: 8000},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var pcm []byte
			for _, s := range tc.samples {
				pcm = le16(pcm, s)
			}
			if got := Peak(pcm); got != tc.want {
				t.Errorf("Peak(%v) = %d, want %d", tc.samples, got, tc.want)
			}
		})
	}
}

// A trailing odd byte (malformed S16LE) must not panic and must be ignored.
func TestPeakOddLength(t *testing.T) {
	pcm := []byte{0x10, 0x27, 0x00} // 10000, then a stray byte
	if got := Peak(pcm); got != 10000 {
		t.Errorf("Peak(odd) = %d, want 10000", got)
	}
}
