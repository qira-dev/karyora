package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestConnectionRecordShape pins the coded wire tags of the card-10
// working candidate's record schema (the P3.2 §3.3 basis): the JSON field
// names the connection wire carries, the fingerprint's wire absence (a
// secret-derived digest is internal, never marshaled), and the tested
// state's truthful accessors.
func TestConnectionRecordShape(t *testing.T) {
	rec := ConnectionRecord{
		ConnectionID:    "conn-1",
		IdempotencyKey:  "idem-1",
		Provider:        "mock",
		BaseURL:         "https://mock-fixture.invalid",
		ModelID:         "m",
		SecretRef:       "connection:conn-1/api-key",
		CredentialEpoch: 2,
		Fingerprint:     "sha256:aa",
		TestedAt:        "2026-11-02T00:00:00Z",
		TestedDigest:    "sha256:bb",
		TestedEpoch:     1,
		CreatedAt:       "2026-11-02T00:00:00Z",
		UpdatedAt:       "2026-11-02T00:00:00Z",
	}
	must := map[string]bool{
		`"connection_id":"conn-1"`:                 true,
		`"provider":"mock"`:                        true,
		`"model_id":"m"`:                           true,
		`"secret_ref":"connection:conn-1/api-key"`: true,
		`"credential_epoch":2`:                     true,
		`"tested_at":"2026-11-02T00:00:00Z"`:       true,
		`"tested_epoch":1`:                         true,
	}
	b := mustJSONString(rec)
	for frag := range must {
		if !strings.Contains(b, frag) {
			t.Errorf("wire form lacks %s: %s", frag, b)
		}
	}
	if strings.Contains(b, "sha256:aa") || strings.Contains(b, "fingerprint") {
		t.Errorf("the idempotency fingerprint must never marshal to the wire: %s", b)
	}
	if !rec.Tested() {
		t.Error("a record with a tested instant reports Tested")
	}
	// The tested-but-changed case: the credential epoch moved past the
	// tested one (a rotation) — the tested result is stale-truthfully not
	// current even though the digest still matches.
	if rec.TestCurrent() {
		t.Error("a rotation (epoch bump) must render the tested state not-current")
	}
	changed := rec
	changed.BaseURL = "https://other.invalid"
	changed.TestedEpoch = 2
	if changed.TestCurrent() {
		t.Error("a changed base URL must render the tested state not-current")
	}
	current := rec
	current.TestedEpoch = 2
	current.TestedDigest = current.InputsDigest()
	if !current.TestCurrent() {
		t.Error("an unchanged record with a matching digest and epoch reports test-current")
	}
}

// TestConnectionInputsDigest pins the tested-at inputs digest: it covers
// the non-secret inputs (provider, base URL, model id, headers), it never
// covers the credential VALUE (D-37: no value-derived material on disk),
// and it is stable across identical inputs.
func TestConnectionInputsDigest(t *testing.T) {
	a := ConnectionRecord{Provider: "mock", BaseURL: "https://x.invalid", ModelID: "m"}
	b := ConnectionRecord{Provider: "mock", BaseURL: "https://x.invalid", ModelID: "m"}
	if a.InputsDigest() != b.InputsDigest() {
		t.Fatal("identical inputs must digest identically")
	}
	b.ModelID = "other"
	if a.InputsDigest() == b.InputsDigest() {
		t.Fatal("a changed model id must change the digest")
	}
	h := ConnectionHeader{Name: "X-Lab", Value: "1"}
	withHeader := a
	withHeader.CustomHeaders = []ConnectionHeader{h}
	if a.InputsDigest() == withHeader.InputsDigest() {
		t.Fatal("a changed header set must change the digest")
	}
	if !strings.HasPrefix(a.InputsDigest(), "sha256:") {
		t.Fatalf("the digest rides the C2-12 grammar, got %q", a.InputsDigest())
	}
}

