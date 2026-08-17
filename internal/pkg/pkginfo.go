package pkg

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

func ParsePKGINFO(r io.Reader) (*PackageInfo, error) {
	p := &PackageInfo{}
	sc := bufio.NewScanner(r)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)

		switch key {
		case "pkgname":
			p.Name = val
		case "pkgver":
			p.Version = val
		case "pkgdesc":
			p.Desc = val
		case "url":
			p.URL = val
		case "arch":
			p.Arch = val
		case "packager":
			p.Packager = val
		case "size":
			p.InstalledSize, _ = strconv.ParseInt(val, 10, 64)
		case "builddate":
			p.BuildDate, _ = strconv.ParseInt(val, 10, 64)
		case "depend":
			p.Depends = append(p.Depends, val)
		case "optdepend":
			p.OptDepends = append(p.OptDepends, val)
		case "provides":
			p.Provides = append(p.Provides, val)
		case "conflict":
			p.Conflicts = append(p.Conflicts, val)
		case "replaces":
			p.Replaces = append(p.Replaces, val)
		case "license":
			p.License = append(p.License, val)
		}
	}

	return p, sc.Err()
}
