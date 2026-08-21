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
	"sync"
	"time"

	"forge/internal/archive"
	"forge/internal/config"
	"forge/internal/db"
	"forge/internal/dep"
	"forge/internal/fetch"
	"forge/internal/pkg"
	"forge/internal/resolve"
	"forge/internal/transaction"
	"forge/internal/vercmp"
)

type Options struct {
	AllowReplace bool
}

// Prepared is the result of a fully prepared install transaction, ready to
// commit. Upgrade can append RemoveActions to Plan before committing
type Prepared struct {
	Plan        *transaction.Plan
	Filter      *archive.PathFilter
	DownloadDur time.Duration
	VerifyDur   time.Duration
}

type downloadJob struct {
	pkg      *pkg.PackageInfo
	repoName string
	servers  []string
	dest     string
}

func Run(ctx context.Context, cfg *config.Config, fetcher *fetch.Fetcher, syncDBs []*db.RepoDB, targets []string) error {
	return RunWithOptions(ctx, cfg, fetcher, syncDBs, targets, Options{})
}

func RunWithOptions(ctx context.Context, cfg *config.Config, fetcher *fetch.Fetcher, syncDBs []*db.RepoDB, targets []string, opts Options) error {
	start := time.Now()

	prepared, err := Prepare(ctx, cfg, fetcher, syncDBs, targets, opts)
	if err != nil {
		return err
	}

	if prepared.Plan.IsEmpty() {
		fmt.Println("forge: nothing to do")
		return nil
	}

	commitStart := time.Now()
	tx, err := transaction.New(cfg, prepared.Plan, prepared.Filter)
	if err != nil {
		return err
	}
	defer tx.Close()

	tx.SetProgress(func(msg string) { fmt.Println("forge:", msg) })

	if err := tx.Commit(); err != nil {
		return err
	}
	commitDur := time.Since(commitStart)

	fmt.Printf("forge: installed %d package(s) in %s (download %s, resolve+verify %s, commit %s)\n",
		len(prepared.Plan.Actions),
		time.Since(start),
		prepared.DownloadDur,
		prepared.VerifyDur,
		commitDur,
	)

	return nil
}

