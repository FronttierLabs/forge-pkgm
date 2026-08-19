package scriptlet

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// install script, then invokes the requested hook if it is defined.
const wrapperTemplate = `#!/bin/sh
set -e
. /tmp/%s
if command -v "$1" >/dev/null 2>&1; then
	"$1" "$2" "$3"
fi
`

// Run executes a named install-script hook inside the target root.
//
// The raw install script is written to /tmp inside root, then a small wrapper
// sources it and calls the requested hook (pre_install, post_install,
// pre_upgrade, post_upgrade, pre_remove, post_remove) with oldVersion and
// newVersion as $1/$2. Missing hooks are skipped without error.
//
// The shell runs chrooted into root, so this requires root privileges and a
// working /bin/sh (or /usr/bin/bash) inside the target root.
func Run(ctx context.Context, root, raw, hook, oldVersion, newVersion string) error {
	if raw == "" {
		return nil
	}

	shell, err := shellPath(root)
	if err != nil {
		return err
	}
	if shell == "" {
		fmt.Fprintf(os.Stderr, "forge: warning: no shell available in %s; skipping hook %s\n", root, hook)
		return nil
	}

	tmpDir := filepath.Join(root, "tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return err
	}

	// Write the package install script under root/tmp. It is visible as
	// /tmp/<name> inside the chroot.
	scriptFile, err := os.CreateTemp(tmpDir, "forge-install-*.sh")
	if err != nil {
		return err
	}
	scriptRel := "/tmp/" + filepath.Base(scriptFile.Name())
	if _, err := scriptFile.WriteString(raw); err != nil {
		scriptFile.Close()
		os.Remove(scriptFile.Name())
		return err
	}
	if err := scriptFile.Close(); err != nil {
		os.Remove(scriptFile.Name())
		return err
	}
	defer os.Remove(filepath.Join(root, "tmp", filepath.Base(scriptRel)))

	// Write the hook wrapper next to it.
	wrapFile, err := os.CreateTemp(tmpDir, "forge-hook-*.sh")
	if err != nil {
		return err
	}
	wrapName := wrapFile.Name()
	wrapRel := "/tmp/" + filepath.Base(wrapName)
	if _, err := wrapFile.WriteString(fmt.Sprintf(wrapperTemplate, filepath.Base(scriptRel))); err != nil {
		wrapFile.Close()
		os.Remove(wrapName)
		return err
	}
	if err := wrapFile.Close(); err != nil {
		os.Remove(wrapName)
		return err
	}
	if err := os.Chmod(wrapName, 0o755); err != nil {
		os.Remove(wrapName)
		return err
	}
	defer os.Remove(wrapName)

	cmd := exec.CommandContext(ctx, shell, wrapRel, hook, oldVersion, newVersion)
	cmd.Dir = "/"
	cmd.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG=C",
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Chroot: root}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("scriptlet %s: %w\n%s", hook, err, out)
	}
	return nil
}

// shellPath returns the path of a usable shell.
func shellPath(root string) (string, error) {
	bash := filepath.Join(root, "usr/bin/bash")
	if fi, err := os.Stat(bash); err == nil && !fi.IsDir() {
		return "/usr/bin/bash", nil
	}

	sh := filepath.Join(root, "bin/sh")
	if fi, err := os.Stat(sh); err == nil && !fi.IsDir() {
		return "/bin/sh", nil
	}

	return "", nil
}
