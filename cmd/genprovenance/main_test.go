package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// baseArgs returns the required flag set for one synthetic build; tests
// override individual flags.
func baseArgs(t *testing.T, dir string) []string {
	t.Helper()
	artifact := filepath.Join(dir, "uawp-linux-amd64")
	body := []byte("synthetic artifact bytes\n")
	if err := os.WriteFile(artifact, body, 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	goMod := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goMod, []byte("module example.test\n\ngo 1.25.14\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	goSum := filepath.Join(dir, "go.sum")
	if err := os.WriteFile(goSum, []byte("example.test hash v0.0.0 h1:AAA=\n"), 0o644); err != nil {
		t.Fatalf("write go.sum: %v", err)
	}
	return []string{
		"--artifact", artifact,
		"--name", "uawp-linux-amd64",
		"--revision", "1590935c17a7b1a1e2395738970a965a8b2ee401",
		"--ref", "refs/tags/v0.0.0-test",
		"--runner-label", "ubuntu-latest",
		"--runner-image", "ubuntu24-20261001.1",
		"--workflow", ".github/workflows/release-build.yml",
		"--workflow-commit", "1590935c17a7b1a1e2395738970a965a8b2ee401",
		"--run-id", "1234567890",
		"--job", "build linux-amd64",
		"--go-version", "go1.25.14",
		"--cc-version", "gcc (Ubuntu 13.3.0-6ubuntu2~24.04) 13.3.0",
		"--dependency-map-version", "1.18",
		"--go-mod", goMod,
		"--go-sum", goSum,
		"--cgo", "1",
		"--goos", "linux",
		"--goarch", "amd64",
		"--build-flags", "-trimpath",
		"--build-ldflags", "-X main.version=v0.0.0-test -X main.commit=1590935 -X main.buildDate=2026-10-30T00:00:00Z",
		"--build-date", "2026-10-30T00:00:00Z",
		"--out", filepath.Join(dir, "uawp-linux-amd64.provenance.json"),
	}
}

func loadRecord(t *testing.T, path string) Record {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("record must be valid JSON: %v (%q)", err, data)
	}
	return record
}

func TestEmitsRecordWithComputedDigest(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := run(baseArgs(t, dir), &stdout, &stderr); code != 0 {
		t.Fatalf("run: code=%d stderr=%q", code, stderr.String())
	}
	record := loadRecord(t, filepath.Join(dir, "uawp-linux-amd64.provenance.json"))

	body, err := os.ReadFile(filepath.Join(dir, "uawp-linux-amd64"))
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	sum := sha256.Sum256(body)
	want := hex.EncodeToString(sum[:])
	if record.Subject.Sha256 != want {
		t.Fatalf("subject digest must be over the artifact bytes: got %s want %s", record.Subject.Sha256, want)
	}
	if record.Predicate != "uawp-build-provenance/v1" || record.RecordType != "uawp-release-provenance" {
		t.Fatalf("predicate/record_type identity: %+v", record)
	}
	if record.Source.Revision != "1590935c17a7b1a1e2395738970a965a8b2ee401" || record.Source.Ref != "refs/tags/v0.0.0-test" {
		t.Fatalf("source binding: %+v", record.Source)
	}
	if record.Builder.RunnerLabel == "" || record.Builder.RunnerImage == "" || record.Builder.RunID == "" {
		t.Fatalf("builder identity incomplete: %+v", record.Builder)
	}
	if record.Toolchain.Go != "go1.25.14" || record.Toolchain.CCompiler == "" {
		t.Fatalf("toolchain incomplete: %+v", record.Toolchain)
	}
	if record.InputLock.DependencyMapVersion != "1.18" {
		t.Fatalf("input lock dependency-map version: %+v", record.InputLock)
	}
	// The input-lock digests are over the given go.mod/go.sum bytes, not
	// the artifact.
	modSum := sha256.Sum256([]byte("module example.test\n\ngo 1.25.14\n"))
	if record.InputLock.GoModSha256 != hex.EncodeToString(modSum[:]) {
		t.Fatalf("go.mod digest: %s", record.InputLock.GoModSha256)
	}
	if record.Environment.CGOEnabled != "1" || record.Environment.GOOS != "linux" || record.Environment.GOARCH != "amd64" {
		t.Fatalf("environment target: %+v", record.Environment)
	}
	joined := strings.Join(record.Environment.BuildFlags, " ")
	if !strings.Contains(joined, "-trimpath") || !strings.Contains(joined, "-ldflags") {
		t.Fatalf("build flags must carry the recipe: %+v", record.Environment.BuildFlags)
	}
	if record.Environment.BuildTimestampUTC != "2026-10-30T00:00:00Z" {
		t.Fatalf("build timestamp: %s", record.Environment.BuildTimestampUTC)
	}
	// The stdout digest line is the log audit record (C5-14 §4.2 item 5).
	if !strings.Contains(stdout.String(), "sha256 "+want) {
		t.Fatalf("stdout must carry the digest: %q", stdout.String())
	}
}

func TestDigestFollowsArtifactBytes(t *testing.T) {
	dir := t.TempDir()
	args := baseArgs(t, dir)
	if code := run(args, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("first run: code=%d", code)
	}
	first := loadRecord(t, filepath.Join(dir, "uawp-linux-amd64.provenance.json"))

	artifact := filepath.Join(dir, "uawp-linux-amd64")
	if err := os.WriteFile(artifact, []byte("different bytes\n"), 0o644); err != nil {
		t.Fatalf("rewrite artifact: %v", err)
	}
	if code := run(args, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("second run: code=%d", code)
	}
	second := loadRecord(t, filepath.Join(dir, "uawp-linux-amd64.provenance.json"))
	if first.Subject.Sha256 == second.Subject.Sha256 {
		t.Fatalf("digest must change with the artifact bytes")
	}
}

func TestMissingArtifactFails(t *testing.T) {
	dir := t.TempDir()
	args := baseArgs(t, dir)
	for i, a := range args {
		if a == filepath.Join(dir, "uawp-linux-amd64") {
			args[i] = filepath.Join(dir, "missing.bin")
			break
		}
	}
	var stderr bytes.Buffer
	if code := run(args, &bytes.Buffer{}, &stderr); code == 0 {
		t.Fatalf("a missing artifact must fail the run")
	}
}

func TestMissingRequiredFlagFails(t *testing.T) {
	dir := t.TempDir()
	args := baseArgs(t, dir)
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--revision" {
			args = append(args[:i:i+1], args[i+2:]...)
			break
		}
	}
	var stderr bytes.Buffer
	if code := run(args, &bytes.Buffer{}, &stderr); code != 2 {
		t.Fatalf("a missing required flag must exit 2, got %d (stderr %q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--revision is required") {
		t.Fatalf("stderr must name the missing flag: %q", stderr.String())
	}
}

func TestMalformedBuildDateFails(t *testing.T) {
	dir := t.TempDir()
	args := baseArgs(t, dir)
	for i, a := range args {
		if a == "2026-10-30T00:00:00Z" {
			args[i] = "not-a-timestamp"
			break
		}
	}
	var stderr bytes.Buffer
	if code := run(args, &bytes.Buffer{}, &stderr); code != 2 {
		t.Fatalf("a malformed build date must exit 2, got %d", code)
	}
}
