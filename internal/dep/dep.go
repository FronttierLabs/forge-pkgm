package dep

import (
	"fmt"
	"strings"

	"forge/internal/vercmp"
)

type Operator int

const (
	OpNone Operator = iota
	OpLess
	OpLessEq
	OpEqual
	OpGreaterEq
	OpGreater
)

type Dep struct {
	Name    string
	Op      Operator
	Version string
}

type Provide struct {
	Name    string
	Version string
}

var operators = []struct {
	op Operator
	s  string
}{
	{OpGreaterEq, ">="},
	{OpLessEq, "<="},
	{OpEqual, "="},
	{OpGreater, ">"},
	{OpLess, "<"},
}

func ParseDep(s string) (Dep, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Dep{}, fmt.Errorf("empty dependency")
	}

	for _, o := range operators {
		idx := strings.Index(s, o.s)
		if idx > 0 && idx+len(o.s) < len(s) {
			return Dep{
				Name:    s[:idx],
				Op:      o.op,
				Version: s[idx+len(o.s):],
			}, nil
		}
	}

	if strings.ContainsAny(s, "<>=") {
		return Dep{}, fmt.Errorf("malformed dependency %q", s)
	}

	return Dep{Name: s, Op: OpNone}, nil
}

func (d Dep) Satisfies(pkgVersion string) bool {
	if d.Op == OpNone {
		return true
	}
	if pkgVersion == "" {
		return false
	}

	if d.Op == OpEqual {
		return equalSatisfies(d.Version, pkgVersion)
	}

	c := vercmp.Compare(pkgVersion, d.Version)
	switch d.Op {
	case OpGreater:
		return c > 0
	case OpGreaterEq:
		return c >= 0
	case OpLess:
		return c < 0
	case OpLessEq:
		return c <= 0
	default:
		return true
	}
}

// equalSatisfies implements Arch
// a dependency like "foo=2.42.2" matches any release of 2.42.2,
// while "foo=2.42.2-1" requires that exact release.
func equalSatisfies(depVersion, pkgVersion string) bool {
	de, pe := vercmp.ParseEVR(depVersion), vercmp.ParseEVR(pkgVersion)

	if de.Epoch != pe.Epoch {
		return false
	}

	if vercmp.Compare(de.Version, pe.Version) != 0 {
		return false
	}

	if de.Release == "" {
		return true
	}

	return vercmp.Compare(de.Release, pe.Release) == 0
}

func ParseProvide(s string) Provide {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '='); i > 0 {
		return Provide{Name: s[:i], Version: s[i+1:]}
	}
	return Provide{Name: s}
}

func (p Provide) Satisfies(d Dep, pkgVersion string) bool {
	if p.Name != d.Name {
		return false
	}
	if d.Op == OpNone {
		return true
	}
	if p.Version != "" {
		return d.Satisfies(p.Version)
	}
	return d.Satisfies(pkgVersion)
}
