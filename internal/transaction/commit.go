package transaction

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"forge/internal/archive"
	"forge/internal/config"
	"forge/internal/db"
	"forge/internal/scriptlet"
)

// Tx is an "atomic" transaction over the local database and install root.
type Tx struct {
	cfg    *config.Config
	plan   *Plan
	filter *archive.PathFilter
	lock   *Lock

	installBackups map[string]string // rel path = absolute backup path
	removalBackups map[string]string // rel path = absolute backup path
	written        []*db.LocalEntry
	progress       func(string) // DB entries written by this tx
}

// new acquires the database lock and prepares a transaction.
func New(cfg *config.Config, plan *Plan, filter *archive.PathFilter) (*Tx, error) {
	lock, err := AcquireLock(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	return &Tx{
		cfg:            cfg,
		plan:           plan,
		filter:         filter,
		lock:           lock,
		installBackups: make(map[string]string),
		removalBackups: make(map[string]string),
	}, nil
}

// releases the database lock.
// SetProgress installs an optional status reporter used during Commit.
func (tx *Tx) SetProgress(f func(string)) {
	tx.progress = f
}

func (tx *Tx) report(format string, args ...interface{}) {
	if tx.progress != nil {
		tx.progress(fmt.Sprintf(format, args...))
	}
}

func (tx *Tx) Close() error {
	return tx.lock.Close()
}

// Commit applies the plan. On any failure it attempts to roll back every
// mutation made so far and returns the original error.
func (tx *Tx) Commit() error {
	if err := tx.backupOverwrites(); err != nil {
		tx.rollback()
		return err
	}

	installedNames := make(map[string]bool)
	for _, action := range tx.plan.Actions {
		if action.Install != nil {
			installedNames[action.Install.Pkg.Name] = true
		}
	}

	for _, action := range tx.plan.Actions {
		switch {
		case action.Install != nil:
			inst := action.Install
			hookPre, hookPost := "pre_install", "post_install"
			if inst.OldVersion != "" {
				hookPre, hookPost = "pre_upgrade", "post_upgrade"
			}

			if inst.Script != "" {
				if err := tx.runHook(hookPre, inst.Script, inst.OldVersion, inst.Pkg.Version); err != nil {
					tx.rollback()
					return err
				}
			}

			tx.report("installing %s-%s", inst.Pkg.Name, inst.Pkg.Version)

			if _, _, err := archive.ExtractPackageFiltered(
				inst.Archive,
				tx.cfg.Root,
				tx.filter,
			); err != nil {
				tx.rollback()
				return fmt.Errorf("extract %s: %w", inst.Archive, err)
			}

			tx.preserveConfigs(inst)

			if inst.Script != "" {
				if err := tx.runHook(hookPost, inst.Script, inst.OldVersion, inst.Pkg.Version); err != nil {
					tx.rollback()
					return err
				}
			}

		case action.Remove != nil:
			// During an upgrade, the old package's remove hooks must not run:
			// pacman uses pre_upgrade/post_upgrade from the new package only.
			isUpgrade := installedNames[action.Remove.Entry.Package.Name]

			if action.Remove.Script != "" && !isUpgrade {
				if err := tx.runHook("pre_remove", action.Remove.Script, action.Remove.Entry.Package.Version, ""); err != nil {
					tx.rollback()
					return err
				}
			}

			tx.report("removing %s-%s", action.Remove.Entry.Package.Name, action.Remove.Entry.Package.Version)

			if err := tx.applyRemove(action.Remove); err != nil {
				tx.rollback()
				return err
			}

			if action.Remove.Script != "" && !isUpgrade {
				if err := tx.runHook("post_remove", action.Remove.Script, action.Remove.Entry.Package.Version, ""); err != nil {
					tx.rollback()
					return err
				}
			}
		}
	}

	for _, action := range tx.plan.Actions {
		if action.Install == nil {
			continue
		}
		if err := db.WriteLocalEntry(tx.cfg, action.Install.Pkg, action.Install.Files, action.Install.Script); err != nil {
			tx.rollback()
			return err
		}
		tx.written = append(tx.written, &db.LocalEntry{
			Package: action.Install.Pkg,
			Files:   action.Install.Files,
			Dir:     db.LocalEntryPath(tx.cfg, action.Install.Pkg),
		})
	}

	for _, action := range tx.plan.Actions {
		if action.Remove == nil {
			continue
		}
		if err := os.RemoveAll(action.Remove.Entry.Dir); err != nil {
			tx.rollback()
			return err
		}
	}

	tx.cleanupBackups()
	return nil
}

// runHook runs a named install-script hook inside the transaction root.
func (tx *Tx) runHook(hook, script, oldVer, newVer string) error {
	if script == "" {
		return nil
	}
	return scriptlet.Run(context.Background(), tx.cfg.Root, script, hook, oldVer, newVer)
}

// preserveConfigs implements a conservative .pacnew policy. When an install or
// upgrade overwrites a file listed in the package's Backup metadata, the
// existing user file is kept and the new file is written as path.pacnew.
func (tx *Tx) preserveConfigs(inst *InstallAction) {
	for _, rel := range inst.Pkg.Backup {
		backup, exists := tx.installBackups[rel]
		if !exists {
			continue
		}

		target := filepath.Join(tx.cfg.Root, filepath.FromSlash(rel))
		pacnew := target + ".pacnew"

		if err := os.Rename(target, pacnew); err == nil {
			if err := os.Rename(backup, target); err == nil {
				delete(tx.installBackups, rel)
				fmt.Printf("forge: config %s: new file saved as %s\n", rel, filepath.Base(pacnew))
			} else {
				_ = os.Rename(pacnew, target)
			}
		}
	}
}

// installOwned returns the set of paths that install actions will own after
// this transaction. Remove actions must not delete any of these: the install
// step has already superseded them via backup+extract.
func (tx *Tx) installOwned() map[string]struct{} {
	owned := map[string]struct{}{}
	for _, action := range tx.plan.Actions {
		if action.Install == nil {
			continue
		}
		for _, f := range action.Install.Files {
			owned[f] = struct{}{}
		}
	}
	return owned
}

// backupOverwrites renames aside any existing file or symlink that an install
// action will overwrite. Renaming keeps the backup on the same filesystem so
// rollback restore is atomic-ish.
func (tx *Tx) backupOverwrites() error {
	for rel := range tx.installOwned() {
		target := filepath.Join(tx.cfg.Root, filepath.FromSlash(rel))
		if _, err := os.Lstat(target); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}

		backup := target + ".forgebak"
		if err := os.Rename(target, backup); err != nil {
			return err
		}
		tx.installBackups[rel] = backup
	}
	return nil
}

