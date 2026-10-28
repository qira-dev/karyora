# Build and Dependency Evidence Protocol (Phase 2, C2-03)

Contract version 1. Implements the Core Structure Roadmap CR-2.1 pin rule and the
workstream rule "prove and pin the toolchain before the first product build".
Changes require a version bump and an ADR.

## Proof-before-pin protocol

1. **Candidate selection.** Each Go toolchain and third-party dependency is named
   with a candidate version before any proof runs.
2. **Proof.** The candidate passes, in order: compatibility (supported Go version,
   D-06 target build), target-build (compile for each relevant target row),
   security/vulnerability review (advisories, `govulncheck`), license review
   (compatible, obligations understood), and provenance (official source, recorded
   digest). Proof outputs are dated and stored with the pin record.
3. **Pin.** Only after proof: exact versions are written to `go.mod`/`go.sum`
   (checksums), and a pin record is added to the repository evidence set.
4. **Build.** The first product Core build happens only after the pin. Phase 5
   revalidates release inputs; it never defers the initial pin.
5. No version is treated as permanent: upgrading repeats the proof, and any change
   is recorded. A scaffold check (gofmt/vet/build on the skeleton) is not a product
   build and does not require a pin.

## Pin/evidence record fields

| Field | Content |
|---|---|
| Component | Toolchain or dependency name |
| Version | Exact version + checksums (go.sum digest set, toolchain archive digest) |
| Proof | Dated commands and summarized outputs: build, `govulncheck`, license review, source URL |
| Decision | Approving role and date; ADR reference if a proposed baseline was changed |
| Revalidation | Phase 5 recheck date/owner |

## Build inputs and artifact identity

Every build captures: source revision (commit), environment (Go version, OS/arch),
dependency checksum set, and the artifact digest. Version metadata is stamped via
`-ldflags -X` (version, commit, build date); the scaffold `version` constant in
`cmd/uawp` is replaced by stamped metadata, never hand-edited for releases.

## License, SBOM, and static controls

- SBOM: SPDX JSON generated per build artifact; license inventory reviewed on every
  dependency change; an unknown or incompatible license fails the build.
- Static analysis: `go vet` and the dependency-rule test (`tests/deps`) are gates;
  additional analyzers are added through the pin process with owned outputs.
- Race/fuzz: the race detector runs on the test suite in CI; fuzz targets are added
  with the first nontrivial parser (CAS ingest, config loading) and run in scheduled
  CI before Phase 3 relies on them.
- Failure policy: any gate failure blocks the merge/release; exceptions are recorded
  per the security baseline's release-exception process.

## Cross-target evidence (D-06 matrix)

Compile, link, runtime smoke, and packaging are recorded as four distinct evidence
kinds per target row. A Go-only cross-build is never represented as proof for a CGO
dependency (D-09); native per-target runners or a verified C cross-toolchain plus
linked-runtime checks are required (C2-04/CR-2.3). Target architecture is not a
release-support claim (D-30).

## CI

`.github/workflows/ci.yml` runs on push/PR with four jobs: **verify** (gofmt
check, `go vet ./...`, `go build ./...`, and `go test -race ./...`, which
includes the dependency-rule test), **security** (the Rule 14 pipeline
mirror: gitleaks, semgrep, and trivy with pinned versions; the live and
injection stages stay Not Run — there is no live target), **cross-target**
(the compile-only D-06 six-row matrix with CGO disabled; per D-09 the CGO-bound
SQLite driver and the composition root that imports it build only in the
native verify job, and their cross-target linked-runtime evidence remains the
recorded Phase 5 work), and **plugin-runtime-conformance** (the C4-28
runtime evidence job: the internal/pluginhost enforcement fixtures and
internal/plugins contract tests execute on hosted runners for every D-06
row that has one — linux amd64/arm64, darwin amd64/arm64, windows amd64,
windows arm64 via the windows-11-arm public-repo runner — measuring the
enforced plugin boundary per target; if GitHub retires a runner label the
affected row degrades to compile-only and the workflow comment says so).
Workflow runs are inspected per push; artifact retention follows the
default GitHub policy until a dedicated evidence-retention task extends it.
