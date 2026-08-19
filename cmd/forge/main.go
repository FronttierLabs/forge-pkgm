package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"forge/internal/config"
	"forge/internal/db"
	"forge/internal/fetch"
	"forge/internal/install"
	"forge/internal/pkg"
	"forge/internal/remove"
	"forge/internal/repo"
	"forge/internal/upgrade"
)

var version = "dev"

type options struct {
	root     string
	dbpath   string
	cachedir string
	conf     string
	arch     string
	verbose  int
}

func main() {
	opts := options{}
	args := os.Args[1:]
	remaining := []string{}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			usage()
			return
		case "-v", "--verbose":
			opts.verbose++
		case "--root":
			i++
			if i >= len(args) {
				fail("--root requires a value")
			}
			opts.root = args[i]
		case "--dbpath":
			i++
			if i >= len(args) {
				fail("--dbpath requires a value")
			}
			opts.dbpath = args[i]
		case "--cachedir":
			i++
			if i >= len(args) {
				fail("--cachedir requires a value")
			}
			opts.cachedir = args[i]
		case "--config":
			i++
			if i >= len(args) {
				fail("--config requires a value")
			}
			opts.conf = args[i]
		case "--arch":
			i++
			if i >= len(args) {
				fail("--arch requires a value")
			}
			opts.arch = args[i]
		default:
			if strings.HasPrefix(args[i], "-") {
				fail(fmt.Sprintf("unknown flag %q", args[i]))
			}
			remaining = append(remaining, args[i])
		}
	}

	if len(remaining) == 0 {
		usage()
		os.Exit(1)
	}

	cmd := remaining[0]
	cmdArgs := remaining[1:]
	cfg := loadConfig(opts)

	switch cmd {
	case "version":
		fmt.Printf("forge %s\n", version)
	case "install":
		requireArgs(cmd, cmdArgs, 1)
		runInstall(cfg, cmdArgs)
	case "remove":
		requireArgs(cmd, cmdArgs, 1)
		runRemove(cfg, cmdArgs)
	case "update":
		runUpdate(cfg)
	case "upgrade":
		runUpgrade(cfg)
	case "list":
		runList(cfg, cmdArgs)
	case "info":
		requireArgs(cmd, cmdArgs, 1)
		runInfo(cfg, cmdArgs)
	case "search":
		requireArgs(cmd, cmdArgs, 1)
		runSearch(cfg, cmdArgs)
	case "clean":
		runClean(cfg, cmdArgs)
	case "repo-add":
		requireArgs(cmd, cmdArgs, 2)
		runRepoAdd(cmdArgs)
	default:
		fail(fmt.Sprintf("unknown command %q", cmd))
	}
}

func loadConfig(opts options) *config.Config {
	cfg := &config.Config{Architecture: "x86_64"}

	path := opts.conf
	if path == "" {
		path = "/etc/forge/forge.conf"
	}

	if _, err := os.Stat(path); err == nil {
		parsed, err := config.Parse(path)
		if err != nil {
			fail(fmt.Sprintf("parse config: %v", err))
		}
		cfg = parsed
	} else if opts.conf != "" {
		fail(fmt.Sprintf("config file not found: %s", path))
	}

	root := opts.root
	if root == "" {
		root = cfg.Root
	}
	if root == "" {
		root = "/"
	}

	if opts.arch != "" {
		cfg.Architecture = opts.arch
	}
	if opts.dbpath != "" {
		cfg.DBPath = opts.dbpath
	}
	if cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(root, "var/lib/forge")
	}
	if opts.cachedir != "" {
		cfg.CacheDir = opts.cachedir
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = filepath.Join(root, "var/cache/forge")
	}
	if !filepath.IsAbs(cfg.DBPath) {
		cfg.DBPath = filepath.Join(root, cfg.DBPath)
	}
	if !filepath.IsAbs(cfg.CacheDir) {
		cfg.CacheDir = filepath.Join(root, cfg.CacheDir)
	}

	cfg.Root = root
	return cfg
}

func syncRepos(cfg *config.Config, force bool) ([]*db.RepoDB, *fetch.Fetcher) {
	ctx := context.Background()
	fetcher := fetch.New()
	fetcher.XferCommand = cfg.XferCommand

	out := make([]*db.RepoDB, 0, len(cfg.Repos))
	for _, r := range cfg.Repos {
		repoDB, err := db.LoadRepo(ctx, fetcher, cfg, r, force)
		if err != nil {
			fail(err.Error())
		}
		out = append(out, repoDB)
	}

	return out, fetcher
}

func runInstall(cfg *config.Config, targets []string) {
	syncDBs, fetcher := syncRepos(cfg, false)
	if err := install.Run(context.Background(), cfg, fetcher, syncDBs, targets); err != nil {
		fail(err.Error())
	}
}

func runRemove(cfg *config.Config, targets []string) {
	if err := remove.Run(cfg, targets); err != nil {
		fail(err.Error())
	}
	fmt.Printf("forge: removed %v\n", targets)
}

func runUpdate(cfg *config.Config) {
	syncRepos(cfg, true)
	fmt.Println("forge: sync databases updated")
}

