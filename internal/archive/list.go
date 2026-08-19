package archive

import (
	"archive/tar"
	"fmt"
	"io"

	"forge/internal/pkg"
)

func ListPackage(archivePath string) (*pkg.PackageInfo, []string, error) {
	return ListPackageFiltered(archivePath, nil)
}

func ListPackageFiltered(archivePath string, filter *PathFilter) (*pkg.PackageInfo, []string, error) {
	rc, err := OpenCompressed(archivePath)
	if err != nil {
		return nil, nil, err
	}
	defer rc.Close()

	tr := tar.NewReader(rc)
	var info *pkg.PackageInfo
	files := make([]string, 0, 256)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("read tar: %w", err)
		}

		name := cleanArchiveName(hdr.Name)
		if name == "" || name == "." {
			continue
		}

		if filter != nil && filter.Skip(name) {
			if hdr.Typeflag == tar.TypeReg {
				if _, err := io.Copy(io.Discard, tr); err != nil {
					return nil, nil, fmt.Errorf("discard %s: %w", name, err)
				}
			}
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeReg:
			if name == ".PKGINFO" {
				info, err = pkg.ParsePKGINFO(tr)
				if err != nil {
					return nil, nil, fmt.Errorf(".PKGINFO: %w", err)
				}
				continue
			}

			if _, err := io.Copy(io.Discard, tr); err != nil {
				return nil, nil, fmt.Errorf("discard %s: %w", name, err)
			}

			if name != ".INSTALL" && name != ".MTREE" && name != ".BUILDINFO" {
				files = append(files, name)
			}

		case tar.TypeSymlink, tar.TypeLink:
			files = append(files, name)

		case tar.TypeDir:
			// Directory ownership is shared; skip for file conflict
			// checks. Extraction still records directories separately.
		}
	}

	if info == nil {
		return nil, nil, fmt.Errorf("package %s has no .PKGINFO", archivePath)
	}

	return info, files, nil
}
