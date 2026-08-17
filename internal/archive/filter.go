package archive

import "strings"

// PathFilter skips package entries whose path matches one of the
// configured directory prefixes "removes bloat"
//
// Patterns are package-relative paths without a leading slash, for
// example:
//
//	usr/share/man
//	usr/share/doc
//	usr/share/locale
type PathFilter struct {
	dirs []string
}

// NewPathFilter builds a PathFilter from package-relative directory
// prefixes. Empty patterns are ignored.
func NewPathFilter(patterns ...string) *PathFilter {
	f := &PathFilter{}
	for _, p := range patterns {
		p = strings.Trim(strings.TrimSpace(p), "/")
		if p != "" {
			f.dirs = append(f.dirs, p)
		}
	}
	return f
}

// Skip reports whether a package-relative archive path should be
// skipped during extraction.
func (f *PathFilter) Skip(name string) bool {
	if f == nil {
		return false
	}

	name = strings.Trim(strings.TrimSpace(name), "/")
	for _, d := range f.dirs {
		if name == d || strings.HasPrefix(name, d+"/") {
			return true
		}
	}
	return false
}

// "bloatblocker as i like to call it" returns the recommended size-reduction filter for
// Arch packages.
func DefaultBloatFilter() *PathFilter {
	return NewPathFilter(
		"usr/share/doc",
		"usr/share/man",
		"usr/share/info",
		"usr/share/gtk-doc",
		"usr/share/help",
		"usr/share/locale",
		"usr/lib/locale",
	)
}
