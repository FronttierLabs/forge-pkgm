package remove

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"forge/internal/config"
	"forge/internal/db"
	"forge/internal/transaction"
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

	selected := map[*db.LocalEntry]bool{}
	for _, target := range targets {
		list := byName[target]
		if len(list) == 0 {
			return fmt.Errorf("package %q is not installed", target)
		}
		for _, e := range list {
			selected[e] = true
		}
	}

	if len(selected) == 0 {
		fmt.Println("forge: nothing to do")
		return nil
	}

	// ownerCount: how many installed packages own each file.
	ownerCount := map[string]int{}
	for _, e := range entries {
		for _, f := range e.Files {
			ownerCount[f]++
		}
	}
	// selectedCount: how many of the packages being removed own each file.
	selectedCount := map[string]int{}
	for e := range selected {
		for _, f := range e.Files {
			selectedCount[f]++
		}
	}

	plan := transaction.NewPlan()
	for e := range selected {
		files := make([]string, 0, len(e.Files))
		for _, f := range e.Files {
			// Only delete a file if every owner is being removed.
			if ownerCount[f] == selectedCount[f] {
				files = append(files, f)
			}
		}

		plan.AddRemove(&transaction.RemoveAction{
			Entry: &db.LocalEntry{
				Package: e.Package,
				Files:   files,
				Dir:     e.Dir,
			},
			Script: e.Script,
		})
	}

	tx, err := transaction.New(cfg, plan, nil)
	if err != nil {
		return err
	}
	defer tx.Close()

	if err := tx.Commit(); err != nil {
		return err
	}

	removeEmptyDirs(cfg.Root, selected)
	return nil
}

// removeEmptyDirs best-effort prunes directory trees left empty by the
// removal. It is deliberately not part of the transaction because empty
// directories are harmless leftovers.
func removeEmptyDirs(root string, entries map[*db.LocalEntry]bool) {
	dirs := map[string]struct{}{}

	for e := range entries {
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
