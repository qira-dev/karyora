// Command genprovenance emits the C5-14 §5.1 v1 provenance predicate record
// for one release-build artifact: the subject name and its computed sha256,
// the source revision, the builder identity, the toolchain versions, the
// input-lock reference, the build recipe, and the environment facts. The
// record is a provenance-style attestation only — no SLSA or
// reproducible-build claim is made or representable in it (C5-14 §5.4), and
// it never carries credentials or key material (C5-13 §9.1). The release
// pipeline's independent verification stage (C5-14 §6 stage 3) recomputes
// the digest and checks the recorded fields.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// predicateVersion names the v1 provenance predicate this tool emits. The
// name deliberately stays outside any SLSA vocabulary: C5-14 §5.4 withholds
// SLSA and reproducible-build claims for v1.
const predicateVersion = "uawp-build-provenance/v1"
const recordType = "uawp-release-provenance"

// Subject names the artifact and its content digest (the publishable
// identity; C5-14 §3).
type Subject struct {
	Name   string `json:"name"`
	Sha256 string `json:"sha256"`
}

// Source pins the exact revision the artifact was built from.
type Source struct {
	Revision string `json:"revision"`
	Ref      string `json:"ref"`
}

// Builder identifies the ephemeral build environment (C5-14 §4.2: one job =
// one target = one ephemeral environment; SHA-pinned actions).
type Builder struct {
	RunnerLabel    string `json:"runner_label"`
	RunnerImage    string `json:"runner_image"`
	Workflow       string `json:"workflow"`
	WorkflowCommit string `json:"workflow_commit"`
	RunID          string `json:"run_id"`
	Job            string `json:"job"`
}

// Toolchain records the Go and C toolchain versions (record-and-verify at
// each release cut, C5-14 §7.3).
type Toolchain struct {
	Go        string `json:"go"`
	CCompiler string `json:"c_compiler"`
}

// InputLock binds the record to the C5-12 release-input lock: the
// dependency-map contract version and the go.mod/go.sum content digests at
// the revision.
type InputLock struct {
	DependencyMapVersion string `json:"dependency_map_version"`
	GoModSha256          string `json:"go_mod_sha256"`
	GoSumSha256          string `json:"go_sum_sha256"`
}

// Environment carries the build facts (CGO, target, flags, timestamp).
type Environment struct {
	CGOEnabled        string   `json:"cgo_enabled"`
	GOOS              string   `json:"goos"`
	GOARCH            string   `json:"goarch"`
	BuildFlags        []string `json:"build_flags"`
	BuildTimestampUTC string   `json:"build_timestamp_utc"`
}