// Prepare resolves, download verifie conflict-check and builds an
// install transaction plan without mutating the filesystem or local DB.
func Prepare(ctx context.Context, cfg *config.Config, fetcher *fetch.Fetcher, syncDBs []*db.RepoDB, targets []string, opts Options) (*Prepared, error) {
	u, pkgRepo := resolve.NewUniverse(syncDBs)

	resolved, err := resolve.New(u).Resolve(targets)
	if err != nil {
		return nil, err
	}

	localEntries, err := db.ListLocal(cfg)
	if err != nil {
		return nil, err
	}

	localByName := make(map[string]*db.LocalEntry)
	for _, e := range localEntries {
		if old, ok := localByName[e.Package.Name]; !ok || vercmp.Compare(e.Package.Version, old.Package.Version) > 0 {
			localByName[e.Package.Name] = e
		}
	}

	var filter *archive.PathFilter
	if len(cfg.NoExtract) > 0 {
		filter = archive.NewPathFilter(cfg.NoExtract...)
	}

	finalPlan := make([]*pkg.PackageInfo, 0, len(resolved))
	for _, p := range resolved {
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
					return nil, fmt.Errorf("%s: %s is installed; %s is available (use forge upgrade)", p.Name, le.Package.Version, p.Version)
				}
			}
		}
		finalPlan = append(finalPlan, p)
	}

	if len(finalPlan) == 0 {
		return &Prepared{Plan: transaction.NewPlan(), Filter: filter}, nil
	}

	serversByRepo := make(map[string][]string, len(cfg.Repos))
	for _, r := range cfg.Repos {
		serversByRepo[r.Name] = r.Servers
	}

	replaced := make(map[string]*db.LocalEntry)
	replacedNames := make(map[string]bool)
	for _, p := range finalPlan {
		for _, raw := range p.Replaces {
			rd, err := dep.ParseDep(raw)
			if err != nil {
				continue
			}
			oldPkg, ok := localByName[rd.Name]
			if !ok || oldPkg.Package.Name == p.Name {
				continue
			}
			if rd.Op == dep.OpNone || rd.Satisfies(oldPkg.Package.Version) {
				if _, exists := replaced[oldPkg.Package.Name]; !exists {
					replaced[oldPkg.Package.Name] = oldPkg
					replacedNames[oldPkg.Package.Name] = true
				}
			}
		}
	}

	//1 build download jobs for every package in the transaction.
	jobs := make([]downloadJob, 0, len(finalPlan))
	for _, p := range finalPlan {
		repoName, ok := pkgRepo[p]
		if !ok {
			return nil, fmt.Errorf("package %s-%s has no source repo", p.Name, p.Version)
		}
		servers, ok := serversByRepo[repoName]
		if !ok || len(servers) == 0 {
			return nil, fmt.Errorf("no Server configured for repo %s", repoName)
		}
		jobs = append(jobs, downloadJob{
			pkg:      p,
			repoName: repoName,
			servers:  servers,
			dest:     filepath.Join(cfg.CacheDir, "pkg", p.Filename),
		})
	}

	//2 download everything. Parallel when cfg.ParallelDownloads > 0.
	dlStart := time.Now()
	if err := downloadAll(ctx, cfg, fetcher, jobs); err != nil {
		return nil, err
	}
	dlDur := time.Since(dlStart)

	//2.5verify, list, and conflict-check in deterministic order, then
	// build the transaction plan.
	planStart := time.Now()

	owned := make(map[string]*db.LocalEntry)
	for _, e := range localEntries {
		for _, f := range e.Files {
			owned[f] = e
		}
	}

	txPlan := transaction.NewPlan()
	for _, job := range jobs {
		p := job.pkg

		if err := verify(p, job.dest); err != nil {
			return nil, err
		}

		infoPre, filesPre, err := archive.ListPackageFiltered(job.dest, filter)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p.Filename, err)
		}
		if infoPre.Name != p.Name {
			return nil, fmt.Errorf("package metadata mismatch: repo has %s, archive has %s", p.Name, infoPre.Name)
		}

		for _, f := range filesPre {
			if oe, exists := owned[f]; exists && oe.Package.Name != p.Name && !replacedNames[oe.Package.Name] {
				return nil, fmt.Errorf("%s: file conflict: %s is owned by %s", p.Name, f, oe.Package.Name)
			}
		}

		le := &db.LocalEntry{
			Package: infoPre,
			Files:   filesPre,
			Dir:     db.LocalEntryPath(cfg, infoPre),
		}
		for _, f := range filesPre {
			owned[f] = le
		}

		installScript, err := archive.ReadInstall(job.dest)
		if err != nil {
			return nil, fmt.Errorf("read install script %s: %w", p.Filename, err)
		}

		oldVersion := ""
		if le, exists := localByName[p.Name]; exists {
			if vercmp.Compare(p.Version, le.Package.Version) > 0 {
				oldVersion = le.Package.Version
			}
		}

		txPlan.AddInstall(&transaction.InstallAction{
			Pkg:        p,
			Archive:    job.dest,
			Files:      filesPre,
			Script:     string(installScript),
			OldVersion: oldVersion,
		})
	}
	for _, oldPkg := range replaced {
		txPlan.AddRemove(&transaction.RemoveAction{
			Entry: &db.LocalEntry{
				Package: oldPkg.Package,
				Files:   oldPkg.Files,
				Dir:     oldPkg.Dir,
			},
			Script: oldPkg.Script,
		})
	}

	verifyDur := time.Since(planStart)

	return &Prepared{
		Plan:        txPlan,
		Filter:      filter,
		DownloadDur: dlDur,
		VerifyDur:   verifyDur,
	}, nil
}

// downloadAll fetches every job to its destination. workers controls how many
// concurrent fetches run; 0 or 1 means sequential.
func downloadAll(ctx context.Context, cfg *config.Config, fetcher *fetch.Fetcher, jobs []downloadJob) error {
	workers := cfg.ParallelDownloads
	if workers <= 1 {
		workers = 1
	}
	if workers > len(jobs) {
		workers = len(jobs)
	}

	total := len(jobs)

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
		sem      = make(chan struct{}, workers)
	)

	for i, job := range jobs {
		sem <- struct{}{}
		wg.Add(1)
		go func(job downloadJob, idx int) {
			defer wg.Done()
			defer func() { <-sem }()

			mu.Lock()
			fmt.Printf("forge: downloading %s (%d/%d)\n", job.pkg.Name+"-"+job.pkg.Version, idx+1, total)
			mu.Unlock()

			var lastErr error
			for _, server := range job.servers {
				url := fetch.ExpandRepoURL(server, job.repoName, cfg.Architecture, job.pkg.Filename)
				if err := fetcher.Fetch(ctx, url, job.dest); err != nil {
					lastErr = fmt.Errorf("download %s: %w", url, err)
					continue
				}
				lastErr = nil
				break
			}
			if lastErr != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = lastErr
				}
				mu.Unlock()
				return
			}

			mu.Lock()
			fmt.Printf("forge: downloaded %s\n", job.pkg.Name+"-"+job.pkg.Version)
			mu.Unlock()
		}(job, i)
	}

	wg.Wait()
	return firstErr
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
