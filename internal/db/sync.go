package db

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"forge/internal/archive"
	"forge/internal/config"
	"forge/internal/fetch"
	"forge/internal/pkg"
)

type RepoDB struct {
	Name     string
	Packages []*pkg.PackageInfo
}

func LoadRepo(ctx context.Context, f *fetch.Fetcher, cfg *config.Config, repo config.Repo, force bool) (*RepoDB, error) {
	if len(repo.Servers) == 0 {
		return nil, fmt.Errorf("repo %q has no servers configured", repo.Name)
	}

	dbCache := filepath.Join(cfg.CacheDir, "sync", repo.Name+".db")

	needFetch := force
	if !needFetch {
		if cfg.SyncInterval > 0 {
			st, err := os.Stat(dbCache)
			if err != nil || time.Since(st.ModTime()) >= cfg.SyncInterval {
				needFetch = true
			}
		} else {
			needFetch = true
		}
	}

	if needFetch {
		var lastErr error
		for _, server := range repo.Servers {
			dbURL := fetch.ExpandRepoURL(server, repo.Name, cfg.Architecture, repo.Name+".db")
			if err := f.Fetch(ctx, dbURL, dbCache); err != nil {
				lastErr = fmt.Errorf("download %s: %w", dbURL, err)
				continue
			}
			lastErr = nil
			break
		}
		if lastErr != nil {
			return nil, lastErr
		}
	}

	rc, err := archive.OpenCompressed(dbCache)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dbCache, err)
	}
	defer rc.Close()

	db := &RepoDB{Name: repo.Name}
	tr := tar.NewReader(rc)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", dbCache, err)
		}

		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}
		if filepath.Base(hdr.Name) != "desc" {
			continue
		}

		pi, err := pkg.ParseDesc(tr)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", hdr.Name, err)
		}

		db.Packages = append(db.Packages, pi)
	}

	return db, nil
}
