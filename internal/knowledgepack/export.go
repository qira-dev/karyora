package knowledgepack

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// exportBundle is the canonical export shape (C3-06): the profile that
// defines the byte form, the manifest, and every entry's exact bytes. The
// manifest inside is the authority; entries are the recoverable content.
type exportBundle struct {
	ProfileID  string              `json:"profile_id"`
	Manifest   *Manifest           `json:"manifest"`
	Entries    []exportEntry       `json:"entries"`
	Signatures []SignatureEnvelope `json:"signatures,omitempty"`
}

type exportEntry struct {
	Path        string `json:"path"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"` // exact UTF-8 bytes (imports are text-validated)
}

// timeNow is swappable for deterministic tests.
var timeNow = time.Now

// Export writes the canonical export bundle for one pack version after
// re-verifying it (C3-06): a pack that does not verify is never exported,
// so an export artifact can always be re-imported and re-verified to the
// same root digest. envs carry the optional detached signatures; pass nil
// for unsigned local packs.
func Export(w io.Writer, m *Manifest, entries map[string][]byte, envs []SignatureEnvelope, key ed25519.PublicKey) error {
	if m == nil {
		return fmt.Errorf("knowledgepack: manifest is nil")
	}
	var env *SignatureEnvelope
	if len(envs) > 0 {
		env = &envs[0]
	}
	verdict, err := Verify(m, entries, env, key, timeNow())
	if err != nil {
		return err
	}
	if verdict.Status != VerdictAccepted {
		return fmt.Errorf("knowledgepack: refusing to export a pack with verdict %q (reasons: %v)", verdict.Status, verdict.Reasons)
	}
	bundle := exportBundle{
		ProfileID:  ProfileID,
		Manifest:   m,
		Entries:    make([]exportEntry, 0, len(m.Entries)),
		Signatures: envs,
	}
	for i := range m.Entries {
		e := &m.Entries[i]
		data, ok := entries[e.Path]
		if !ok {
			return fmt.Errorf("knowledgepack: entry %q bytes missing at export", e.Path)
		}
		bundle.Entries = append(bundle.Entries, exportEntry{
			Path:        e.Path,
			ContentType: e.ContentType,
			Content:     string(data),
		})
	}
	canonical, err := Canonicalize(bundle)
	if err != nil {
		return err
	}
	_, err = w.Write(canonical)
	return err
}

// ImportExported parses a canonical export bundle back into a manifest and
// entries, ready for Verify (C3-06 round-trip). It decodes strictly and
// rejects unknown schema versions through Manifest.Validate.
func ImportExported(bundleBytes []byte) (*Manifest, map[string][]byte, []SignatureEnvelope, error) {
	dec := json.NewDecoder(bytes.NewReader(bundleBytes))
	dec.DisallowUnknownFields()
	var bundle exportBundle
	if err := dec.Decode(&bundle); err != nil {
		return nil, nil, nil, fmt.Errorf("knowledgepack: export bundle is malformed: %w", err)
	}
	if bundle.ProfileID != ProfileID {
		return nil, nil, nil, fmt.Errorf("knowledgepack: export profile %q is not %q", bundle.ProfileID, ProfileID)
	}
	if bundle.Manifest == nil {
		return nil, nil, nil, fmt.Errorf("knowledgepack: export bundle lacks a manifest")
	}
	entries := make(map[string][]byte, len(bundle.Entries))
	for _, e := range bundle.Entries {
		entries[e.Path] = []byte(e.Content)
	}
	return bundle.Manifest, entries, bundle.Signatures, nil
}