// TestConnectionFingerprint pins the create-idempotency fingerprint: the
// same body (including the credential bytes) fingerprints identically; any
// body difference — including only the credential — fingerprints
// differently. The fingerprint is a one-way digest; the value never
// appears in it as recoverable content.
func TestConnectionFingerprint(t *testing.T) {
	secret := []byte("karyora-test-canary-fake-value")
	base := FingerprintOf("mock", "https://x.invalid", "m", nil, secret)
	same := FingerprintOf("mock", "https://x.invalid", "m", nil, secret)
	if base != same {
		t.Fatal("the same body must fingerprint identically")
	}
	otherSecret := FingerprintOf("mock", "https://x.invalid", "m", nil, []byte("karyora-test-canary-other-fake"))
	if base == otherSecret {
		t.Fatal("a different credential must change the fingerprint")
	}
	noSecret := FingerprintOf("mock", "https://x.invalid", "m", nil, nil)
	if base == noSecret {
		t.Fatal("the credential's presence must change the fingerprint")
	}
	otherModel := FingerprintOf("mock", "https://x.invalid", "other", nil, secret)
	if base == otherModel {
		t.Fatal("a changed model id must change the fingerprint")
	}
	if strings.Contains(base, string(secret)) {
		t.Fatal("the fingerprint must never embed the credential bytes")
	}
}

// TestConnectionHeaderRestriction pins the Core-side restriction of the
// optional custom headers (MB §12.5; the single rule the adapter
// delegates to): forbidden credential-bearing/hop-by-hop names refuse,
// malformed names refuse, header injection (newlines) refuses, oversized
// values refuse, and an over-bounded set refuses.
func TestConnectionHeaderRestriction(t *testing.T) {
	ok := []ConnectionHeader{{Name: "X-Lab", Value: "1"}}
	if err := ValidateHeaders(ok); err != nil {
		t.Fatalf("a benign header must pass: %v", err)
	}
	for _, name := range []string{"Authorization", "host", "Cookie", "X-Forwarded-For", "Proxy-Authorization"} {
		if err := ValidateConnectionHeader(name, "x"); err == nil {
			t.Errorf("header %q must be forbidden", name)
		}
	}
	if err := ValidateConnectionHeader("Bad Name", "x"); err == nil {
		t.Error("a malformed header name must refuse")
	}
	if err := ValidateConnectionHeader("X-Lab", "a\r\nb"); err == nil {
		t.Error("header injection must refuse")
	}
	if err := ValidateConnectionHeader("X-Lab", strings.Repeat("x", 1025)); err == nil {
		t.Error("an oversized header value must refuse")
	}
	many := make([]ConnectionHeader, 17)
	for i := range many {
		many[i] = ConnectionHeader{Name: "X-Lab", Value: "1"}
	}
	if err := ValidateHeaders(many); err == nil {
		t.Error("an over-bounded header set must refuse")
	}
}

// TestConnectionRecordValidation pins the record-shape law: the Core-issued
// id, provider id, exact Model ID, and secret reference are all required;
// embedded URL credentials refuse.
func TestConnectionRecordValidation(t *testing.T) {
	base := ConnectionRecord{ConnectionID: "conn-1", Provider: "mock", ModelID: "m", SecretRef: "connection:conn-1/api-key"}
	if err := ValidateRecord(base); err != nil {
		t.Fatalf("a well-formed record must pass: %v", err)
	}
	noID := base
	noID.ConnectionID = ""
	if err := ValidateRecord(noID); err == nil {
		t.Error("a record without its Core-issued id must refuse")
	}
	noModel := base
	noModel.ModelID = " "
	if err := ValidateRecord(noModel); err == nil {
		t.Error("a record without the exact Model ID must refuse")
	}
	noRef := base
	noRef.SecretRef = ""
	if err := ValidateRecord(noRef); err == nil {
		t.Error("a record without its secret reference must refuse")
	}
	embedded := base
	embedded.BaseURL = "https://user:pass@example.invalid"
	if err := ValidateRecord(embedded); err == nil {
		t.Error("URL-embedded credentials must refuse")
	}
}

func mustJSONString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
