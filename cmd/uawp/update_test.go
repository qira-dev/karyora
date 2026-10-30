package main

// The composition-root wiring tests (sub-milestone 6): the production ports
// over real HTTP (httptest), the fail-closed blocked artifact-signature
// layer, the real C5-09 backup adapter + rename activator + audit chain
// composing the machine end to end, and the `uawp update` command surface.
// The machine's own behaviors are the C5-19 suite's (internal/updater); what
// these tests prove is THE WIRING.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/config"
	"github.com/qira-dev/karyora/internal/platform"
	"github.com/qira-dev/karyora/internal/updater"
)

func TestTargetRowForPlatform(t *testing.T) {
	for _, tc := range []struct {
		goos, goarch, want string
	}{
		{"windows", "amd64", "windows-amd64"},
		{"windows", "arm64", "windows-arm64"},
		{"darwin", "amd64", "macos-amd64"},
		{"darwin", "arm64", "macos-arm64"},
		{"linux", "amd64", "linux-amd64"},
		{"linux", "arm64", "linux-arm64"},
	} {
		got, err := targetRowForPlatform(tc.goos, tc.goarch)
		if err != nil || got.id != tc.want {
			t.Fatalf("targetRowForPlatform(%s/%s) = %+v, %v; want %s", tc.goos, tc.goarch, got, err, tc.want)
		}
		if got.artifactName() != "uawp-"+tc.goos+"-"+tc.goarch {
			t.Fatalf("artifactName = %q; want the release-channel convention", got.artifactName())
		}
	}
	// C5-11: every unlisted combination is unsupported by rule.
	for _, tc := range [][2]string{{"freebsd", "amd64"}, {"linux", "386"}, {"windows", "386"}} {
		if _, err := targetRowForPlatform(tc[0], tc[1]); err == nil {
			t.Fatalf("targetRowForPlatform(%s/%s) should refuse an unlisted row", tc[0], tc[1])
		}
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadPinnedRoot(t *testing.T) {
	_, rootPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub := rootPriv.Public().(ed25519.PublicKey)
	fp := updater.Fingerprint(pub)
	dir := t.TempDir()

	// Absent anchor: the honest pre-ceremony state (zero key — fail-closed).
	pinned, err := loadPinnedRoot(dir)
	if err != nil || pinned.Key != nil {
		t.Fatalf("absent anchor = %+v, %v; want zero key, no error", pinned, err)
	}
	// A consistent anchor loads.
	writeTestFile(t, filepath.Join(dir, pinnedRootAnchorFile),
		fmt.Sprintf(`{"key": %q, "fingerprint": %q}`, hex.EncodeToString(pub), fp))
	pinned, err = loadPinnedRoot(dir)
	if err != nil || string(pinned.Key) != string(pub) {
		t.Fatalf("pinned = %v, %v; want the anchor key", pinned.Key, err)
	}
	// A fingerprint/key mismatch is a hard failure (a tampered anchor is
	// never silently accepted).
	writeTestFile(t, filepath.Join(dir, pinnedRootAnchorFile),
		fmt.Sprintf(`{"key": %q, "fingerprint": "deadbeef"}`, hex.EncodeToString(pub)))
	if _, err := loadPinnedRoot(dir); err == nil {
		t.Fatal("a fingerprint/key mismatch must refuse")
	}
	// A malformed key is a hard failure.
	writeTestFile(t, filepath.Join(dir, pinnedRootAnchorFile), `{"key": "zz"}`)
	if _, err := loadPinnedRoot(dir); err == nil {
		t.Fatal("a malformed anchor key must refuse")
	}
}

// testChannel is an httptest server speaking the release-channel conventions:
// /update.json, /artifacts/<row>/<name>, /bindings/<sha256>.json.
type testChannel struct {
	srv      *httptest.Server
	metadata []byte
	artifact []byte
	bindings map[string][]byte
}

func newTestChannel(t *testing.T, metadata, artifact []byte) *testChannel {
	t.Helper()
	c := &testChannel{metadata: metadata, artifact: artifact, bindings: map[string][]byte{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/update.json", func(w http.ResponseWriter, r *http.Request) {
		if c.metadata == nil {
			http.Error(w, "absent", http.StatusNotFound)
			return
		}
		w.Write(c.metadata)
	})
	mux.HandleFunc("/artifacts/", func(w http.ResponseWriter, r *http.Request) {
		if c.artifact == nil {
			http.Error(w, "absent", http.StatusNotFound)
			return
		}
		w.Write(c.artifact)
	})
	mux.HandleFunc("/bindings/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/bindings/")
		rec, ok := c.bindings[strings.TrimSuffix(name, ".json")]
		if !ok {
			http.Error(w, "absent", http.StatusNotFound)
			return
		}
		w.Write(rec)
	})
	c.srv = httptest.NewServer(mux)
	t.Cleanup(c.srv.Close)
	return c
}

