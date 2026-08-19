# forge-pkgm

Forge is an Arch/pacman-compatible package manager written in Go.

It is designed for Fronttier Linux, but because it is a single static binary and
uses pacman compatible repository database and packages archives, it can
theoretically run on any Linux distribution where you point it at an isolated
root filesystem.

 > **Status:** pre-alpha. No production ready.

----------

## What it can do now

- Install and remove binary packages from pacman style repositories.
- Resolve dependencies, including `provides`, `conflicts`, and versioned
  requirements like `glibc>=2.39` or `util-linux-libs=2.42.2`.
- Read and parse pacman-compatible repository databases:
  `.db`, `.db.tar.zst`, `.db.tar.xz`, `.db.tar.gz`.
- Install into any target root with `--root`, so forge never touches your host.
- Use multiple mirrors with automatic fallback.
- Use Arch repositories, CachyOS repositories, or your own custom repo.
-remove bloat at the source, with `NoExtract`, such as man pages, docs,
  and locales.
- Preserve setuid, setgid, and sticky bits from package archives.
- Track installed files in a local package database.

--------

## Why not just use pacman?

Forge is not a pacman fork and it does not shell out to `pacman` or `libalpm`.
It speaks the same repository/package format so it can reuse existing
pacman-compatible repositories without needing to repackage thousands of
packages.

For Fronttier, this means:

- Arch `core` and `extra` provide the base userspace.
- CachyOS repos can optionally provide optimized packages.
- A Fronttier `fronttier` repo can overlay distro-specific packages.
- Forge remains one static Go binary.

--------------

## Installation / building from source

Forge is intentionally easy to build on any distro with a Go toolchain.

### Requirements

- Go 1.22 or newer
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
  
## Install the binary
- You can run it from the build directory:
```bash
./forge --help
```

## Or install it system-wide

```bash
sudo install -m 0755 forge /usr/local/bin/forge
sudo mkdir -p /etc/forge
```

# config 

if you want stability with forge use the default recomended '/etc/forge/forge.conf'

```bash
[options]
Architecture = x86_64
SyncInterval = 24h
SigLevel = Never
XferCommand = /usr/bin/curl -4 -L -C - --retry 5 --retry-delay 3 --connect-timeout 10 -o %o %u
NoExtract = usr/share/man usr/share/doc usr/share/info usr/share/locale usr/lib/locale usr/share/gtk-doc usr/share/help

[core]
Server = https://geo.mirror.pkgbuild.com/$repo/os/$arch

[extra]
Server = https://geo.mirror.pkgbuild.com/$repo/os/$arch
```
- if you want extra stability you can add multiple repo at the core config part in '/etc/forge/forge.conf'

```bash
[core]
Server = https://mirror.rackspace.com/archlinux/$repo/os/$arch
Server = https://mirrors.edge.kernel.org/archlinux/$repo/os/$arch
Server = https://mirror.pkgbuild.com/$repo/os/$arch
```
- cachyOS repositories are also supported
```bash
[cachyos]
Server = https://mirror.cachyos.org/repo/x86_64/cachyos
```

# Usage
```bash
forge install zlib
forge remove zlib
forge update
forge upgrade
forge list
forge info zlib
forge search zlib
```

## Install into an isolated root

```bash
sudo mkdir -p /opt/test-root

sudo forge \
  --root /opt/test-root \
  --config /etc/forge/forge.conf \
  install zlib
```
- now the files are under :

```bash
/opt/test-root/usr/...
/opt/test-root/var/lib/forge/...
/opt/test-root/var/cache/forge/...
```
- Why Forge can work on "any distro"
Forge only needs:

- a Linux kernel

- writable target sub root

- compatible HTTP or file repository URL's, it can even be local repo!

# It does not need:

- pacman

- libalpm

- Arch Linux as the host

- systemd

Because it is a static Go binary, you can copy forge to another system and
run it directly.

## This makes Forge useful as a bootstrap tool: install Arch binary packages into an empty directory to build a new root filesystem for another distro or container.

# Known limitations
- **This is pre-alpha software.** Missing or incomplete features include:

- No group expansion yet, such as forge install base base-devel.

- No package .INSTALL scripts yet.

- No transaction lock yet.

- No package signature verification yet.

- Upgrades are basic and are not yet fully atomic.

- Config files are not protected from package overwrites yet.

- Package cache pruning is not implemented.

- **Do not use Forge on a production system** unless you accept those risks.




