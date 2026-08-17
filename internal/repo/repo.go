package repo

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/klauspost/compress/zstd"

	"forge/internal/archive"
	"forge/internal/pkg"
)

func Add(repoName, dbPath string, pkgPaths []string) error {
	f, err := os.Create(dbPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", dbPath, err)
	}
	defer f.Close()

	zw, err := zstd.NewWriter(f)
	if err != nil {
		return fmt.Errorf("zstd writer: %w", err)
	}
	defer zw.Close()

	tw := tar.NewWriter(zw)
	defer tw.Close()

	for _, pkgPath := range pkgPaths {
		info, err := readPackageMeta(pkgPath)
		if err != nil {
			return err
		}

		desc := pkg.WriteDesc(info)
		name := info.Name + "-" + info.Version + "/desc"

		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(desc)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			return fmt.Errorf("write header %s: %w", name, err)
		}

		if _, err := tw.Write(desc); err != nil {
			return fmt.Errorf("write desc %s: %w", name, err)
		}
	}

	return nil
}

func readPackageMeta(pkgPath string) (*pkg.PackageInfo, error) {
	rc, err := archive.OpenCompressed(pkgPath)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	tr := tar.NewReader(rc)
	var info *pkg.PackageInfo

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", pkgPath, err)
		}

		if filepath.Base(hdr.Name) != ".PKGINFO" {
			continue
		}

		info, err = pkg.ParsePKGINFO(tr)
		if err != nil {
			return nil, fmt.Errorf("parse .PKGINFO: %w", err)
		}
		break
	}

	if info == nil {
		return nil, fmt.Errorf("%s has no .PKGINFO", pkgPath)
	}

	info.Filename = filepath.Base(pkgPath)

	if st, err := os.Stat(pkgPath); err == nil {
		info.Size = st.Size()
	}

	if h, err := sha256File(pkgPath); err == nil {
		info.SHA256Sum = h
	}

	return info, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
