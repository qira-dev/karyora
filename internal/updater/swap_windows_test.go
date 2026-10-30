//go:build windows

package updater

// The Windows activator's rename-aside mechanism (the S7/S9 port's Windows
// implementation) exercised over plain files in a temp install location —
// the CI windows runners execute these; the linux/darwin suite skips them.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeBinary(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsSwapRetainsReplacesAndRestores(t *testing.T) {
	dir := t.TempDir()
	install := filepath.Join(dir, "current-uawp.exe")
	retain := filepath.Join(dir, "retained-uawp.exe")
	staged := filepath.Join(dir, "candidate")
	writeBinary(t, install, "OLD-BYTES")
	writeBinary(t, staged, "NEW-BYTES")
	digest, err := fileSHA256(staged)
	if err != nil {
		t.Fatal(err)
	}
	a := ReplaceActivator{}
	if err := a.Swap(context.Background(), SwapRequest{
		StagedPath: staged, InstallPath: install, RetainPath: retain, ExpectedNewDigest: digest,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(install)
	if err != nil || string(got) != "NEW-BYTES" {
		t.Fatalf("install = %q (%v); want the new bytes", got, err)
	}
	got, err = os.ReadFile(retain)
	if err != nil || string(got) != "OLD-BYTES" {
		t.Fatalf("retain = %q (%v); want the retained prior bytes", got, err)
	}
	// S9: the retained prior binary goes back — binary only.
	if err := a.Restore(context.Background(), retain, install); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(install)
	if err != nil || string(got) != "OLD-BYTES" {
		t.Fatalf("install after restore = %q (%v); want the prior bytes", got, err)
	}
}

func TestWindowsSwapFailureRestoresOriginal(t *testing.T) {
	dir := t.TempDir()
	install := filepath.Join(dir, "current-uawp.exe")
	retain := filepath.Join(dir, "retained-uawp.exe")
	writeBinary(t, install, "OLD-BYTES")
	// The staged candidate is missing: the first rename (aside) succeeds and
	// the second fails — the mechanism must put the original back.
	a := ReplaceActivator{}
	if err := a.Swap(context.Background(), SwapRequest{
		StagedPath: filepath.Join(dir, "missing"), InstallPath: install, RetainPath: retain,
		ExpectedNewDigest: "00",
	}); err == nil {
		t.Fatal("the swap should fail on a missing candidate")
	}
	got, err := os.ReadFile(install)
	if err != nil || string(got) != "OLD-BYTES" {
		t.Fatalf("install = %q (%v); want the original back after the failed swap", got, err)
	}
}

func TestWindowsRestoreFailsWithoutRetainedBinary(t *testing.T) {
	dir := t.TempDir()
	install := filepath.Join(dir, "current-uawp.exe")
	retain := filepath.Join(dir, "absent-uawp.exe")
	writeBinary(t, install, "NEW-BYTES")
	a := ReplaceActivator{}
	if err := a.Restore(context.Background(), retain, install); err == nil {
		t.Fatal("the restore should fail when the retained prior binary is unavailable")
	}
	// The failure never discards the installed bytes (never a data discard).
	got, err := os.ReadFile(install)
	if err != nil || string(got) != "NEW-BYTES" {
		t.Fatalf("install = %q (%v); want the installed bytes intact", got, err)
	}
}
