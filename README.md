# Forge-pkgm
Forge is an Arch/pacman-compatible package manager written in Go.

It is designed for Fronttier Linux, but because it is a single static binary and
uses pacman-compatible repository databases and package archives, it can
theoretically run on any Linux distribution where you point it at an isolated
root filesystem.

Status: alpha. Not yet production ready.

## First run: you MUST create a config file

Forge ships with **no repositories enabled**. If you run it without a config,
package resolution fails with:
```bash

    forge: cannot satisfy dependency "fastfetch"
```
That is not a bug. Forge does not know where to download from until you give it
a pacman-style config.

Create a config file:
```bash
    mkdir -p conf

    cat > conf/forge.conf <<'EOF'
    [options]
    Architecture = x86_64
    SyncInterval = 24h
    SigLevel = Never
    ParallelDownloads = 4
    NoExtract = usr/share/man usr/share/doc usr/share/info usr/share/locale usr/lib/locale usr/share/gtk-doc usr/share/help

    [core]
    Server = https://mirror.rackspace.com/archlinux/$repo/os/$arch
    Server = https://mirrors.edge.kernel.org/archlinux/$repo/os/$arch
    Server = https://mirror.pkgbuild.com/$repo/os/$arch

    [extra]
    Server = https://mirror.rackspace.com/archlinux/$repo/os/$arch
    Server = https://mirrors.edge.kernel.org/archlinux/$repo/os/$arch
    Server = https://mirror.pkgbuild.com/$repo/os/$arch
    EOF
```

Then run with `--config`:

    ./forge --config conf/forge.conf install fastfetch

Or install the config system-wide so you can omit `--config`:

    sudo mkdir -p /etc/forge
    sudo cp conf/forge.conf /etc/forge/forge.conf

Multiple mirrors are recommended. Some mirrors resolve to IPv6-only addresses
and will fail on hosts without IPv6 connectivity. Forge falls back to the next
Server line automatically.

---

## Always install into an isolated root

Forge is designed to install into an **empty directory**, not into your live
system. Create a fake root and pass it with `--root`:
```bash

    mkdir -p ~/fakeroot

    ./forge \
      --root ~/fakeroot \
      --config conf/forge.conf \
      install fastfetch
```
Your host is never touched. Files land under:

    ~/fakeroot/usr/
    ~/fakeroot/var/lib/forge/
    ~/fakeroot/var/cache/forge/

- **Do not run `./forge install ...` without `--root` on a system you care about**:
it would install Arch packages directly into your running system.

## Debian 

The Go toolchain package is called `golang`, not `go`:
```bash
    sudo apt install golang
```
After that, `go`, `go test`, and `go build` all work.

## What it can do now
- Refuse removal that would break installed dependencies (forge remove --nodeps to override).

- Preserve user configs on upgrade: modified Backup files are saved as .pacnew instead of being overwritten.

- Install and remove binary packages from pacman-style repositories.

- Resolve dependencies, including provides, conflicts, replaces, and versioned requirements like glibc>=2.39 or util-linux-libs=2.42.2.

- Expand package groups, so forge install base works.

- Read and parse pacman-compatible repository databases:.db, .db.tar.zst, .db.tar.xz, .db.tar.gz.

- Install into any target root with --root, so forge never touches your host.

- Use multiple mirrors with automatic fallback.

- Use Arch repositories, CachyOS repositories, or your own custom repo.

- Download packages in parallel (ParallelDownloads in forge.conf).

- Remove bloat at the source with NoExtract, such as man pages, docs, and
locales.

- Preserve setuid, setgid, and sticky bits from package archives.

- Track installed files in a local package database.

- Run package install scripts (.INSTALL) in a chroot: pre_install, post_install, pre_upgrade, post_upgrade, pre_remove, post_remove.

- Perform atomic transactions with an exclusive database lock, file backups, and rollback on failure.

- Prune the package cache with forge clean (or forge clean all for repo DBs
too).

## Why not just use pacman?
Forge is not a pacman fork and it does not shell out to pacman or libalpm.
It speaks the same repository/package format so it can reuse existing
pacman-compatible repositories without needing to repackage thousands of
packages.

For Fronttier, this means:

- Arch core and extra provide the base userspace.

- CachyOS repos can optionally provide optimized packages.

- A Fronttier fronttier repo can overlay distro-specific packages.

- Forge remains one static Go binary.

## Installation / building from source 
Forge is intentionally easy to build on any distro with a Go toolchain.

### Requirements
- Go 1.24 or newer

- Internet access for Go modules and package mirrors

- Linux kernel

### Build

```bash

git clone https://github.com/FronttierLabs/forge-pkgm.git
cd forge-pkgm

go test ./...

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w" \
  -o forge ./cmd/forge
```


```bash

./build.sh

```

- Or a plain static build:

```bash


CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o forge ./cmd/forge

Run the tests
go test ./...

```

- Static analysis and vulnerability checking are also supported:

```bash


go vet ./...
staticcheck ./...
govulncheck ./...

```

Install the binary
Run it from the build directory:

```bash


./forge --help

```

Or install it system-wide:

```bash


sudo install -m 0755 forge /usr/local/bin/forge
sudo mkdir -p /etc/forge

```

## Configuration
- Forge reads pacman-style configuration. The default path is /etc/forge/forge.conf.

```bash


[options]
Architecture = x86_64
SyncInterval = 24h
SigLevel = Never
ParallelDownloads = 4
XferCommand = /usr/bin/curl -sS -4 -L -C - --retry 5 --retry-delay 3 --connect-timeout 10 -o %o %u
NoExtract = usr/share/man usr/share/doc usr/share/info usr/share/locale usr/lib/locale usr/share/gtk-doc usr/share/help

[core]
Server = https://geo.mirror.pkgbuild.com/$repo/os/$arch

[extra]
Server = https://geo.mirror.pkgbuild.com/$repo/os/$arch

Multiple mirrors per repo are supported:

[core]
Server = https://mirror.rackspace.com/archlinux/$repo/os/$arch
Server = https://mirrors.edge.kernel.org/archlinux/$repo/os/$arch
Server = https://mirror.pkgbuild.com/$repo/os/$arch

CachyOS repositories are also supported:

[cachyos]
Server = https://mirror.cachyos.org/repo/x86_64/cachyos
```



## Usage

```bash



forge version
forge install zlib
forge remove zlib
forge update
forge upgrade
forge list
forge info zlib
forge search zlib
forge clean
forge clean all

```

- Install a group:

```bash


forge install base

```


- Install into an isolated root:

```bash

sudo mkdir -p /opt/test-root

sudo forge \
  --root /opt/test-root \
  --config /etc/forge/forge.conf \
  install zlib

```


- Files land under:

```bash


/opt/test-root/usr/...
/opt/test-root/var/lib/forge/...
/opt/test-root/var/cache/forge/...

```


- Why Forge can work on "any distro" Forge only needs:

- a Linux kernel

- a writable target sub-root

- compatible HTTP or file repository URLs (it can even use a local repo)

- **It does not need**:

- pacman

- libalpm

- Arch Linux as the host

- systemd

Because it is a static Go binary, you can copy forge to another system and run it directly.

- This makes Forge useful as a bootstrap tool: install Arch binary packages into an empty directory to build a new root  filesystem for another distro or container.

## Known **limitations**
- This is alpha software. Missing or incomplete features include:

- No package signature verification yet (SigLevel is parsed but not enforced).

- No partial-upgrade avoidance.

- Do not use Forge on a production system unless you accept those risks.