// applyRemove moves the files owned by a RemoveAction aside, except those that
// an InstallAction in the same transaction re-owns.
func (tx *Tx) applyRemove(r *RemoveAction) error {
	skip := tx.installOwned()

	for _, f := range r.Entry.Files {
		if _, inSkip := skip[f]; inSkip {
			continue
		}

		target := filepath.Join(tx.cfg.Root, filepath.FromSlash(f))
		if _, err := os.Lstat(target); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}

		backup := target + ".forgermbak"
		if err := os.Rename(target, backup); err != nil {
			return err
		}
		tx.removalBackups[f] = backup
	}

	return nil
}

// cleanupBackups deletes backup files after a successful package install
func (tx *Tx) cleanupBackups() {
	for _, backup := range tx.installBackups {
		_ = os.Remove(backup)
	}
	for _, backup := range tx.removalBackups {
		_ = os.Remove(backup)
	}
	tx.installBackups = map[string]string{}
	tx.removalBackups = map[string]string{}
}

// rollback restores the filesystem and DB to the pre-transaction state as
// much as possible.
func (tx *Tx) rollback() {
	// Remove files extracted by this tx, in reverse action order.
	for i := len(tx.plan.Actions) - 1; i >= 0; i-- {
		action := tx.plan.Actions[i]
		if action.Install == nil {
			continue
		}
		for _, f := range action.Install.Files {
			target := filepath.Join(tx.cfg.Root, filepath.FromSlash(f))
			if fi, err := os.Lstat(target); err == nil && !fi.IsDir() {
				_ = os.Remove(target)
			}
		}
	}

	// Restore backups taken during install
	for rel, backup := range tx.installBackups {
		target := filepath.Join(tx.cfg.Root, filepath.FromSlash(rel))
		if _, err := os.Stat(backup); err == nil {
			_ = os.Rename(backup, target)
		}
	}
	for rel, backup := range tx.removalBackups {
		target := filepath.Join(tx.cfg.Root, filepath.FromSlash(rel))
		if _, err := os.Stat(backup); err == nil {
			_ = os.Rename(backup, target)
		}
	}
	tx.installBackups = map[string]string{}
	tx.removalBackups = map[string]string{}

	// Remove DB entries written by this tx.
	for _, e := range tx.written {
		_ = os.RemoveAll(e.Dir)
	}
}
