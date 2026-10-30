# Karyora — User Guide

This guide describes the Karyora desktop application as it is shipped today. It states only what the product actually does; capabilities that do not exist yet are listed as limitations, never promised. The same guide is available in 12 languages in this folder.

## 1. What Karyora is

- Karyora is a local-first desktop application. Your knowledge packs, jobs, and settings are stored on your own machine.
- The product works without any cloud service of the project. The only network traffic it can generate is traffic you initiate: requests to your AI providers, a knowledge-pack import you request, and — when you configure it — the update channel.
- AI features use your own provider accounts (bring your own key, BYOK). You configure your own API keys, and the provider bills you directly. Karyora never resells provider usage and never acts as a middleman for provider payments.

## 2. Community and official releases

- Karyora's source code is released under the Apache License 2.0. You may use, study, modify, and redistribute it under that license, including building it yourself.
- The public repository is `https://github.com/qira-dev/karyora`. Each release is marked by a `v*` tag, and the tag's source tree is that release's source.
- An "official" release is one that was built by the project's own build pipeline at a tagged revision, with recorded evidence. Official builds and community builds have the same features; the difference is who built the artifact and who stands behind it, never what the artifact can do.
- In v1 there is no official download page and no official update channel yet. Until they are published, you can obtain Karyora from the public repository and build it from source.

## 3. Supported platforms (v1)

| OS | CPU | Floor |
|---|---|---|
| Windows 11 | x86_64 | Windows 11, 23H2 or newer |
| Windows 11 | ARM64 | Windows 11 on ARM |
| macOS | x86_64 | macOS 15 (Sequoia) or newer |
| macOS | ARM64 | macOS 15 (Sequoia) or newer, Apple Silicon |
| Linux | x86_64 | glibc 2.34 or newer; Ubuntu LTS 22.04/24.04, Debian 12+, RHEL 9+ |
| Linux | ARM64 | glibc 2.34 or newer; the same distribution families |

Explicitly not supported in v1: Windows 10; macOS 14 or older; musl-based distributions such as Alpine; every combination not listed above. Support follows each vendor's own security-support window and is re-checked at every release.

## 4. Packages and how to verify them

- The produced package set is: Windows MSI, macOS pkg, Linux deb, Linux rpm, and a Linux portable archive — one artifact per supported CPU architecture.
- Every package is produced together with a `SHA256SUMS` manifest and per-artifact SBOM and provenance records. To verify a package: compute the sha256 digest of your download and compare it with the manifest entry. If the digest does not match, do not install the package.
- Packages are not digitally signed in v1 — by design. Platform signature-verification steps (OS-level signature checks, repository keys) will be documented together with the first signed release. Until then, the digest comparison above is the verification you can perform.
- The `LICENSE` and `NOTICE` files, which carry the project's license and third-party attributions, are part of the source repository.

## 5. Updates

- Karyora contains an update mechanism, and it ships disabled by default in v1. No update channel is configured, so an update check reports a typed refusal naming that no endpoint is configured. This is intentional: the official update channel will be switched on only when it is published.
- Until then, upgrading means installing the newer package over the older one. Your data is preserved across upgrades (see section 6).

## 6. Your data

- All product data lives on your machine. The default data directory is `%APPDATA%\Karyora\data` on Windows, `~/Library/Application Support/Karyora/data` on macOS, and `~/.local/share/karyora/data` on Linux (the XDG `XDG_DATA_HOME` setting is respected). You can choose a different location with the `--data-dir` flag.
- The product does not perform scheduled backups. To back up, stop the product and copy the data directory. Restoring means putting the copied directory back in place. There is no cloud backup.
- Uninstalling or upgrading does not delete your data. Deleting data is a separate, explicit action that you perform yourself.
- Data at rest is not encrypted. Protect the data directory with your own means, such as full-disk encryption. This is a known limitation, stated plainly.
- Provider credentials (your API keys) are stored only in your operating system's keyring. They are never written to files, logs, or prompt content.

## 7. Privacy

- The product does not include telemetry: nothing is sent to the project's servers.
- Diagnostics are produced locally and stay privacy-safe by default; you control what, if anything, leaves your machine.

## 8. Known limitations in v1

- Third-party plugin execution is disabled.
- Team and remote operation are not available.
- Transferring an active job from one machine to another is not supported.
- Portable knowledge-pack sync is not available.
- The "Work" grouping feature does not exist.
- There are no paid subscriptions and no entitlements: nothing to buy, no paid tiers.
- No numeric performance promises are made.
- Data at rest is not encrypted (see section 6).
- There is no official download page or update channel yet (see sections 2 and 5).

## 9. License

- Karyora is released under the Apache License 2.0. The complete license text is in the `LICENSE` file, and third-party attributions are in the `NOTICE` file, both in the source repository.
- Building Karyora yourself, forking it, or redistributing it under the license keeps every right the license grants. This right does not depend on anything you buy, because there is nothing to buy.

## 10. Command-line reference

The command-line tool is `uawp`.

| Command | What it does |
|---|---|
| `uawp start` | Starts the local service. A second start on the same data directory is refused while one is already running. |
| `uawp stop` | Stops the service with a bounded, graceful shutdown. |
| `uawp status` | Reports whether the service is running. |
| `uawp doctor` | Runs local health and diagnostics checks. |
| `uawp update check` / `uawp update apply` / `uawp update status` | Checks for, applies, or reports updates. In v1, with no channel configured, check and apply refuse with a typed refusal naming that no endpoint is configured; `status` reports the update state. |
| `uawp job submit` / `uawp job status` / `uawp job events` | Submits a durable job, reports its status, or follows its events. |
| `uawp credential store --provider <id> [--name api-key]` | Stores your provider API key in the OS keyring. |
| `uawp credential status` | Reports whether a credential is present — never its value. |
| `uawp credential remove` | Removes the stored credential. |
| `uawp version` / `uawp help` | Prints the version or the help. |

A stored credential by itself opens nothing: a live provider call still has to pass the product's data-handling rules for that provider.
