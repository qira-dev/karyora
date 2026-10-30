package updater

import (
	"os"
	"path/filepath"
	"time"
)

// The durable machine state (C5-18 §5): the ONLY cross-crash artifacts the
// machine ever creates are (a) the disposable staging area, (b) the committed
// backup (the C5-09 path's own record — referenced, never copied here),
// (c) the update journal, and (d) the retained prior binary. This file owns
// (c) plus the persisted client state (the C5-17 §4.2 client-state design:
// highest-seen metadata version, last-validated time, root continuity — plus
// the machine's pause/refusal/last-known-good records). Every write is a
// whole-record atomic replace (temp file + fsync + rename — the delivered
// durability discipline of the outbox/journal foundations, CR-2.3 class);
// nothing here is ever appended-in-place or partially written.

const (
	stateVersion  = 1
	journalVer    = 1
	stateFileName = "state.json"
	journalFile   = "journal.json"
)

// Journal phases (C5-18 §4.2's durable phases; §5.1's journal). A journal in
// any of these phases means an update is mid-flight and startup
// reconciliation (§5.2) must resolve it before anything else runs.
const (
	PhaseStaged      = "STAGED"            // S3 complete: the candidate is downloaded and digest-verified
	PhaseVerified    = "VERIFIED"          // S4 complete: signature + supply-chain checks passed
	PhaseBackedUp    = "BACKED_UP"         // S5 complete: the C5-09 backup committed
	PhaseIntent      = "ACTIVATION_INTENT" // T7: validation passed; the swap is authorized
	PhaseActivated   = "ACTIVATED"         // T8: the swap executed; awaiting health (S8)
	PhaseRollingBack = "ROLLING_BACK"      // T10: the binary rollback is in progress
)

// LastKnownGood is the rollback anchor (C5-18 S9/S10): the version and
// content digest of the retained prior binary. The pointer NEVER regresses
// (it advances only on a health PASS — the §2.2 invariant).
type LastKnownGood struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

// ClientState is the persisted machine state (one atomic JSON record).
type ClientState struct {
	StateVersion int
	// The C5-17 §4.2 client state:
	RootVersionSeen             uint64    `json:"root_version_seen"`
	ReleaseSignerKey            []byte    `json:"release_signer_key"`
	ReleaseSignerKeyFingerprint string    `json:"release_signer_key_fingerprint"`
	RootExpiresAt               time.Time `json:"root_expires_at"`
	HighestSeenMetadataVersion  uint64    `json:"highest_seen_metadata_version"`
	HighestSeenManifestDigest   string    `json:"highest_seen_manifest_digest"`
	LastValidated               time.Time `json:"last_validated"`
	// The machine's own records:
	Paused         bool          `json:"paused"`
	PauseReason    string        `json:"pause_reason,omitempty"`
	RefusedThrough string        `json:"refused_through,omitempty"` // the highest refused release version (a failed update)
	LastKnownGood  LastKnownGood `json:"last_known_good"`
}

// Journal is the update journal (C5-18 §5.1's v1 fields): a small durable
// record whose existence and phase drive the startup reconciliation.
type Journal struct {
	JournalVersion  int       `json:"journal_version"`
	Phase           string    `json:"phase"`
	OldVersion      string    `json:"old_version"`
	NewVersion      string    `json:"new_version"`
	CandidateDigest string    `json:"candidate_digest"`
	CandidatePath   string    `json:"candidate_path"`
	BackupReference string    `json:"backup_reference,omitempty"`
	RetainedDigest  string    `json:"retained_digest,omitempty"`
	MigrationFrom   int       `json:"migration_from"`
	MigrationTo     int       `json:"migration_to"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// store paths (the Layout's state directory — the update-state class; never
// the user-data classes, C5-15 §5.0).
func statePath(layout Layout) string   { return filepath.Join(layout.StateDir, stateFileName) }
func journalPath(layout Layout) string { return filepath.Join(layout.StateDir, journalFile) }

// loadState reads the persisted client state; a missing file is the fresh
// zero state (not an error); a corrupt file is a hard typed failure — the
// machine never invents state.
func loadState(layout Layout) (ClientState, error) {
	var st ClientState
	raw, err := os.ReadFile(statePath(layout))
	if err != nil {
		if os.IsNotExist(err) {
			return ClientState{StateVersion: stateVersion}, nil
		}
		return st, fail(ReasonPortUnavailable, "read update state: %v", err)
	}
	if err := jsonUnmarshalStrict(raw, &st); err != nil {
		return st, fail(ReasonMalformed, "update state is corrupt: %v", err)
	}
	if st.StateVersion != stateVersion {
		return st, fail(ReasonMalformed, "update state version %d is not %d", st.StateVersion, stateVersion)
	}
	return st, nil
}

// saveState atomically replaces the client state.
func saveState(layout Layout, st ClientState) error {
	return writeAtomic(statePath(layout), st)
}

// loadJournal reads the update journal; a missing file means no update is
// mid-flight (nil journal).
func loadJournal(layout Layout) (*Journal, error) {
	raw, err := os.ReadFile(journalPath(layout))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fail(ReasonPortUnavailable, "read update journal: %v", err)
	}
	var j Journal
	if err := jsonUnmarshalStrict(raw, &j); err != nil {
		return nil, fail(ReasonMalformed, "update journal is corrupt: %v", err)
	}
	if j.JournalVersion != journalVer {
		return nil, fail(ReasonMalformed, "update journal version %d is not %d", j.JournalVersion, journalVer)
	}
	return &j, nil
}

// saveJournal atomically replaces the journal (whole-record replace —
// §5.1: "every write is a whole-record replace").
func saveJournal(layout Layout, j *Journal) error {
	if j == nil {
		if err := os.Remove(journalPath(layout)); err != nil && !os.IsNotExist(err) {
			return fail(ReasonPortUnavailable, "remove update journal: %v", err)
		}
		return nil
	}
	j.UpdatedAt = time.Now().UTC()
	return writeAtomic(journalPath(layout), *j)
}

// writeAtomic replaces the file at path with the JSON encoding of v using
// the temp-file + fsync + rename discipline (the delivered durability
// class). The directory sync is best-effort: POSIX needs it for rename
// durability; platforms where directory fsync is unsupported simply skip it
// (the whole-record replace semantics are unchanged).
func writeAtomic(path string, v any) error {
	data, err := jsonMarshal(v)
	if err != nil {
		return fail(ReasonPortUnavailable, "marshal state record: %v", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".updtmp-*")
	if err != nil {
		return fail(ReasonPortUnavailable, "create temp state file: %v", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fail(ReasonPortUnavailable, "write temp state file: %v", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fail(ReasonPortUnavailable, "fsync temp state file: %v", err)
	}
	if err := tmp.Close(); err != nil {
		return fail(ReasonPortUnavailable, "close temp state file: %v", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fail(ReasonPortUnavailable, "chmod temp state file: %v", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fail(ReasonPortUnavailable, "replace state file: %v", err)
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
