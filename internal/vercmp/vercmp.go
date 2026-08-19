package vercmp

import (
	"strconv"
	"strings"
)

type EVR struct {
	Epoch   uint64
	Version string
	Release string
}

func ParseEVR(s string) EVR {
	var v EVR

	if i := strings.IndexByte(s, ':'); i >= 0 {
		v.Epoch, _ = strconv.ParseUint(s[:i], 10, 64)
		s = s[i+1:]
	}

	if i := strings.IndexByte(s, '-'); i >= 0 {
		v.Version = s[:i]
		v.Release = s[i+1:]
	} else {
		v.Version = s
	}

	return v
}

func Compare(a, b string) int {
	ae, be := ParseEVR(a), ParseEVR(b)

	if ae.Epoch != be.Epoch {
		if ae.Epoch < be.Epoch {
			return -1
		}
		return 1
	}

	if c := rpmvercmp(ae.Version, be.Version); c != 0 {
		return c
	}

	return rpmvercmp(ae.Release, be.Release)
}

func isAlnum(c byte) bool {
	return c >= '0' && c <= '9' ||
		c >= 'a' && c <= 'z' ||
		c >= 'A' && c <= 'Z'
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func rpmvercmp(a, b string) int {
	i, j := 0, 0

	for {
		for i < len(a) && !isAlnum(a[i]) && a[i] != '~' && a[i] != '^' {
			i++
		}
		for j < len(b) && !isAlnum(b[j]) && b[j] != '~' && b[j] != '^' {
			j++
		}

		hasTildeA := i < len(a) && a[i] == '~'
		hasTildeB := j < len(b) && b[j] == '~'
		if hasTildeA || hasTildeB {
			if !hasTildeA {
				return 1
			}
			if !hasTildeB {
				return -1
			}
			i++
			j++
			continue
		}

		hasCaretA := i < len(a) && a[i] == '^'
		hasCaretB := j < len(b) && b[j] == '^'
		if hasCaretA || hasCaretB {
			if !hasCaretA {
				return 1
			}
			if !hasCaretB {
				return -1
			}
			i++
			j++
			continue
		}

		if i >= len(a) || j >= len(b) {
			break
		}

		if isDigit(a[i]) && isDigit(b[j]) {
			for i < len(a) && a[i] == '0' {
				i++
			}
			for j < len(b) && b[j] == '0' {
				j++
			}

			for i < len(a) && j < len(b) && isDigit(a[i]) && isDigit(b[j]) {
				if a[i] != b[j] {
					if a[i] < b[j] {
						return -1
					}
					return 1
				}
				i++
				j++
			}

			if i < len(a) && isDigit(a[i]) {
				return 1
			}
			if j < len(b) && isDigit(b[j]) {
				return -1
			}
		} else {
			if a[i] != b[j] {
				if a[i] < b[j] {
					return -1
				}
				return 1
			}
			i++
			j++
		}
	}

	if i < len(a) {
		return 1
	}
	if j < len(b) {
		return -1
	}
	return 0
}
