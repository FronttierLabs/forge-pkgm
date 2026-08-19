package archive

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

type tarEntry struct {
	name     string
	typeflag byte
	linkname string
	data     string
	mode     int64
}

func writeTar(t *testing.T, path string, entries []tarEntry) {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for _, e := range entries {
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		hdr := &tar.Header{
			Name:     e.name,
			Typeflag: e.typeflag,
			Linkname: e.linkname,
			Mode:     mode,
			Size:     int64(len(e.data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if e.typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.data)); err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCleanArchiveName(t *testing.T) {
	cases := map[string]string{
		"./usr": "usr",
		"/usr":  "usr",
		"usr":   "usr",
	}
	for in, want := range cases {
		if got := cleanArchiveName(in); got != want {
			t.Errorf("cleanArchiveName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSafeTargetRejectsEscape(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"..", "../x", "a/../../x", "/abs"} {
		if _, err := safeTarget(root, name); err == nil {
			t.Errorf("safeTarget(%q) expected error", name)
		}
	}
}

func TestSafeTargetOK(t *testing.T) {
	root := t.TempDir()
	got, err := safeTarget(root, "usr/bin/foo")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "usr", "bin", "foo")
	if got != want {
		t.Fatalf("safeTarget = %q, want %q", got, want)
	}
}

func TestExtractRejectsParentEscape(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(t.TempDir(), "evil.pkg.tar")
	writeTar(t, pkg, []tarEntry{{name: "../evil", typeflag: tar.TypeReg, data: "pwned"}})

	if _, _, err := ExtractPackageFiltered(pkg, root, nil); err == nil {
		t.Fatal("expected error for ../ in archive")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "evil")); err == nil {
		t.Fatal("file escaped root")
	}
}

func TestExtractRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(t.TempDir(), "symlink-evil.pkg.tar")
	writeTar(t, pkg, []tarEntry{
		{name: "link", typeflag: tar.TypeSymlink, linkname: "../../outside"},
		{name: "link/x", typeflag: tar.TypeReg, data: "pwned"},
	})

	if _, _, err := ExtractPackageFiltered(pkg, root, nil); err == nil {
		t.Fatal("expected symlink escape error")
	}
}

func TestExtractHappyPath(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(t.TempDir(), "ok.pkg.tar")
	writeTar(t, pkg, []tarEntry{
		{name: ".PKGINFO", typeflag: tar.TypeReg, data: "pkgname = foo\npkgver = 1.0-1\narch = x86_64\n"},
		{name: "usr/", typeflag: tar.TypeDir, mode: 0o755},
		{name: "usr/bin/foo", typeflag: tar.TypeReg, data: "#!/bin/sh\n"},
	})

	info, files, err := ExtractPackageFiltered(pkg, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "foo" {
		t.Fatalf("info.Name = %q, want foo", info.Name)
	}
	if _, err := os.Stat(filepath.Join(root, "usr", "bin", "foo")); err != nil {
		t.Fatalf("extracted file missing: %v", err)
	}

	for _, f := range files {
		if f == "usr" || f == "usr/" {
			t.Errorf("directory recorded in files: %q", f)
		}
	}

	found := false
	for _, f := range files {
		if f == "usr/bin/foo" {
			found = true
		}
	}
	if !found {
		t.Error("usr/bin/foo not in file list")
	}
}
