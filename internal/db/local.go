package db

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"forge/internal/config"
	"forge/internal/pkg"
)

type LocalEntry struct {
	Package *pkg.PackageInfo
	Files   []string
	Dir     string
}

func LocalEntryPath(cfg *config.Config, p *pkg.PackageInfo) string {
	return filepath.Join(cfg.DBPath, "local", p.Name+"-"+p.Version)
}

//function for local entry cant remove
func WriteLocalEntry(cfg *config.Config, p *pkg.PackageInfo, files []string) error {
	dir := LocalEntryPath(cfg, p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(dir, "desc"), pkg.WriteDesc(p), 0o644); err != nil {
		return fmt.Errorf("write desc: %w", err)
	}

	sort.Strings(files)

	f, err := os.Create(filepath.Join(dir, "files"))
	if err != nil {
		return fmt.Errorf("write files: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString("%FILES%\n"); err != nil {
		return err
	}
	for _, file := range files {
		if _, err := f.WriteString(file + "\n"); err != nil {
			return err
		}
	}

	return nil
}

func ListLocal(cfg *config.Config) ([]*LocalEntry, error) {
	dir := filepath.Join(cfg.DBPath, "local")

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []*LocalEntry
	for _, de := range entries {
		if !de.IsDir() {
			continue
		}

		full := filepath.Join(dir, de.Name())

		pi, err := pkg.ParseDescFile(filepath.Join(full, "desc"))
		if err != nil {
			continue
		}

		files, err := readLocalFiles(filepath.Join(full, "files"))
		if err != nil {
			files = nil
		}

		out = append(out, &LocalEntry{Package: pi, Files: files, Dir: full})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Package.Name < out[j].Package.Name
	})

	return out, nil
}

func readLocalFiles(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var files []string
	inFiles := false

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())

		if strings.HasPrefix(line, "%") && strings.HasSuffix(line, "%") {
			inFiles = line == "%FILES%"
			continue
		}

		if inFiles && line != "" {
			files = append(files, line)
		}
	}

	return files, sc.Err()
}