func TestMetadataFetcherFetchesAndBounds(t *testing.T) {
	bundle := []byte(`{"manifest":{"payload":"x","signature":{"algorithm":"ed25519","key_id":"k","value":"AQID"}}}`)
	c := newTestChannel(t, bundle, nil)
	f, err := newHTTPMetadataFetcher(c.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.Fetch(context.Background())
	if err != nil || got.Manifest.Payload != "x" {
		t.Fatalf("fetch = %+v, %v; want the decoded bundle", got, err)
	}
	// A channel error is the fetcher's reported failure (the machine types it).
	c.metadata = nil
	if _, err := f.Fetch(context.Background()); err == nil {
		t.Fatal("an absent bundle must fail the fetch")
	}
	// Malformed JSON is refused (the machine re-verifies everything).
	c.metadata = []byte("not-json")
	if _, err := f.Fetch(context.Background()); err == nil {
		t.Fatal("malformed metadata must fail the fetch")
	}
	// An unparseable endpoint never constructs a fetcher.
	if _, err := newHTTPMetadataFetcher("not-a-url"); err == nil {
		t.Fatal("a non-URL endpoint must refuse construction")
	}
}

func TestArtifactDownloaderExactSizeAndExecutable(t *testing.T) {
	c := newTestChannel(t, nil, []byte("EXACT-BYTES"))
	f, _ := newHTTPMetadataFetcher(c.srv.URL)
	d := &httpArtifactDownloader{fetcher: f, target: updateTarget{id: "linux-amd64", goos: "linux", goarch: "amd64"}}
	dest := filepath.Join(t.TempDir(), "staged")
	sum := sha256.Sum256([]byte("EXACT-BYTES"))
	entry := updater.ArtifactEntry{TargetID: "linux-amd64", Kind: "uawp-binary", SHA256: hex.EncodeToString(sum[:]), Size: int64(len("EXACT-BYTES"))}
	if err := d.Download(context.Background(), entry, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "EXACT-BYTES" {
		t.Fatalf("staged = %q, %v; want the exact bytes", got, err)
	}
	info, err := os.Stat(dest)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v, %v; want 0755 (the staged artifact becomes the binary)", info.Mode().Perm(), err)
	}
	// A short stream is refused (the machine deletes staging and types it).
	c.artifact = []byte("SHORT")
	if err := d.Download(context.Background(), entry, dest); err == nil {
		t.Fatal("a short stream must fail")
	}
	// An oversized stream is refused (never truncate silently).
	c.artifact = []byte("EXACT-BYTES-AND-MORE")
	if err := d.Download(context.Background(), entry, dest); err == nil {
		t.Fatal("an oversized stream must fail")
	}
}

func TestChannelBindingResolverReturnsServedDigest(t *testing.T) {
	c := newTestChannel(t, []byte("{}"), nil)
	f, _ := newHTTPMetadataFetcher(c.srv.URL)
	r := &channelBindingResolver{fetcher: f}
	sum := sha256.Sum256([]byte(`{"binding":"record"}`))
	digest := hex.EncodeToString(sum[:])
	c.bindings[digest] = []byte(`{"binding":"record"}`)
	got, err := r.Resolve(context.Background(), updater.SupplyChainPtr{ArtifactSHA256: digest, BindingRecordSHA256: digest})
	if err != nil || got != digest {
		t.Fatalf("resolve = %q, %v; want the served bytes' sha256", got, err)
	}
	// An absent record fails the resolution (C5-19 A5's unresolvable row).
	if _, err := r.Resolve(context.Background(), updater.SupplyChainPtr{ArtifactSHA256: "ff"}); err == nil {
		t.Fatal("an unresolvable pointer must fail")
	}
}

func TestBlockedSignatureVerifierAlwaysRefuses(t *testing.T) {
	err := blockedSignatureVerifier{}.Verify(context.Background(), "/staged", nil, "linux-amd64")
	if err == nil {
		t.Fatal("the blocked layer must refuse every artifact")
	}
	ue, ok := updater.AsUpdaterError(err)
	if !ok || ue.Reason != updater.ReasonArtifactSignature {
		t.Fatalf("err = %v; want the typed artifact_signature refusal", err)
	}
	if !strings.Contains(ue.Detail, "BLOCKED") {
		t.Fatalf("detail = %q; want the recorded blocked state", ue.Detail)
	}
}

// updateWiringFixture assembles the real production wiring against a temp
// install: an httptest channel, a real data directory, the pinned anchor,
// and the engine from newUpdateEngine (the REAL composition function).
type updateWiringFixture struct {
	h       *harness
	ds      platform.DirSet
	layout  updater.Layout
	channel *testChannel
	wired   *wiredUpdateEngine
}

// signBundleForTest builds a valid MetadataBundle (root + manifest) with
// GENERATED TEST KEYS ONLY — the same discipline the C5-19 suite records.
func signBundleForTest(t *testing.T, artifact []byte, releaseVersion string, metadataVersion uint64) (bundleBytes []byte, rootPub ed25519.PublicKey) {
	t.Helper()
	rootPub, rootPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, signerPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	artDigest := sha256.Sum256(artifact)
	rootPayload, err := updater.CanonicalPayload(updater.RootPayload{
		SchemaVersion: updater.RootSchema,
		RootVersion:   1,
		ExpiresAt:     time.Now().Add(24 * time.Hour),
		ReleaseSigner: updater.SignerDelegation{PublicKey: signerPriv.Public().(ed25519.PublicKey), Threshold: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	rootSig := ed25519.Sign(rootPriv, rootPayload)
	manifestPayload, err := updater.CanonicalPayload(updater.ReleaseManifest{
		SchemaVersion:   updater.ManifestSchema,
		Channel:         "stable",
		ReleaseVersion:  releaseVersion,
		MetadataVersion: metadataVersion,
		RootVersion:     1,
		PublishedAt:     time.Now(),
		ExpiresAt:       time.Now().Add(24 * time.Hour),
		Artifacts: []updater.ArtifactEntry{{
			TargetID: "linux-amd64",
			Kind:     "uawp-binary",
			SHA256:   hex.EncodeToString(artDigest[:]),
			Size:     int64(len(artifact)),
			// A well-formed TEST signature: the C5-13 layer's verdict is the
			// stubbed verifier's in the full-path fixture (the blocked-layer
			// fixture refuses on the typed reason before any of this matters).
			Signature: &updater.Signature{Algorithm: "ed25519", KeyID: "test", Value: make([]byte, ed25519.SignatureSize)},
		}},
		Migrations: updater.MigrationManifest{FromSchema: 1, ToSchema: 1, RollbackCompatible: true,
			Steps: []updater.MigrationStep{{ID: "m1", ReversibilityClass: updater.ReversibilityForward}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifestSig := ed25519.Sign(signerPriv, manifestPayload)
	type wireDoc struct {
		Payload   string `json:"payload"`
		Signature struct {
			Algorithm string `json:"algorithm"`
			KeyID     string `json:"key_id"`
			Value     []byte `json:"value"`
		} `json:"signature"`
	}
	mk := func(payload []byte, sig []byte, key ed25519.PublicKey) wireDoc {
		var d wireDoc
		d.Payload = string(payload)
		d.Signature.Algorithm = "ed25519"
		d.Signature.KeyID = updater.Fingerprint(key)
		d.Signature.Value = sig
		return d
	}
	rootDoc := mk(rootPayload, rootSig, rootPub)
	out, err := json.Marshal(struct {
		Root     *wireDoc `json:"root,omitempty"`
		Manifest wireDoc  `json:"manifest"`
	}{Root: &rootDoc,
		Manifest: mk(manifestPayload, manifestSig, signerPriv.Public().(ed25519.PublicKey))})
	if err != nil {
		t.Fatal(err)
	}
	return out, rootPub
}

func newUpdateWiringFixture(t *testing.T) *updateWiringFixture {
	t.Helper()
	f := &updateWiringFixture{h: newHarness(t)}
	ds, err := platform.Resolve(&platform.Overrides{DataDir: filepath.Join(f.h.t.TempDir(), "data")})
	if err != nil {
		t.Fatal(err)
	}
	f.ds = ds
	if err := os.MkdirAll(ds.Data, 0o700); err != nil {
		t.Fatal(err)
	}
	installDir := f.h.t.TempDir()
	f.layout = updater.Layout{
		InstallPath: filepath.Join(installDir, "current-uawp"),
		RetainPath:  filepath.Join(installDir, "retained-uawp"),
		StagingDir:  filepath.Join(ds.Cache, "update-staging"),
		StateDir:    filepath.Join(ds.State, "update"),
	}
	for _, d := range []string{f.layout.StagingDir, f.layout.StateDir, installDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(f.layout.InstallPath, []byte("INSTALLED-OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.channel = newTestChannel(t, nil, []byte("NEW-RELEASE-BYTES"))
	bundle, rootPub := signBundleForTest(t, f.channel.artifact, "1.1.0", 2)
	f.channel.metadata = bundle
	// The out-of-band anchor file (what the release documentation would
	// install): the pinned root key.
	writeTestFile(t, filepath.Join(f.layout.StateDir, pinnedRootAnchorFile),
		fmt.Sprintf(`{"key": %q}`, hex.EncodeToString(rootPub)))

	// The full validated configuration (defaults layer) — the storage
	// option matrix refuses zero-valued bounds.
	cfg, err := config.Load("test-corr", nil, func(string) string { return "" }, nil)
	if err != nil {
		t.Fatal(err)
	}
	wired := newUpdateEngine(updateEngineParams{
		cfg:           cfg,
		ds:            ds,
		layout:        f.layout,
		pinned:        updater.PinnedRoot{Key: rootPub},
		target:        updateTarget{id: "linux-amd64", goos: "linux", goarch: "amd64"},
		correlationID: "test-corr",
		installed:     "1.0.0",
	})
	// The fixture overrides the config-derived endpoint (the config surface
	// is exercised in its own tests) with the test channel's URL.
	fetcher, err := newHTTPMetadataFetcher(f.channel.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	wired.engine.Fetcher = fetcher
	wired.engine.Downloader = &httpArtifactDownloader{fetcher: fetcher, target: updateTarget{id: "linux-amd64", goos: "linux", goarch: "amd64"}}
	wired.engine.SupplyChain = &channelBindingResolver{fetcher: fetcher}
	f.wired = wired
	return f
}

// TestUpdateApplyRefusesAtBlockedSignature proves the honest end state of
// the full real wiring: a perfectly valid metadata+artifact run refuses
// fail-closed at the C5-13 layer, staging and journal cleared, the T2
// monotonic state persisted, the installed binary untouched.
func TestUpdateApplyRefusesAtBlockedSignature(t *testing.T) {
	f := newUpdateWiringFixture(t)
	wired := f.wired
	if err := wired.store.connect(context.Background(), "1.0.0"); err != nil {
		t.Fatal(err)
	}
	defer wired.store.close()
	defer wired.audit.Flush()

	out, err := wired.engine.RunUpdate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != updater.StateRefused || out.Reason != updater.ReasonArtifactSignature {
		t.Fatalf("outcome = %+v; want the typed artifact_signature refusal at S4", out)
	}
	entries, _ := os.ReadDir(f.layout.StagingDir)
	if len(entries) != 0 {
		t.Fatalf("staging = %v; want cleared", entries)
	}
	st, _, err := updater.ReadStatus(f.layout)
	if err != nil || st.HighestSeenMetadataVersion != 2 {
		t.Fatalf("state = %+v; want the T2-persisted metadata version", st)
	}
	got, _ := os.ReadFile(f.layout.InstallPath)
	if string(got) != "INSTALLED-OLD" {
		t.Fatalf("the installed binary changed: %q", got)
	}
}

// passingArtifactSig is the substitution point for the BLOCKED C5-13 layer:
// the machine's S5–S10 composition (real backup, real activator, real audit)
// is proven with this stub exactly the way the C5-19 suite stubs it —
// generated TEST KEYS only, never a passing production claim.
type passingArtifactSig struct{}

func (passingArtifactSig) Verify(_ context.Context, _ string, _ *updater.Signature, _ string) error {
	return nil
}

// passingHealth stubs S8 (the real probeHealthChecker spawns a real binary —
// the probe command surface has its own test below).
type passingHealth struct{}

func (passingHealth) Check(_ context.Context, _ updater.HealthCheckSpec) (bool, string) {
	return true, "pass"
}

// TestUpdateApplyFullPathComposes proves the wiring carries a healthy
// release through S5–S10: the real C5-09 backup commits, the real rename
// activator swaps and retains, the audit chain receives the machine's
// events, and the state settles at S10 with the advanced last-known-good.
func TestUpdateApplyFullPathComposes(t *testing.T) {
	f := newUpdateWiringFixture(t)
	wired := f.wired
	wired.engine.ArtifactSig = passingArtifactSig{}
	wired.engine.Health = passingHealth{}
	if err := wired.store.connect(context.Background(), "1.0.0"); err != nil {
		t.Fatal(err)
	}
	defer wired.store.close()
	defer wired.audit.Flush()

	out, err := wired.engine.RunUpdate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != updater.StateUpdatedStable || out.Version != "1.1.0" {
		t.Fatalf("outcome = %+v; want S10 at 1.1.0", out)
	}
	got, _ := os.ReadFile(f.layout.InstallPath)
	if string(got) != "NEW-RELEASE-BYTES" {
		t.Fatalf("install = %q; want the new release", got)
	}
	got, _ = os.ReadFile(f.layout.RetainPath)
	if string(got) != "INSTALLED-OLD" {
		t.Fatalf("retain = %q; want the retained prior binary", got)
	}
	st, _, err := updater.ReadStatus(f.layout)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastKnownGood.Version != "1.1.0" {
		t.Fatalf("last-known-good = %+v; want 1.1.0", st.LastKnownGood)
	}
	// The real C5-09 backup ran: the delivered backups root carries a
	// generation directory with its manifest.
	roots, err := os.ReadDir(filepath.Join(f.ds.Data, "backups"))
	if err != nil || len(roots) == 0 {
		t.Fatalf("backups root = %v, %v; want the committed generation", roots, err)
	}
	// The audit chain received the machine's events through the real service.
	s, err := wired.store.get()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.AuditRecords()
	if err != nil || len(rows) == 0 {
		t.Fatalf("audit rows = %v, %v; want the update events", rows, err)
	}
}

// TestUpdateCheckCommandSurface exercises the command surface itself: the
// empty-endpoint refusal is the honest typed outcome, status reports, and
// the subcommand guard works.
func TestUpdateCheckCommandSurface(t *testing.T) {
	h := newHarness(t)
	code, outp := h.run("update", "check", "--json")
	if code != 1 {
		t.Fatalf("exit = %d, out = %s; want the typed refusal exit", code, outp)
	}
	var o updater.Outcome
	if err := json.Unmarshal([]byte(outp), &o); err != nil || o.Reason != updater.ReasonFetchFailed {
		t.Fatalf("out = %s (%v); want the fetch_failed outcome", outp, err)
	}
	code, outp = h.run("update", "status", "--json")
	if code != 0 {
		t.Fatalf("status exit = %d, out = %s", code, outp)
	}
	if !strings.Contains(outp, "last_known_good") {
		t.Fatalf("status out = %s; want the state report", outp)
	}
	// An unknown subcommand is refused with the usage line.
	if code, outp := h.run("update", "bogus"); code != 1 || !strings.Contains(outp, "usage: uawp") {
		t.Fatalf("bogus exit = %d, out = %s; want the usage refusal", code, outp)
	}
}

// TestUpdateHealthProbeCommand exercises the internal S8 mechanism against a
// real temp data directory: the probe creates/migrates the store and its
// schema comparison decides the verdict.
func TestUpdateHealthProbeCommand(t *testing.T) {
	h := newHarness(t)
	dataDir := filepath.Join(h.t.TempDir(), "data")
	// A deliberately wrong target fails and reports the landed schema.
	code, outp := h.run("update", "health-probe", "--data-dir", dataDir, "--schema-to", "999", "--json")
	if code != 1 {
		t.Fatalf("exit = %d, out = %s; want the mismatch failure", code, outp)
	}
	var rep healthProbeReport
	if err := json.Unmarshal([]byte(outp), &rep); err != nil {
		t.Fatalf("out = %s (%v); want the probe report", outp, err)
	}
	if rep.Passed || rep.SchemaVersion == 999 {
		t.Fatalf("report = %+v; want a failed mismatch naming the landed schema", rep)
	}
	// The correct target passes (the same binary now owns the directory).
	code, outp = h.run("update", "health-probe", "--data-dir", dataDir, "--schema-to", fmt.Sprint(rep.SchemaVersion), "--json")
	if code != 0 {
		t.Fatalf("exit = %d, out = %s; want the pass", code, outp)
	}
	if err := json.Unmarshal([]byte(outp), &rep); err != nil || !rep.Passed {
		t.Fatalf("out = %s (%v); want the passing report", outp, err)
	}
	// A missing --schema-to is refused.
	if code, outp := h.run("update", "health-probe", "--data-dir", dataDir, "--json"); code != 1 || !strings.Contains(outp, "--schema-to") {
		t.Fatalf("exit = %d, out = %s; want the missing-flag refusal", code, outp)
	}
}
