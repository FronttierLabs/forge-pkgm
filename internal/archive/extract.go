package archive

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"forge/internal/pkg"
)

func ExtractPackage(archivePath, root string) (*pkg.PackageInfo, []string, error) {
	return ExtractPackageFiltered(archivePath, root, nil)
}

func ExtractPackageFiltered(archivePath, root string, filter *PathFilter) (*pkg.PackageInfo, []string, error) {
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
			// For regular files we must consume the payload so the tar
			// reader stays positioned correctly. Other entry types have
			// no payload.
			if hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeRegA {
				if _, err := io.Copy(io.Discard, tr); err != nil {
					return nil, nil, fmt.Errorf("discard %s: %w", name, err)
				}
			}
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			switch name {
			case ".PKGINFO":
				info, err = pkg.ParsePKGINFO(tr)
				if err != nil {
					return nil, nil, fmt.Errorf(".PKGINFO: %w", err)
				}
				continue

			case ".INSTALL", ".MTREE", ".BUILDINFO":
				if _, err := io.Copy(io.Discard, tr); err != nil {
					return nil, nil, fmt.Errorf("discard %s: %w", name, err)
				}
				continue
			}

			target, err := safeTarget(root, name)
			if err != nil {
				return nil, nil, err
			}
			if err := ensureNoSymlinkParent(root, target); err != nil {
				return nil, nil, err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return nil, nil, fmt.Errorf("mkdir parent %s: %w", filepath.Dir(target), err)
			}
			if err := removeExisting(target); err != nil {
				return nil, nil, err
			}
			if err := writeRegular(target, hdr, tr); err != nil {
				return nil, nil, err
			}
			files = append(files, name)

		case tar.TypeDir:
			target, err := safeTarget(root, name)
			if err != nil {
				return nil, nil, err
			}
			dirMode := os.FileMode(hdr.Mode) & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
			if err := os.MkdirAll(target, dirMode); err != nil {
				return nil, nil, fmt.Errorf("mkdir %s: %w", target, err)
			}
			if err := os.Chmod(target, dirMode); err != nil {
				return nil, nil, fmt.Errorf("chmod %s: %w", target, err)
			}
			files = append(files, name)

		case tar.TypeSymlink:
			target, err := safeTarget(root, name)
			if err != nil {
				return nil, nil, err
			}
			if err := ensureNoSymlinkParent(root, target); err != nil {
				return nil, nil, err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return nil, nil, fmt.Errorf("mkdir parent %s: %w", filepath.Dir(target), err)
			}
			if err := removeExisting(target); err != nil {
				return nil, nil, err
			}
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return nil, nil, fmt.Errorf("symlink %s: %w", target, err)
			}
			files = append(files, name)

		case tar.TypeLink:
			target, err := safeTarget(root, name)
			if err != nil {
				return nil, nil, err
			}
			linkTarget, err := safeTarget(root, cleanArchiveName(hdr.Linkname))
			if err != nil {
				return nil, nil, err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return nil, nil, fmt.Errorf("mkdir parent %s: %w", filepath.Dir(target), err)
			}
			if err := removeExisting(target); err != nil {
				return nil, nil, err
			}
			if err := os.Link(linkTarget, target); err != nil {
				return nil, nil, fmt.Errorf("hardlink %s: %w", target, err)
			}
			files = append(files, name)

		default:
		}
	}

	if info == nil {
		return nil, nil, fmt.Errorf("package %s has no .PKGINFO", archivePath)
	}

	return info, files, nil
}

func cleanArchiveName(name string) string {
	name = filepath.ToSlash(name)
	name = strings.TrimPrefix(name, "./")
	name = strings.TrimPrefix(name, "/")
	return name
}

func safeTarget(root, name string) (string, error) {
	if name == "" || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("invalid archive path %q", name)
	}
	if name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "/../") {
		return "", fmt.Errorf("archive path escapes root: %q", name)
	}

	clean := path.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("archive path escapes root: %q", name)
	}

	target := filepath.Join(root, filepath.FromSlash(clean))
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive path escapes root: %q", name)
	}

	return target, nil
}

func ensureNoSymlinkParent(root, target string) error {
	rel, err := filepath.Rel(root, filepath.Dir(target))
	if err != nil {
		return err
	}
	if rel == "." || rel == "" {
		return nil
	}

	cur := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		cur = filepath.Join(cur, part)

		fi, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to extract through symlink %s", cur)
		}
	}

	return nil
}

// removeExisting emoves a path unless it is a directory. It is safe
// to call for a path that does not yet exist.
func removeExisting(target string) error {
	fi, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("lstat %s: %w", target, err)
	}

	if fi.IsDir() {
		return fmt.Errorf("refusing to replace directory %s", target)
	}

	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing %s: %w", target, err)
	}

	return nil
}

func writeRegular(target string, hdr *tar.Header, tr *tar.Reader) error {
// Preserve setuid, setgid, and sticky bits as well as normal
// permissions. Forge must not strip them if strip it fails
	mode := os.FileMode(hdr.Mode) & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)

	f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create %s: %w", target, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, tr); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	if err := f.Chmod(mode); err != nil {
		return fmt.Errorf("chmod %s: %w", target, err)
	}

	return nil
}
