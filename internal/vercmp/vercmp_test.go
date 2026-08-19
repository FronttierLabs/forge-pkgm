package vercmp

import "testing"

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int // sign only: -1, 0, 1
	}{
		{"1.0", "1.0", 0},
		{"1.0", "1.1", -1},
		{"1.1", "1.0", 1},
		{"1.0-1", "1.0-2", -1},
		{"1.0-2", "1.0-1", 1},
		{"1.0", "1.0.1", -1},
		{"1.0.1", "1.0", 1},
		{"1.0~rc1", "1.0", -1},
		{"1.0^git", "1.0", -1},
		{"1.0", "1.0^git", 1},
		{"1:1.0", "2.0", 1},
		{"2.0", "1:1.0", -1},
		{"1.0a", "1.0b", -1},
		{"1.0b", "1.0a", 1},
	}

	for _, c := range cases {
		got := sign(Compare(c.a, c.b))
		if got != c.want {
			t.Errorf("Compare(%q, %q) = sign %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestParseEVR(t *testing.T) {
	e := ParseEVR("1:2.42.2-3")
	if e.Epoch != 1 || e.Version != "2.42.2" || e.Release != "3" {
		t.Fatalf("ParseEVR epoch/rel = %+v", e)
	}

	e2 := ParseEVR("2.42.2")
	if e2.Epoch != 0 || e2.Version != "2.42.2" || e2.Release != "" {
		t.Fatalf("ParseEVR no-release = %+v", e2)
	}
}
