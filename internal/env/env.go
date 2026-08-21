package env

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"forge/internal/config"
	"forge/internal/db"
	"forge/internal/fetch"
	"forge/internal/install"
)

// baseDir is where all forge envs live.
const BaseDir = "/opt/forge/env"

// root returns the root filesystem for a named env.
func Root(name string) string {
	return filepath.Join(BaseDir, name)
}

// exists reports whether the env directory already exists.
func Exists(name string) bool {
	fi, err := os.Stat(Root(name))
	return err == nil && fi.IsDir()
}

// create installs packages and their dependency closure into a new
// the host package cache is shared only the local DB is env-specific.
func Create(cfg *config.Config, fetcher *fetch.Fetcher, syncDBs []*db.RepoDB, name string, targets []string) error {
	root := Root(name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}

	envCfg := *cfg
	envCfg.Root = root
	envCfg.DBPath = filepath.Join(root, "var/lib/forge")
	// cacheDir remains the host cache so downloads are shared across envs.

	// every shell env needs a shell and the filesystem layout (/bin -> usr/bin).
	base := []string{"filesystem", "bash"}
	seen := make(map[string]bool, len(base)+len(targets))
	merged := make([]string, 0, len(base)+len(targets))
	for _, t := range append(base, targets...) {
		if !seen[t] {
			seen[t] = true
			merged = append(merged, t)
		}
	}

	return install.Run(context.Background(), &envCfg, fetcher, syncDBs, merged)
}

// list returns the names of all envs, sorted in lists
func List() ([]string, error) {
	entries, err := os.ReadDir(BaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}

	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}

	return names, nil
}

// remove deletes an env and its contents.
func Remove(name string) error {
	root := Root(name)
	if !Exists(name) {
		return fmt.Errorf("env %q does not exist", name)
	}
	return os.RemoveAll(root)
}

// purges deletes every env.
func Purge() error {
	names, err := List()
	if err != nil {
		return err
	}
	for _, n := range names {
		if err := os.RemoveAll(Root(n)); err != nil {
			return err
		}
	}
	return nil
}

// mountSpecials intentionally does not bind-mount /dev, /proc, or /sys.
// A plain chroot is enough for shell/TUI programs, and staying out of the host
// mount namespace avoids the post-exit cleanup problems that bind mounts caused.
// The env root gets empty dev/proc/sys directories so path expectations hold.
func mountSpecials(root string) (func(), error) {
	for _, d := range []string{"dev", "proc", "sys"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			return func() {}, err
		}
	}
	return func() {}, nil
}

// Enter drops the user into an interactive shell inside the env.
func Enter(name string) error {
	root := Root(name)
	if !Exists(name) {
		return fmt.Errorf("env %q does not exist", name)
	}

	cleanup, err := mountSpecials(root)
	if err != nil {
		return err
	}
	defer cleanup()

	cmd := exec.Command("/bin/bash", "-l")
	cmd.Dir = "/"
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/root",
		"TERM=" + os.Getenv("TERM"),
		"PS1=[forge:" + name + "] \\u@\\h:\\w\\$ ",
		"DISPLAY=",
		"WAYLAND_DISPLAY=",
		"XDG_RUNTIME_DIR=",
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Chroot: root}

	return cmd.Run()
}

// Run executes a command (non-interactively) inside the env.
func Run(name string, args ...string) error {
	root := Root(name)
	if !Exists(name) {
		return fmt.Errorf("env %q does not exist", name)
	}
	if len(args) == 0 {
		return fmt.Errorf("env run requires a command")
	}

	cleanup, err := mountSpecials(root)
	if err != nil {
		return err
	}
	defer cleanup()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = "/"
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/root",
		"TERM=" + os.Getenv("TERM"),
		"DISPLAY=",
		"WAYLAND_DISPLAY=",
		"XDG_RUNTIME_DIR=",
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Chroot: root}

	return cmd.Run()
}
