package upgrade

import (
	"context"
	"fmt"

	"forge/internal/config"
	"forge/internal/db"
	"forge/internal/fetch"
	"forge/internal/install"
	"forge/internal/resolve"
	"forge/internal/transaction"
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
	outdated := map[string]*db.LocalEntry{}
	for name, le := range localByName {
		cands := u.CandidatesByName(name)
		if len(cands) == 0 {
			continue
		}
		best := cands[0]
		if vercmp.Compare(best.Version, le.Package.Version) > 0 {
			targets = append(targets, name)
			outdated[name] = le
		}
	}

	if len(targets) == 0 {
		fmt.Println("forge: nothing to do")
		return nil
	}

	fmt.Printf("forge: upgrading %v\n", targets)

	prepared, err := install.Prepare(ctx, cfg, fetcher, syncDBs, targets, install.Options{AllowReplace: true})
	if err != nil {
		return err
	}

	if prepared.Plan.IsEmpty() {
		return fmt.Errorf("upgrade targets produced an empty plan")
	}

	// Append RemoveActions only for packages that actually get a new version
	// in this transaction. The transaction's shared-path skip rule transfers
	// ownership of overlapping files from old to new atomically.
	installNames := make(map[string]bool)
	for _, action := range prepared.Plan.Actions {
		if action.Install != nil {
			installNames[action.Install.Pkg.Name] = true
		}
	}

	for name, old := range outdated {
		if !installNames[name] {
			continue
		}
		prepared.Plan.AddRemove(&transaction.RemoveAction{
			Entry: old,
		})
	}

	tx, err := transaction.New(cfg, prepared.Plan, prepared.Filter)
	if err != nil {
		return err
	}
	defer tx.Close()

	tx.SetProgress(func(msg string) { fmt.Println("forge:", msg) })

	if err := tx.Commit(); err != nil {
		return err
	}

	fmt.Printf("forge: upgraded %d package(s)\n", len(prepared.Plan.Actions))
	return nil
}
