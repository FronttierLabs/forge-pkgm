package upgrade

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"forge/internal/config"
	"forge/internal/db"
	"forge/internal/fetch"
	"forge/internal/install"
	"forge/internal/resolve"
	"forge/internal/vercmp"
)

func Run(ctx context.Context, cfg *config.Config, fetcher *fetch.Fetcher, syncDBs []*db.RepoDB) error {
	u, _ := resolve.NewUniverse(syncDBs)

	localEntries, err := db.ListLocal(cfg)
	if err != nil {
		return err
	}

	localByName := make(map[string]*db.LocalEntry)
	for _, e := range localEntries {
		if old, ok := localByName[e.Package.Name]; !ok || vercmp.Compare(e.Package.Version, old.Package.Version) > 0 {
			localByName[e.Package.Name] = e
		}
	}

	var targets []string
	for name, le := range localByName {
		cands := u.CandidatesByName(name)
		if len(cands) == 0 {
			continue
		}
		best := cands[0]
		if vercmp.Compare(best.Version, le.Package.Version) > 0 {
			targets = append(targets, name)
		}
	}

	if len(targets) == 0 {
		fmt.Println("forge: nothing to do")
		return nil
	}

	fmt.Printf("forge: upgrading %v\n", targets)

	if err := install.RunWithOptions(ctx, cfg, fetcher, syncDBs, targets, install.Options{AllowReplace: true}); err != nil {
		return err
	}

	localEntries, err = db.ListLocal(cfg)
	if err != nil {
		return err
	}

	byName := map[string][]*db.LocalEntry{}
	for _, e := range localEntries {
		byName[e.Package.Name] = append(byName[e.Package.Name], e)
	}

	owner := map[string][]*db.LocalEntry{}
	for _, e := range localEntries {
		for _, f := range e.Files {
			owner[f] = append(owner[f], e)
		}
	}

	for _, entries := range byName {
		if len(entries) <= 1 {
			continue
		}

		newEntry := entries[0]
		for _, e := range entries[1:] {
			if vercmp.Compare(e.Package.Version, newEntry.Package.Version) > 0 {
				newEntry = e
			}
		}

		newFiles := make(map[string]struct{}, len(newEntry.Files))
		for _, f := range newEntry.Files {
			newFiles[f] = struct{}{}
		}

		for _, old := range entries {
			if old == newEntry || old.Package.Version == newEntry.Package.Version {
				continue
			}

			for _, f := range old.Files {
				if _, inNew := newFiles[f]; inNew {
					continue
				}

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
					if o == old || o.Package.Name == newEntry.Package.Name {
						continue
					}
					stillOwned = true
					break
				}
				if stillOwned {
					continue
				}

				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("remove stale %s: %w", path, err)
				}
			}

			if err := os.RemoveAll(old.Dir); err != nil {
				return err
			}
		}
	}

	return nil
}
