package remove

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"forge/internal/config"
	"forge/internal/db"
	"forge/internal/dep"
	"forge/internal/pkg"
	"forge/internal/transaction"
)

type Options struct {
	Nodeps bool
}

func Run(cfg *config.Config, targets []string) error {
	return RunWithOptions(cfg, targets, Options{})
}

func RunWithOptions(cfg *config.Config, targets []string, opts Options) error {
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

	if !opts.Nodeps {
		if err := checkDependents(entries, selected); err != nil {
			return err
		}
	}

	ownerCount := map[string]int{}
	for _, e := range entries {
		for _, f := range e.Files {
			ownerCount[f]++
		}
	}

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

// checkDependents refuses removal when a remaining installed package depends
// on something that only the selected packages currently provide.
func checkDependents(entries []*db.LocalEntry, selected map[*db.LocalEntry]bool) error {
	selectedPkgs := make([]*pkg.PackageInfo, 0)
	remainingPkgs := make([]*pkg.PackageInfo, 0)

	for _, e := range entries {
		if selected[e] {
			selectedPkgs = append(selectedPkgs, e.Package)
		} else {
			remainingPkgs = append(remainingPkgs, e.Package)
		}
	}

	var broken []string
	for _, e := range entries {
		if selected[e] {
			continue
		}

		for _, raw := range e.Package.Depends {
			d, err := dep.ParseDep(raw)
			if err != nil {
				continue
			}

			if pkgSatisfiesAny(selectedPkgs, d) && !pkgSatisfiesAny(remainingPkgs, d) {
				broken = append(broken, fmt.Sprintf("%s requires %s", e.Package.Name, raw))
				break
			}
		}
	}

	if len(broken) > 0 {
		return fmt.Errorf("cannot remove packages; dependencies would be broken:\n  %s\n(use --nodeps to override)", strings.Join(broken, "\n  "))
	}

	return nil
}

func pkgSatisfiesAny(pkgs []*pkg.PackageInfo, d dep.Dep) bool {
	for _, p := range pkgs {
		if packageSatisfies(p, d) {
			return true
		}
	}
	return false
}

func packageSatisfies(p *pkg.PackageInfo, d dep.Dep) bool {
	if p.Name == d.Name {
		return d.Satisfies(p.Version)
	}

	for _, raw := range p.Provides {
		pr := dep.ParseProvide(raw)
		if pr.Name == d.Name {
			return pr.Satisfies(d, p.Version)
		}
	}

	return false
}

// removeEmptyDirs best-effort prunes directory trees left empty by removal.
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
