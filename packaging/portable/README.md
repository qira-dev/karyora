# Karyora Core — portable archive (Linux)

This archive is the C5-15 §3.3 Option A portable artifact for the SAME
supported Linux families as the native packages: glibc >= 2.34 on
x86_64 or aarch64 (Ubuntu LTS 22.04/24.04, Debian 12+, RHEL 9+). It is
NOT an escape hatch for unsupported platforms: musl/Alpine and every
unlisted distribution or libc combination is unsupported by rule and
gets no artifact (C5-11).

## Contents

- `uawp-linux-<arch>` — the Karyora Core binary (self-contained: the
  CGO-linked SQLite engine ships inside it; you never install Go,
  Node.js, PostgreSQL, Redis, NATS, or Docker — D-07).
- `PORTABLE-README.md` — this file.

## Verify before you install (stop on ANY failure — never a warning)

1. **Checksum**: `sha256sum uawp-linux-<arch>` must equal the digest
   published beside this archive for your row.
2. **Signature**: the release signature check (the Linux GPG release
   key, signature-THEN-digest, fail-stop) is the release-day signing
   stage and is documented with the first SIGNED release. Until then
   the artifacts are UNSIGNED BY DESIGN and only the checksum step
   applies — this line is updated when signing ships (C5-13 §9.2).

## Install

1. Extract the archive to a directory of your choice.
2. Make the binary executable: `chmod 755 uawp-linux-<arch>`.
3. Put it on your PATH (copy or link it into a directory already on
   PATH), or run it by its path.
4. First run: `uawp-linux-<arch> version` — it prints the build identity
   (version, commit, build date).

## Data

Your data lives at OS-standard user application-data locations — the
portable archive never writes outside the directory you extract it to
until you run the product. Removing this archive or the binary leaves
your data untouched; explicit data deletion is a separate, deliberate,
documented act.

## Supported environments

Exactly the six release-matrix rows are supported; a below-floor or
unlisted environment is documented as unsupported and untested — the
package neither checks nor fakes a support claim (C5-15 §3.5).
