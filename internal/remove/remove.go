package remove

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"forge/internal/config"
	"forge/internal/db"
)

func Run(cfg *config.Config, targets []string) error {
	entries, err := db.ListLocal(cfg)
	if err != nil {
		return err
	}

	byName := map[string][]*db.LocalEntry{}
	for _, e := range entries {
		byName[e.Package.Name] = append(byName[e.Package.Name], e)
	}

	selected := make([]*db.LocalEntry, 0)
	selectedSet := make(map[*db.LocalEntry]bool)

	for _, target := range targets {
		list := byName[target]
		if len(list) == 0 {
			return fmt.Errorf("package %q is not installed", target)
		}
		for _, e := range list {
			selected = append(selected, e)
			selectedSet[e] = true
		}
	}

	owner := map[string][]*db.LocalEntry{}
	for _, e := range entries {
		for _, f := range e.Files {
			owner[f] = append(owner[f], e)
		}
	}

	for _, e := range selected {
		for _, f := range e.Files {
			path := filepath.Join(cfg.Root, filepath.FromSlash(f))

			fi, err := os.Lstat(path)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return err
			}
			if fi.IsDir() {
				continue
			}

			stillOwned := false
			for _, o := range owner[f] {
				if !selectedSet[o] {
					stillOwned = true
					break
				}
			}
			if stillOwned {
				continue
			}

			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove %s: %w", path, err)
			}
		}
	}

	removeEmptyDirs(cfg.Root, selected)

	for _, e := range selected {
		if err := os.RemoveAll(e.Dir); err != nil {
			return err
		}
	}

	return nil
}

func removeEmptyDirs(root string, entries []*db.LocalEntry) {
	dirs := map[string]struct{}{}

	for _, e := range entries {
		for _, f := range e.Files {
			p := filepath.Dir(filepath.Join(root, filepath.FromSlash(f)))

			for {
				rel, err := filepath.Rel(root, p)
				if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					break
				}

				dirs[p] = struct{}{}

				parent := filepath.Dir(p)
				if parent == p {
					break
				}
				p = parent
			}
		}
	}

	list := make([]string, 0, len(dirs))
	for d := range dirs {
		list = append(list, d)
	}

	sort.Slice(list, func(i, j int) bool {
		return strings.Count(list[i], string(filepath.Separator)) >
			strings.Count(list[j], string(filepath.Separator))
	})

	for _, d := range list {
		_ = os.Remove(d)
	}
}
