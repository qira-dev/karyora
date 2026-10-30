package updater

// The C5-19 adversarial-matrix tests (Core CR-5.2 C5-19 §8.1: "the fixtures
// are DESIGNED Go test identifiers … the names here are the design contract
// the implementation binds to"). This file carries the harness and the
// metadata-layer (M) and artifact-layer (A) rows; lifecycle_test.go carries
// the L rows. Every key here is a GENERATED TEST KEY (ed25519 keys created
// inside the tests — never a ceremony, never real key material, C5-19 §14's
// execution-status discipline); the A4 per-target rows and the S1–S6
// signing-recovery rows stay Not Run — BLOCKED (no platform identity, no
// custody, no signing operation exists; C5-13 §9.2).

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"
)

// ---- the harness ----

const (
	testTarget       = "linux-amd64"
	installedVersion = "1.0.0"
	newVersion       = "1.1.0"
	lowerVersion     = "0.9.0"
	artifactMagic    = "UAWP-BINARY-"
	kindBinary       = "uawp-binary"
)

type hClock struct{ now time.Time }

func (c *hClock) Now() time.Time { return c.now }

type hFetcher struct {
	bundle MetadataBundle
	err    error
	calls  int
}

func (f *hFetcher) Fetch(_ context.Context) (MetadataBundle, error) {
	f.calls++
	if f.err != nil {
		return MetadataBundle{}, f.err
	}
	return f.bundle, nil
}

type hDownloader struct {
	h *hHarness
	// content overrides the served artifact bytes (the corruption/partial
	// fixtures); nil means "download exactly what the metadata describes".
	content []byte
	err     error
	calls   int
}

func (d *hDownloader) Download(_ context.Context, _ ArtifactEntry, destPath string) error {
	d.calls++
	if d.err != nil {
		return d.err
	}
	data := d.content
	if data == nil {
		data = d.h.servedArtifact
	}
	return writeFile(destPath, data)
}

type hBackupper struct {
	ref   string
	err   error
	calls int
}

func (b *hBackupper) CommitBackup(_ context.Context) (string, error) {
	b.calls++
	if b.err != nil {
		return "", b.err
	}
	return b.ref, nil
}

type hSupply struct {
	resolved map[string]string // artifact digest -> binding record digest
	err      error
	calls    int
}

func (s *hSupply) Resolve(_ context.Context, ptr SupplyChainPtr) (string, error) {
	s.calls++
	if s.err != nil {
		return "", s.err
	}
	got, ok := s.resolved[ptr.ArtifactSHA256]
	if !ok {
		return "", errors.New("binding record not found")
	}
	return got, nil
}

// hArtSig is the C5-13 artifact-signature layer's test stand-in: a REAL
// Ed25519 verification of the manifest-carried signature over the artifact's
// exact bytes, against the PLATFORM identity key (the test's artifact key) —
// exactly the property the adversarial rows need (an attacker-chosen
// signature does not verify under the platform key).
type hArtSig struct {
	platformKey ed25519.PublicKey
	calls       int
}

func (a *hArtSig) Verify(_ context.Context, artifactPath string, sig *Signature, _ string) error {
	a.calls++
	if sig == nil {
		return fail(ReasonArtifactSignature, "signature absent")
	}
	raw, err := readFile(artifactPath)
	if err != nil {
		return err
	}
	var pub ed25519.PublicKey = a.platformKey
	if len(pub) != ed25519.PublicKeySize {
		return fail(ReasonArtifactSignature, "platform identity key unavailable")
	}
	if !ed25519.Verify(pub, raw, sig.Value) {
		return fail(ReasonArtifactSignature, "the artifact signature does not verify under the platform identity")
	}
	return nil
}

type hKind struct {
	calls int
}

func (k *hKind) Validate(_ context.Context, artifactPath, kind string) error {
	k.calls++
	if kind != kindBinary {
		return fail(ReasonArtifactMalformed, "unknown kind %q", kind)
	}
	raw, err := readFile(artifactPath)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(string(raw), artifactMagic) {
		return fail(ReasonArtifactMalformed, "the artifact lacks the %q magic for kind %q", artifactMagic, kind)
	}
	return nil
}

type hHealth struct {
	pass   bool
	reason string
	calls  int
	last   HealthCheckSpec
}

func (h *hHealth) Check(_ context.Context, spec HealthCheckSpec) (bool, string) {
	h.calls++
	h.last = spec
	return h.pass, h.reason
}