func runUpgrade(cfg *config.Config) {
	syncDBs, fetcher := syncRepos(cfg, true)
	if err := upgrade.Run(context.Background(), cfg, fetcher, syncDBs); err != nil {
		fail(err.Error())
	}
}

func runList(cfg *config.Config, args []string) {
	if len(args) == 1 {
		syncDBs, _ := syncRepos(cfg, false)

		for _, r := range syncDBs {
			if r.Name != args[0] {
				continue
			}
			sort.Slice(r.Packages, func(i, j int) bool {
				return r.Packages[i].Name < r.Packages[j].Name
			})
			for _, p := range r.Packages {
				fmt.Printf("%s %s\n", p.Name, p.Version)
			}
			return
		}

		fail(fmt.Sprintf("repo %q not found", args[0]))
	}

	entries, err := db.ListLocal(cfg)
	if err != nil {
		fail(err.Error())
	}

	for _, e := range entries {
		fmt.Printf("%s %s\n", e.Package.Name, e.Package.Version)
	}
}

func runInfo(cfg *config.Config, args []string) {
	query := args[0]

	if entries, err := db.ListLocal(cfg); err == nil {
		for _, e := range entries {
			if e.Package.Name == query || e.Package.Name+"-"+e.Package.Version == query {
				printPkgInfo(e.Package)
				return
			}
		}
	}

	syncDBs, _ := syncRepos(cfg, false)
	for _, r := range syncDBs {
		for _, p := range r.Packages {
			if p.Name == query || p.Name+"-"+p.Version == query {
				printPkgInfo(p)
				return
			}
		}
	}

	fail(fmt.Sprintf("package %q not found", query))
}

func runSearch(cfg *config.Config, args []string) {
	term := strings.ToLower(args[0])
	syncDBs, _ := syncRepos(cfg, false)

	for _, r := range syncDBs {
		for _, p := range r.Packages {
			if strings.Contains(strings.ToLower(p.Name), term) ||
				strings.Contains(strings.ToLower(p.Desc), term) {
				fmt.Printf("%s/%s %s\n", r.Name, p.Name, p.Version)
			}
		}
	}
}

func runRepoAdd(args []string) {
	repoName := args[0]
	pkgFiles := args[1:]

	dbPath := repoName + ".db.tar.zst"
	if err := repo.Add(repoName, dbPath, pkgFiles); err != nil {
		fail(err.Error())
	}

	link := repoName + ".db"
	_ = os.Remove(link)
	if err := os.Symlink(filepath.Base(dbPath), link); err != nil {
		fail(fmt.Sprintf("create %s symlink: %v", link, err))
	}

	fmt.Printf("forge: wrote %s -> %s\n", dbPath, link)
}

func runClean(cfg *config.Config, args []string) {
	removeSync := false
	for _, a := range args {
		if a == "all" {
			removeSync = true
		}
	}

	var freed int64

	pkgDir := filepath.Join(cfg.CacheDir, "pkg")
	freed += dirSize(pkgDir)
	if err := os.RemoveAll(pkgDir); err != nil {
		fail(fmt.Sprintf("clean package cache: %v", err))
	}

	if removeSync {
		syncDir := filepath.Join(cfg.CacheDir, "sync")
		freed += dirSize(syncDir)
		if err := os.RemoveAll(syncDir); err != nil {
			fail(fmt.Sprintf("clean sync cache: %v", err))
		}
	}

	fmt.Printf("forge: freed %s from cache\n", humanSize(freed))
}

func dirSize(dir string) int64 {
	var size int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2f GiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.2f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.2f KiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func printPkgInfo(p *pkg.PackageInfo) {
	fmt.Printf("Name           : %s\n", p.Name)
	fmt.Printf("Version        : %s\n", p.Version)
	fmt.Printf("Architecture   : %s\n", p.Arch)
	fmt.Printf("Description    : %s\n", p.Desc)
	fmt.Printf("Depends On     : %s\n", strings.Join(p.Depends, " "))
	fmt.Printf("Optional Deps  : %s\n", strings.Join(p.OptDepends, " "))
	fmt.Printf("Provides       : %s\n", strings.Join(p.Provides, " "))
	fmt.Printf("Conflicts With : %s\n", strings.Join(p.Conflicts, " "))
}

func requireArgs(cmd string, args []string, min int) {
	if len(args) < min {
		fail(fmt.Sprintf("%s requires at least %d argument(s)", cmd, min))
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `forge - Arch compatible package manager for FronttierLinux

Usage:
  forge [global flags] install <package>...
  forge [global flags] remove <package>...
  forge [global flags] update
  forge [global flags] upgrade
  forge [global flags] list [repo]
  forge [global flags] info <package>
  forge [global flags] search <term>
  forge [global flags] clean [all]
  forge repo-add <repo-name> <package-file>...
  forge version 'prints compiled forge version'

Global flags:
  --root PATH       install root (default /)
  --dbpath PATH     local package database path
  --cachedir PATH   package cache path
  --config PATH     config file
  --arch ARCH       target architecture
  -v, --verbose     increase verbosity`)
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "forge:", msg)
	os.Exit(1)
}
