package pkg

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func ParseDesc(r io.Reader) (*PackageInfo, error) {
	p := &PackageInfo{}
	sc := bufio.NewScanner(r)
	field := ""

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())

		if line == "" {
			field = ""
			continue
		}

		if strings.HasPrefix(line, "%") && strings.HasSuffix(line, "%") {
			field = line[1 : len(line)-1]
			continue
		}

		if field == "" {
			continue
		}

		addDescField(p, field, line)
	}

	return p, sc.Err()
}

func ParseDescFile(path string) (*PackageInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseDesc(f)
}

func addDescField(p *PackageInfo, field, val string) {
	switch field {
	case "FILENAME":
		p.Filename = val
	case "NAME":
		p.Name = val
	case "VERSION":
		p.Version = val
	case "BASE":
		p.Base = val
	case "DESC":
		p.Desc = val
	case "URL":
		p.URL = val
	case "ARCH":
		p.Arch = val
	case "PACKAGER":
		p.Packager = val
	case "CSIZE":
		p.Size, _ = strconv.ParseInt(val, 10, 64)
	case "ISIZE":
		p.InstalledSize, _ = strconv.ParseInt(val, 10, 64)
	case "BUILDDATE":
		p.BuildDate, _ = strconv.ParseInt(val, 10, 64)
	case "MD5SUM":
		p.MD5Sum = val
	case "SHA256SUM":
		p.SHA256Sum = val
	case "PGPSIG":
		p.PGPSig = val
	case "LICENSE":
		p.License = append(p.License, val)
	case "GROUPS":
		p.Groups = append(p.Groups, val)
	case "DEPENDS":
		p.Depends = append(p.Depends, val)
	case "OPTDEPENDS":
		p.OptDepends = append(p.OptDepends, val)
	case "PROVIDES":
		p.Provides = append(p.Provides, val)
	case "CONFLICTS":
		p.Conflicts = append(p.Conflicts, val)
	case "REPLACES":
		p.Replaces = append(p.Replaces, val)
	case "BACKUP":
		p.Backup = append(p.Backup, val)
	}
}

func WriteDesc(p *PackageInfo) []byte {
	var b bytes.Buffer

	add := func(field, val string) {
		if val != "" {
			fmt.Fprintf(&b, "%%%s%%\n%s\n\n", field, val)
		}
	}
	addMany := func(field string, vals []string) {
		for _, v := range vals {
			add(field, v)
		}
	}

	add("FILENAME", p.Filename)
	add("NAME", p.Name)
	add("VERSION", p.Version)
	add("BASE", p.Base)
	add("DESC", p.Desc)
	add("URL", p.URL)
	add("ARCH", p.Arch)
	add("PACKAGER", p.Packager)

	if p.Size > 0 {
		add("CSIZE", strconv.FormatInt(p.Size, 10))
	}
	if p.InstalledSize > 0 {
		add("ISIZE", strconv.FormatInt(p.InstalledSize, 10))
	}
	if p.BuildDate > 0 {
		add("BUILDDATE", strconv.FormatInt(p.BuildDate, 10))
	}

	add("MD5SUM", p.MD5Sum)
	add("SHA256SUM", p.SHA256Sum)
	add("PGPSIG", p.PGPSig)

	addMany("LICENSE", p.License)
	addMany("GROUPS", p.Groups)
	addMany("DEPENDS", p.Depends)
	addMany("OPTDEPENDS", p.OptDepends)
	addMany("PROVIDES", p.Provides)
	addMany("CONFLICTS", p.Conflicts)
	addMany("REPLACES", p.Replaces)
	addMany("BACKUP", p.Backup)

	return b.Bytes()
}
