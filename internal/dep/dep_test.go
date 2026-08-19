package dep

import "testing"

func TestParseDep(t *testing.T) {
	cases := []struct {
		in   string
		want Dep
		ok   bool
	}{
		{in: "foo", want: Dep{Name: "foo"}, ok: true},
		{in: "foo>=1.2", want: Dep{Name: "foo", Op: OpGreaterEq, Version: "1.2"}, ok: true},
		{in: "foo<=1.2", want: Dep{Name: "foo", Op: OpLessEq, Version: "1.2"}, ok: true},
		{in: "foo=1.2", want: Dep{Name: "foo", Op: OpEqual, Version: "1.2"}, ok: true},
		{in: "foo>1.2", want: Dep{Name: "foo", Op: OpGreater, Version: "1.2"}, ok: true},
		{in: "foo<1.2", want: Dep{Name: "foo", Op: OpLess, Version: "1.2"}, ok: true},
		{in: "", ok: false},
		{in: "=1.0", ok: false},
	}

	for _, c := range cases {
		got, err := ParseDep(c.in)
		if c.ok {
			if err != nil {
				t.Errorf("ParseDep(%q) unexpected error: %v", c.in, err)
				continue
			}
			if got != c.want {
				t.Errorf("ParseDep(%q) = %+v, want %+v", c.in, got, c.want)
			}
		} else if err == nil {
			t.Errorf("ParseDep(%q) expected error, got %+v", c.in, got)
		}
	}
}

func TestSatisfies(t *testing.T) {
	// No operator always satisfied.
	if !(Dep{Name: "x"}).Satisfies("1.0") {
		t.Error("bare dep should always be satisfied")
	}

	ge := Dep{Name: "x", Op: OpGreaterEq, Version: "1.0"}
	if !ge.Satisfies("1.0") {
		t.Error("1.0 should satisfy >=1.0")
	}
	if !ge.Satisfies("1.5") {
		t.Error("1.5 should satisfy >=1.0")
	}
	if ge.Satisfies("0.9") {
		t.Error("0.9 should not satisfy >=1.0")
	}

	// Arch semantics: =2.42.2 matches any release of 2.42.2.
	eq := Dep{Name: "x", Op: OpEqual, Version: "2.42.2"}
	for _, v := range []string{"2.42.2", "2.42.2-1", "2.42.2-99"} {
		if !eq.Satisfies(v) {
			t.Errorf("%s should satisfy =2.42.2", v)
		}
	}
	if eq.Satisfies("2.42.3") {
		t.Error("2.42.3 should not satisfy =2.42.2")
	}

	// Exact release match.
	exact := Dep{Name: "x", Op: OpEqual, Version: "2.42.2-1"}
	if !exact.Satisfies("2.42.2-1") {
		t.Error("2.42.2-1 should satisfy =2.42.2-1")
	}
	if exact.Satisfies("2.42.2-2") {
		t.Error("2.42.2-2 should not satisfy =2.42.2-1")
	}
}

func TestParseProvideAndSatisfies(t *testing.T) {
	p := ParseProvide("foo=1.0")
	if p.Name != "foo" || p.Version != "1.0" {
		t.Fatalf("ParseProvide(foo=1.0) = %+v", p)
	}

	if !p.Satisfies(Dep{Name: "foo", Op: OpGreaterEq, Version: "0.9"}, "1.0") {
		t.Error("foo=1.0 should satisfy foo>=0.9")
	}

	p2 := ParseProvide("bar")
	if p2.Name != "bar" || p2.Version != "" {
		t.Fatalf("ParseProvide(bar) = %+v", p2)
	}
	if p2.Satisfies(Dep{Name: "baz"}, "1.0") {
		t.Error("provide bar should not satisfy dependency baz")
	}
}
