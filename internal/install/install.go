package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"forge/internal/archive"
	"forge/internal/config"
	"forge/internal/db"
	"forge/internal/fetch"
	"forge/internal/pkg"
	"forge/internal/resolve"
	"forge/internal/vercmp"
)

type Options struct {
	AllowReplace bool
}

func Run(ctx context.Context, cfg *config.Config, fetcher *fetch.Fetcher, syncDBs []*db.RepoDB, targets []string) error {
	return RunWithOptions(ctx, cfg, fetcher, syncDBs, targets, Options{})
}

func RunWithOptions(ctx context.Context, cfg *config.Config, fetcher *fetch.Fetcher, syncDBs []*db.RepoDB, targets []string, opts Options) error {
	u, pkgRepo := resolve.NewUniverse(syncDBs)

	plan, err := resolve.New(u).Resolve(targets)
	if err != nil {
		return err
	}

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

	owned := make(map[string]*db.LocalEntry)
	for _, e := range localEntries {
		for _, f := range e.Files {
			owned[f] = e
		}
	}

	finalPlan := make([]*pkg.PackageInfo, 0, len(plan))
	for _, p := range plan {
		le, installed := localByName[p.Name]
		if installed {
			c := vercmp.Compare(p.Version, le.Package.Version)
			switch {
			case c == 0:
				continue
			case c < 0:
				fmt.Printf("forge: %s: installed %s is newer than repo %s; skipping\n", p.Name, le.Package.Version, p.Version)
				continue
			default:
				if !opts.AllowReplace {
					return fmt.Errorf("%s: %s is installed; %s is available (use forge upgrade)", p.Name, le.Package.Version, p.Version)
				}
			}
		}
		finalPlan = append(finalPlan, p)
	}

	if len(finalPlan) == 0 {
		fmt.Println("forge: nothing to do")
		return nil
	}

	serversByRepo := make(map[string][]string, len(cfg.Repos))
	for _, r := range cfg.Repos {
		serversByRepo[r.Name] = r.Servers
	}

	var filter *archive.PathFilter
	if len(cfg.NoExtract) > 0 {
		filter = archive.NewPathFilter(cfg.NoExtract...)
	}

	for _, p := range finalPlan {
		repoName, ok := pkgRepo[p]
		if !ok {
			return fmt.Errorf("package %s-%s has no source repo", p.Name, p.Version)
		}

		servers, ok := serversByRepo[repoName]
		if !ok || len(servers) == 0 {
			return fmt.Errorf("no Server configured for repo %s", repoName)
		}

		pkgCache := filepath.Join(cfg.CacheDir, "pkg", p.Filename)

		var lastErr error
		downloaded := false
		for _, server := range servers {
			pkgURL := fetch.ExpandRepoURL(server, repoName, cfg.Architecture, p.Filename)
			if err := fetcher.Fetch(ctx, pkgURL, pkgCache); err != nil {
				lastErr = fmt.Errorf("download %s: %w", pkgURL, err)
				continue
			}
			downloaded = true
			break
		}
		if !downloaded {
			return lastErr
		}

		if err := verify(p, pkgCache); err != nil {
			return err
		}

		infoPre, filesPre, err := archive.ListPackageFiltered(pkgCache, filter)
		if err != nil {
			return fmt.Errorf("read %s: %w", p.Filename, err)
		}
		if infoPre.Name != p.Name {
			return fmt.Errorf("package metadata mismatch: repo has %s, archive has %s", p.Name, infoPre.Name)
		}

		for _, f := range filesPre {
			if oe, exists := owned[f]; exists && oe.Package.Name != p.Name {
				return fmt.Errorf("%s: file conflict: %s is owned by %s", p.Name, f, oe.Package.Name)
			}
		}

		info, files, err := archive.ExtractPackageFiltered(pkgCache, cfg.Root, filter)
		if err != nil {
			return fmt.Errorf("extract %s: %w", p.Filename, err)
		}
		if info.Name != p.Name {
			return fmt.Errorf("package metadata mismatch: repo has %s, archive has %s", p.Name, info.Name)
		}

		if err := db.WriteLocalEntry(cfg, info, files); err != nil {
			return err
		}
	}

	return nil
}

func verify(p *pkg.PackageInfo, file string) error {
	if p.SHA256Sum != "" {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()

		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}

		got := hex.EncodeToString(h.Sum(nil))
		if !strings.EqualFold(got, p.SHA256Sum) {
			return fmt.Errorf("checksum mismatch for %s: got %s want %s", file, got, p.SHA256Sum)
		}
	}

	if p.Size > 0 {
		st, err := os.Stat(file)
		if err != nil {
			return err
		}
		if st.Size() != p.Size {
			return fmt.Errorf("size mismatch for %s: got %d want %d", file, st.Size(), p.Size)
		}
	}

	return nil
}