// Record is the complete §5.1 record; field names are stable machine
// identifiers.
type Record struct {
	RecordType  string      `json:"record_type"`
	Predicate   string      `json:"predicate"`
	Subject     Subject     `json:"subject"`
	Source      Source      `json:"source"`
	Builder     Builder     `json:"builder"`
	Toolchain   Toolchain   `json:"toolchain"`
	InputLock   InputLock   `json:"input_lock"`
	Environment Environment `json:"environment"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("genprovenance", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		artifact    = fs.String("artifact", "", "path of the built artifact (required; its sha256 is computed)")
		name        = fs.String("name", "", "subject name (default: the artifact file name)")
		revision    = fs.String("revision", "", "source revision (git SHA) (required)")
		ref         = fs.String("ref", "", "source ref the build ran on (required)")
		runnerLabel = fs.String("runner-label", "", "hosted runner label (required)")
		runnerImage = fs.String("runner-image", "", "runner image identifier (required)")
		workflow    = fs.String("workflow", "", "build recipe workflow path (required)")
		workflowCmt = fs.String("workflow-commit", "", "the workflow's executing commit (required)")
		runID       = fs.String("run-id", "", "workflow run id (required)")
		job         = fs.String("job", "", "build job name (required)")
		goVersion   = fs.String("go-version", "", "Go toolchain version (required)")
		ccVersion   = fs.String("cc-version", "", "C compiler version (required)")
		depMapVer   = fs.String("dependency-map-version", "", "dependency-map contract version (required)")
		goMod       = fs.String("go-mod", "", "path of go.mod (required; digest is computed)")
		goSum       = fs.String("go-sum", "", "path of go.sum (required; digest is computed)")
		cgo         = fs.String("cgo", "", "CGO_ENABLED value (required)")
		goos        = fs.String("goos", "", "GOOS (required)")
		goarch      = fs.String("goarch", "", "GOARCH (required)")
		buildFlags  = fs.String("build-flags", "", "comma-separated non-linker build flags")
		ldflags     = fs.String("build-ldflags", "", "the -ldflags value used (stamped identity)")
		buildDate   = fs.String("build-date", "", "build start timestamp, RFC3339 UTC (required)")
		out         = fs.String("out", "", "output path of the record JSON (required)")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	required := map[string]string{
		"artifact": *artifact, "revision": *revision, "ref": *ref,
		"runner-label": *runnerLabel, "runner-image": *runnerImage,
		"workflow": *workflow, "workflow-commit": *workflowCmt,
		"run-id": *runID, "job": *job,
		"go-version": *goVersion, "cc-version": *ccVersion,
		"dependency-map-version": *depMapVer,
		"go-mod":                 *goMod, "go-sum": *goSum,
		"cgo": *cgo, "goos": *goos, "goarch": *goarch,
		"build-date": *buildDate, "out": *out,
	}
	for fname, v := range required {
		if v == "" {
			fmt.Fprintf(stderr, "genprovenance: --%s is required\n", fname)
			return 2
		}
	}
	if _, err := time.Parse(time.RFC3339, *buildDate); err != nil {
		fmt.Fprintf(stderr, "genprovenance: --build-date must be RFC3339: %v\n", err)
		return 2
	}

	artifactDigest, err := fileSha256(*artifact)
	if err != nil {
		fmt.Fprintf(stderr, "genprovenance: %v\n", err)
		return 1
	}
	goModDigest, err := fileSha256(*goMod)
	if err != nil {
		fmt.Fprintf(stderr, "genprovenance: %v\n", err)
		return 1
	}
	goSumDigest, err := fileSha256(*goSum)
	if err != nil {
		fmt.Fprintf(stderr, "genprovenance: %v\n", err)
		return 1
	}

	subject := *name
	if subject == "" {
		subject = filepath.Base(*artifact)
	}

	flags := []string{}
	for _, f := range strings.Split(*buildFlags, ",") {
		if f = strings.TrimSpace(f); f != "" {
			flags = append(flags, f)
		}
	}
	if *ldflags != "" {
		flags = append(flags, "-ldflags", *ldflags)
	}

	record := Record{
		RecordType: recordType,
		Predicate:  predicateVersion,
		Subject:    Subject{Name: subject, Sha256: artifactDigest},
		Source:     Source{Revision: *revision, Ref: *ref},
		Builder: Builder{
			RunnerLabel: *runnerLabel, RunnerImage: *runnerImage,
			Workflow: *workflow, WorkflowCommit: *workflowCmt,
			RunID: *runID, Job: *job,
		},
		Toolchain: Toolchain{Go: *goVersion, CCompiler: *ccVersion},
		InputLock: InputLock{
			DependencyMapVersion: *depMapVer,
			GoModSha256:          goModDigest, GoSumSha256: goSumDigest,
		},
		Environment: Environment{
			CGOEnabled: *cgo, GOOS: *goos, GOARCH: *goarch,
			BuildFlags:        flags,
			BuildTimestampUTC: *buildDate,
		},
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "genprovenance: marshal record: %v\n", err)
		return 1
	}
	data = append(data, '\n')
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintf(stderr, "genprovenance: write record: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "subject %s sha256 %s\nrecord %s\n", subject, artifactDigest, *out)
	return 0
}

// fileSha256 computes the sha256 over the file's bytes in bounded chunks.
func fileSha256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
