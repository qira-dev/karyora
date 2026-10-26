package platform

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveLinuxXDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg-data")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-config")
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg-cache")
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg-state")
	t.Setenv("XDG_RUNTIME_DIR", "/tmp/xdg-run")
	t.Setenv("HOME", "/tmp/xdg-home")
	ds, err := Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	want := DirSet{
		Data:    "/tmp/xdg-data/karyora/data",
		Config:  "/tmp/xdg-config/karyora",
		Cache:   "/tmp/xdg-cache/karyora",
		State:   "/tmp/xdg-state/karyora",
		Temp:    "/tmp/xdg-cache/karyora/tmp",
		Runtime: "/tmp/xdg-run/karyora",
	}
	if ds != want {
		t.Fatalf("resolution mismatch:\n got %+v\nwant %+v", ds, want)
	}
}

func TestResolveRuntimeFallback(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("HOME", t.TempDir())
	ds, err := Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	if ds.Runtime == "" {
		t.Fatal("runtime fallback must resolve")
	}
	if filepath.Base(ds.Runtime) != "karyora" {
		t.Fatalf("runtime fallback should stay in the karyora namespace: %s", ds.Runtime)
	}
}

func TestResolveOverrides(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	data := filepath.Join(t.TempDir(), "custom-data")
	rt := filepath.Join(t.TempDir(), "custom-runtime")
	ds, err := Resolve(&Overrides{DataDir: data, RuntimeDir: rt})
	if err != nil {
		t.Fatal(err)
	}
	if ds.Data != data || ds.Runtime != rt {
		t.Fatalf("overrides must apply: %+v", ds)
	}
}

func TestEnsureDirCreatesWithMode(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "a", "b", "c")
	if err := EnsureDir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o700 {
		t.Fatalf("mode = %o, want 700", st.Mode().Perm())
	}
	// Idempotent on an existing directory.
	if err := EnsureDir(target, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureDirRefusesSymlinkComponent(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	err := EnsureDir(filepath.Join(link, "child"), 0o700)
	if err == nil {
		t.Fatal("symlink component must be refused, never followed")
	}
}

func TestEnsureDirRefusesFileComponent(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDir(filepath.Join(file, "child"), 0o700); err == nil {
		t.Fatal("a non-directory component must be refused")
	}
}

func TestEnsureLayoutModes(t *testing.T) {
	tmp := t.TempDir()
	ds := DirSet{Data: filepath.Join(tmp, "data"), Config: filepath.Join(tmp, "config"), State: filepath.Join(tmp, "state"), Runtime: filepath.Join(tmp, "runtime")}
	if err := EnsureLayout(ds); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{ds.Data, ds.Config, ds.State, ds.Runtime} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s broader than 0700: %o", p, st.Mode().Perm())
		}
	}
}

func TestAcquireOwnershipAndConflict(t *testing.T) {
	tmp := t.TempDir()
	ds := DirSet{Data: filepath.Join(tmp, "data"), Runtime: filepath.Join(tmp, "runtime")}
	if err := os.MkdirAll(ds.Data, 0o700); err != nil {
		t.Fatal(err)
	}
	own, tookOver, err := AcquireOwnership(ds, ds.Data, "test-version")
	if err != nil {
		t.Fatal(err)
	}
	if tookOver {
		t.Fatal("first acquisition must not be a takeover")
	}
	if own.Record.PID != os.Getpid() {
		t.Fatalf("record PID = %d, want %d", own.Record.PID, os.Getpid())
	}
	if own.Record.InstanceUUID == "" || own.Record.DataDirIdentity == "" {
		t.Fatalf("record identity fields must be set: %+v", own.Record)
	}
	if runtime.GOOS == "linux" {
		if own.Record.BootID == "" {
			t.Fatalf("boot identifier must be recorded on linux: %+v", own.Record)
		}
		if own.Record.ProcessStartTime == "" {
			t.Fatalf("process start time must be recorded on linux: %+v", own.Record)
		}
	}
	if own.Record.CoreVersion != "test-version" {
		t.Fatalf("core version mismatch: %+v", own.Record)
	}
	// Capability token exists with 0600.
	tokenPath := filepath.Join(ds.Runtime, CapabilityTokenID)
	st, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("token mode = %o, want 600", st.Mode().Perm())
	}
	if len(own.Capability()) == 0 {
		t.Fatal("capability token must be non-empty")
	}

	// A concurrent start loses the atomic race: SINGLE_INSTANCE_CONFLICT.
	if _, _, err := AcquireOwnership(ds, ds.Data, "test-version"); !errors.Is(err, ErrLockHeld) {
		t.Fatalf("second acquire err = %v, want ErrLockHeld", err)
	}

	// Release lets the next owner in with a fresh identity.
	if err := own.Release(); err != nil {
		t.Fatal(err)
	}
	own2, _, err := AcquireOwnership(ds, ds.Data, "test-version")
	if err != nil {
		t.Fatal(err)
	}
	if own2.Record.InstanceUUID == own.Record.InstanceUUID {
		t.Fatal("instance UUID must be fresh per start")
	}
	own2.Release()
}

func TestStaleOwnerTakeover(t *testing.T) {
	tmp := t.TempDir()
	ds := DirSet{Data: filepath.Join(tmp, "data"), Runtime: filepath.Join(tmp, "runtime")}
	if err := os.MkdirAll(ds.Data, 0o700); err != nil {
		t.Fatal(err)
	}
	// Simulate a dead prior owner: a record without a held kernel lock.
	if err := os.MkdirAll(ds.Runtime, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := OwnerRecord{InstanceUUID: "dead-owner", PID: 999999, CoreVersion: "old"}
	b, _ := json.Marshal(stale)
	if err := writeFileAtomic(filepath.Join(ds.Runtime, OwnerRecordName), b, 0o600); err != nil {
		t.Fatal(err)
	}
	own, tookOver, err := AcquireOwnership(ds, ds.Data, "test-version")
	if err != nil {
		t.Fatal(err)
	}
	defer own.Release()
	if !tookOver {
		t.Fatal("a record with a free kernel lock is a stale takeover")
	}
	if own.Record.InstanceUUID == "dead-owner" {
		t.Fatal("takeover must rewrite the owner record")
	}
}

func TestReadOwnerRecordMissing(t *testing.T) {
	if _, err := ReadOwnerRecord(t.TempDir()); !errors.Is(err, ErrOwnerRecordMissing) {
		t.Fatalf("err = %v, want ErrOwnerRecordMissing", err)
	}
}

func TestDataDirIdentityAliasStable(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "data")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "alias")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	a, err := dataDirIdentity(real)
	if err != nil {
		t.Fatal(err)
	}
	b, err := dataDirIdentity(link)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("alias spellings must resolve to one identity: %+v vs %+v", a, b)
	}
	if a.Digest() == "" {
		t.Fatal("identity digest must be non-empty")
	}
}
