package knowledgepack

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// ProfileID is the versioned INTERNAL digest/signature profile (C3-04).
// D-41 stays a technical/security gate for public portability: this profile
// makes local integrity and authenticity testable and repeatable, and makes
// NO public or cross-language compatibility promise. Any change to the
// canonical form bumps this identifier and is a new profile version.
const ProfileID = "uawp-kp-internal-digest-v1"

// SigAlgorithm is the only signature algorithm this profile accepts.
// Unknown or ambiguous algorithms fail closed (C3-04 acceptance).
const SigAlgorithm = "ed25519-v1"

// Canonicalize renders v as canonical JSON for the v1 profile: object keys
// sorted lexicographically at every level, no insignificant whitespace,
// numbers preserved as they decode (json.Number), UTF-8 strings. The same
// logical document always produces the same bytes regardless of Go struct
// field order or map iteration order.
func Canonicalize(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("knowledgepack: canonical marshal: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("knowledgepack: canonical decode: %w", err)
	}
	var buf bytes.Buffer
	if err := writeCanonical(&buf, doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return fmt.Errorf("knowledgepack: canonical key: %w", err)
			}
			buf.Write(kb)
			buf.WriteByte(':')
			if err := writeCanonical(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case string:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Errorf("knowledgepack: canonical string: %w", err)
		}
		buf.Write(b)
	case json.Number:
		buf.WriteString(t.String())
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case nil:
		buf.WriteString("null")
	default:
		return fmt.Errorf("knowledgepack: canonical form of %T is undefined", v)
	}
	return nil
}

// ValidateEntryPath enforces the pack-relative path rules (C3-04): forward
// slashes only, no absolute paths, no dot-dot segments, UTF-8 without
// control characters, bounded length. Ambiguous or escaping paths fail
// closed so an entry can never address anything outside its own pack.
func ValidateEntryPath(path string) error {
	if path == "" {
		return fmt.Errorf("knowledgepack: entry path is empty")
	}
	if len(path) > 256 {
		return fmt.Errorf("knowledgepack: entry path exceeds 256 bytes")
	}
	if !utf8.ValidString(path) {
		return fmt.Errorf("knowledgepack: entry path is not valid UTF-8")
	}
	if strings.Contains(path, "\\") {
		return fmt.Errorf("knowledgepack: entry path %q contains a backslash", path)
	}
	if strings.HasPrefix(path, "/") {
		return fmt.Errorf("knowledgepack: entry path %q is absolute", path)
	}
	if strings.HasSuffix(path, "/") {
		return fmt.Errorf("knowledgepack: entry path %q ends with a separator", path)
	}
	if strings.ContainsRune(path, 0) {
		return fmt.Errorf("knowledgepack: entry path contains a NUL byte")
	}
	for _, r := range path {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("knowledgepack: entry path %q contains a control character", path)
		}
	}
	for _, seg := range strings.Split(path, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("knowledgepack: entry path %q has an empty, dot, or dot-dot segment", path)
		}
	}
	return nil
}

// PackRootDigest computes the versioned pack-root digest (C3-04): the
// sha256 over the canonical JSON of the manifest. Entry digests are part of
// the manifest, so the root covers every entry; entry bytes are verified
// separately against their per-entry digests (verify.go).
func PackRootDigest(m *Manifest) (string, error) {
	if m == nil {
		return "", fmt.Errorf("knowledgepack: manifest is nil")
	}
	canonical, err := Canonicalize(m)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + fmt.Sprintf("%x", sum), nil
}

// SignatureEnvelope is the detached signature for one pack version
// (C3-04). It is carried beside the pack, never inside the manifest it
// signs, so verification can always recompute the signed bytes.
type SignatureEnvelope struct {
	ProfileID string `json:"profile_id"`
	Algorithm string `json:"algorithm"`
	SignerID  string `json:"signer_id"`
	SignedAt  string `json:"signed_at"` // RFC 3339
	Signature string `json:"signature"` // base64 std encoding of the ed25519 signature
}

// Sign produces the detached envelope for the manifest under key
// (C3-04). The caller owns signer identity and timestamp policy.
func Sign(m *Manifest, signerID string, signedAt string, key ed25519.PrivateKey) (*SignatureEnvelope, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("knowledgepack: private key has the wrong size")
	}
	if signerID == "" {
		return nil, fmt.Errorf("knowledgepack: signer identity is required")
	}
	canonical, err := Canonicalize(m)
	if err != nil {
		return nil, err
	}
	sig := ed25519.Sign(key, canonical)
	return &SignatureEnvelope{
		ProfileID: ProfileID,
		Algorithm: SigAlgorithm,
		SignerID:  signerID,
		SignedAt:  signedAt,
		Signature: base64.StdEncoding.EncodeToString(sig),
	}, nil
}

// VerifySignature checks the detached envelope against the exact manifest
// (C3-04). An unknown algorithm, foreign profile, or malformed signature
// fails closed — never a soft pass.
func VerifySignature(m *Manifest, env *SignatureEnvelope, key ed25519.PublicKey) error {
	if env == nil {
		return fmt.Errorf("knowledgepack: signature envelope is absent")
	}
	if env.ProfileID != ProfileID {
		return fmt.Errorf("knowledgepack: signature profile %q is not %q", env.ProfileID, ProfileID)
	}
	if env.Algorithm != SigAlgorithm {
		return fmt.Errorf("knowledgepack: signature algorithm %q is unsupported; refusing to guess", env.Algorithm)
	}
	if env.SignerID == "" {
		return fmt.Errorf("knowledgepack: signature lacks a signer identity")
	}
	sig, err := base64.StdEncoding.DecodeString(env.Signature)
	if err != nil {
		return fmt.Errorf("knowledgepack: signature is not valid base64")
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("knowledgepack: signature size is wrong")
	}
	canonical, err := Canonicalize(m)
	if err != nil {
		return err
	}
	if !ed25519.Verify(key, canonical, sig) {
		return fmt.Errorf("knowledgepack: signature does not verify against the canonical manifest")
	}
	return nil
}
