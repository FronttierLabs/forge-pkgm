package archive

import (
	"archive/tar"
	"fmt"
	"io"
)

// ReadInstall returns the contents of the .INSTALL script inside a package
// archive, or nil if the package has no install script.
func ReadInstall(archivePath string) ([]byte, error) {
	rc, err := OpenCompressed(archivePath)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}

		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if cleanArchiveName(hdr.Name) != ".INSTALL" {
			continue
		}

		raw, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf(".INSTALL: %w", err)
		}
		return raw, nil
	}
}
