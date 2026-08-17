package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

type Repo struct {
	Name    string
	Servers []string
}

type Config struct {
	Root         string
	Architecture string
	DBPath       string
	CacheDir     string
	SyncInterval time.Duration
	SigLevel     string
	GPGDir       string
	Keyring      string
	XferCommand  string
	NoExtract    []string
	Repos        []Repo
}

func Parse(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg := &Config{
		Architecture: "x86_64",
		SigLevel:     "Never",
	}

	repos := map[string]*Repo{}
	repoOrder := []string{}

	addRepo := func(name string) *Repo {
		if r, ok := repos[name]; ok {
			return r
		}
		repos[name] = &Repo{Name: name}
		repoOrder = append(repoOrder, name)
		return repos[name]
	}

	addServer := func(name, server string) {
		r := addRepo(name)
		r.Servers = append(r.Servers, server)
	}

	section := ""
	sc := bufio.NewScanner(f)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}

		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)

		repoName := ""
		isRepo := false

		switch {
		case strings.HasPrefix(section, "repo "):
			repoName = strings.TrimSpace(strings.TrimPrefix(section, "repo "))
			isRepo = repoName != ""
		case section != "" && !strings.EqualFold(section, "options"):
			repoName = section
			isRepo = true
		}

		if isRepo {
			switch key {
			case "Server":
				if val != "" {
					addServer(repoName, val)
				}
			case "Include":
				if err := includeServers(val, repoName, addServer); err != nil {
					return nil, fmt.Errorf("include %s: %w", val, err)
				}
			}
			continue
		}

		switch key {
		case "Root":
			cfg.Root = val
		case "Architecture":
			cfg.Architecture = val
		case "DBPath":
			cfg.DBPath = val
		case "CacheDir":
			cfg.CacheDir = val
		case "SyncInterval":
			if d, err := time.ParseDuration(val); err == nil {
				cfg.SyncInterval = d
			}
		case "SigLevel":
			cfg.SigLevel = val
		case "GPGDir":
			cfg.GPGDir = val
		case "Keyring":
			cfg.Keyring = val
		case "XferCommand":
			cfg.XferCommand = val
		case "NoExtract":
			cfg.NoExtract = append(cfg.NoExtract, strings.Fields(val)...)
		}
	}

	if err := sc.Err(); err != nil {
		return nil, err
	}

	for _, name := range repoOrder {
		cfg.Repos = append(cfg.Repos, *repos[name])
	}

	return cfg, nil
}

func includeServers(path, repoName string, addServer func(string, string)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "Server") {
			_, val, ok := strings.Cut(line, "=")
			if ok && strings.TrimSpace(val) != "" {
				addServer(repoName, strings.TrimSpace(val))
			}
		}
	}

	return sc.Err()
}