type hLocker struct{ acquired int }

func (l *hLocker) Acquire(_ context.Context) error { l.acquired++; return nil }

type hAuditor struct{ events []Event }

func (a *hAuditor) Record(_ context.Context, e Event) { a.events = append(a.events, e) }

func (a *hAuditor) count(name string) int {
	n := 0
	for _, e := range a.events {
		if e.Name == name {
			n++
		}
	}
	return n
}

type hHarness struct {
	t          *testing.T
	dir        string
	layout     Layout
	rootKey    ed25519.PrivateKey
	signerKey  ed25519.PrivateKey
	forgedKey  ed25519.PrivateKey
	artifactK1 ed25519.PrivateKey // the platform artifact identity (TEST KEY)
	artifactK2 ed25519.PrivateKey // an attacker-held artifact key (TEST KEY)
	clock      *hClock
	fetcher    *hFetcher
	downloader *hDownloader
	backupper  *hBackupper
	supply     *hSupply
	artSig     *hArtSig
	kind       *hKind
	health     *hHealth
	locker     *hLocker
	auditor    *hAuditor
	engine     *Engine

	servedArtifact []byte // the artifact bytes the last serve() declared
}

func newHarness(t *testing.T) *hHarness {
	t.Helper()
	dir := t.TempDir()
	h := &hHarness{
		t:   t,
		dir: dir,
		layout: Layout{
			InstallPath: dir + "/install/current-uawp",
			RetainPath:  dir + "/install/retained-uawp",
			StagingDir:  dir + "/staging",
			StateDir:    dir + "/state",
		},
	}
	var err error
	if _, h.rootKey, err = ed25519.GenerateKey(rand.Reader); err != nil {
		t.Fatal(err)
	}
	if _, h.signerKey, err = ed25519.GenerateKey(rand.Reader); err != nil {
		t.Fatal(err)
	}
	if _, h.forgedKey, err = ed25519.GenerateKey(rand.Reader); err != nil {
		t.Fatal(err)
	}
	if _, h.artifactK1, err = ed25519.GenerateKey(rand.Reader); err != nil {
		t.Fatal(err)
	}
	if _, h.artifactK2, err = ed25519.GenerateKey(rand.Reader); err != nil {
		t.Fatal(err)
	}
	h.clock = &hClock{now: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
	h.fetcher = &hFetcher{}
	h.downloader = &hDownloader{h: h}
	h.backupper = &hBackupper{ref: "backup-ref-1"}
	h.supply = &hSupply{}
	h.artSig = &hArtSig{platformKey: h.artifactK1.Public().(ed25519.PublicKey)}
	h.kind = &hKind{}
	h.health = &hHealth{pass: true}
	h.locker = &hLocker{}
	h.auditor = &hAuditor{}
	for _, d := range []string{h.layout.StagingDir, h.layout.StateDir, filepath_dir(h.layout.InstallPath)} {
		if err := mkdirAll(d); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeFile(h.layout.InstallPath, []byte(artifactMagic+"v"+installedVersion)); err != nil {
		t.Fatal(err)
	}
	h.engine = &Engine{
		InstalledVersion: installedVersion,
		TargetID:         testTarget,
		PinnedRoot:       PinnedRoot{Key: h.rootKey.Public().(ed25519.PublicKey)},
		Layout:           h.layout,
		Clock:            h.clock.Now,
		Locker:           h.locker,
		Fetcher:          h.fetcher,
		Downloader:       h.downloader,
		Backupper:        h.backupper,
		SupplyChain:      h.supply,
		ArtifactSig:      h.artSig,
		KindValid:        h.kind,
		Health:           h.health,
		Activator:        RenameActivator{},
		Auditor:          h.auditor,
	}
	return h
}

// serve builds and loads a bundle: an optional root document (signed by the
// pinned root key) plus the manifest (signed by the release signer), with
// per-call overrides applied by the caller to the returned payloads.
type bundleOpts struct {
	rootVersion    uint64
	rootExpires    time.Time
	rootSigner     ed25519.PrivateKey // the key the root DELEGATES
	rootSignedBy   ed25519.PrivateKey // the key that signs the root doc (pinned by default)
	manifestKey    ed25519.PrivateKey // the key that signs the manifest (the delegated signer by default)
	releaseVersion string
	metadataVer    uint64
	manifestRootV  uint64
	expires        time.Time
	artifactBytes  func() []byte       // defaults to the magic-prefixed new-version content
	artifactSigKey *ed25519.PrivateKey // defaults to the platform artifact key
	dropSignature  bool
	minSupported   string
	rollbackCompat *bool
	stepClass      string
	supplyChain    []SupplyChainPtr
}

func (h *hHarness) serve(o bundleOpts) {
	h.t.Helper()
	if o.rootVersion == 0 {
		o.rootVersion = 1
	}
	if o.manifestRootV == 0 {
		o.manifestRootV = o.rootVersion
	}
	if o.releaseVersion == "" {
		o.releaseVersion = newVersion
	}
	if o.metadataVer == 0 {
		o.metadataVer = 2
	}
	if o.expires.IsZero() {
		o.expires = h.clock.now.Add(time.Hour)
	}
	if o.rootExpires.IsZero() {
		o.rootExpires = h.clock.now.Add(24 * time.Hour)
	}
	if o.rootSigner == nil {
		o.rootSigner = h.signerKey
	}
	rootSignerPub := o.rootSigner.Public().(ed25519.PublicKey)
	rootSignedBy := o.rootSignedBy
	if rootSignedBy == nil {
		rootSignedBy = h.rootKey
	}
	manifestKey := o.manifestKey
	if manifestKey == nil {
		manifestKey = h.signerKey
	}
	artifactBytes := []byte(artifactMagic + "v" + o.releaseVersion)
	if o.artifactBytes != nil {
		artifactBytes = o.artifactBytes()
	}
	h.servedArtifact = artifactBytes
	digest := sha256Hex(artifactBytes)
	var sig *Signature
	if !o.dropSignature {
		sigKey := h.artifactK1
		if o.artifactSigKey != nil {
			sigKey = *o.artifactSigKey
		}
		sig = signRaw(artifactBytes, sigKey)
	}
	rollbackCompat := true
	if o.rollbackCompat != nil {
		rollbackCompat = *o.rollbackCompat
	}
	stepClass := o.stepClass
	if stepClass == "" {
		stepClass = ReversibilityForward
	}
	rootPayload, err := CanonicalPayload(RootPayload{
		SchemaVersion: RootSchema,
		RootVersion:   o.rootVersion,
		ExpiresAt:     o.rootExpires,
		ReleaseSigner: SignerDelegation{PublicKey: rootSignerPub, Threshold: 1},
	})
	if err != nil {
		h.t.Fatal(err)
	}
	manifestPayload, err := CanonicalPayload(ReleaseManifest{
		SchemaVersion:      ManifestSchema,
		Channel:            "stable",
		ReleaseVersion:     o.releaseVersion,
		MetadataVersion:    o.metadataVer,
		RootVersion:        o.manifestRootV,
		PublishedAt:        h.clock.now,
		ExpiresAt:          o.expires,
		Artifacts:          []ArtifactEntry{{TargetID: testTarget, Kind: kindBinary, SHA256: digest, Size: int64(len(artifactBytes)), Signature: sig}},
		SupplyChain:        o.supplyChain,
		MinSupportedClient: o.minSupported,
		Migrations:         MigrationManifest{FromSchema: 1, ToSchema: 1, RollbackCompatible: rollbackCompat, Steps: []MigrationStep{{ID: "m1", ReversibilityClass: stepClass}}},
	})
	if err != nil {
		h.t.Fatal(err)
	}
	bundle := MetadataBundle{
		Root:     signedDocPtr(rootPayload, rootSignedBy),
		Manifest: *signedDoc(manifestPayload, manifestKey),
	}
	h.fetcher.bundle = bundle
}

func signedDoc(payload []byte, key ed25519.PrivateKey) *SignedDocument {
	pub := key.Public().(ed25519.PublicKey)
	sig := ed25519.Sign(key, payload)
	return &SignedDocument{
		Payload: string(payload),
		Signature: Signature{
			Algorithm: SigAlgorithm,
			KeyID:     Fingerprint(pub),
			Value:     sig,
		},
	}
}

func signedDocPtr(payload []byte, key ed25519.PrivateKey) *SignedDocument {
	return signedDoc(payload, key)
}

func signRaw(data []byte, key ed25519.PrivateKey) *Signature {
	pub := key.Public().(ed25519.PublicKey)
	return &Signature{Algorithm: SigAlgorithm, KeyID: Fingerprint(pub), Value: ed25519.Sign(key, data)}
}
