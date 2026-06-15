package update

import "testing"

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in     string
		want   [3]int
		wantOK bool
	}{
		{"v1.2.3", [3]int{1, 2, 3}, true},
		{"1.2.3", [3]int{1, 2, 3}, true},
		{"v0.1.0", [3]int{0, 1, 0}, true},
		{"v1.2", [3]int{1, 2, 0}, true},
		{"v2", [3]int{2, 0, 0}, true},
		{"v1.2.3-rc1", [3]int{1, 2, 3}, true},
		{"v1.2.3+build7", [3]int{1, 2, 3}, true},
		{"  v1.2.3  ", [3]int{1, 2, 3}, true},
		{"dev", [3]int{}, false},
		{"", [3]int{}, false},
		{"abc123", [3]int{}, false},
		{"v1.2.x", [3]int{}, false},
		{"v1.2.3.4", [3]int{}, false}, // SplitN keeps "3.4" in the last field
	}
	for _, c := range cases {
		got, ok := parseVersion(c.in)
		if ok != c.wantOK || (ok && got != c.want) {
			t.Errorf("parseVersion(%q) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

func TestGreater(t *testing.T) {
	cases := []struct {
		a, b [3]int
		want bool
	}{
		{[3]int{0, 2, 0}, [3]int{0, 1, 0}, true},
		{[3]int{1, 0, 0}, [3]int{0, 9, 9}, true},
		{[3]int{0, 1, 1}, [3]int{0, 1, 0}, true},
		{[3]int{0, 1, 0}, [3]int{0, 1, 0}, false}, // equal is not greater
		{[3]int{0, 1, 0}, [3]int{0, 2, 0}, false},
		{[3]int{1, 0, 0}, [3]int{1, 0, 1}, false},
	}
	for _, c := range cases {
		if got := greater(c.a, c.b); got != c.want {
			t.Errorf("greater(%v, %v) = %v; want %v", c.a, c.b, got, c.want)
		}
	}
}
