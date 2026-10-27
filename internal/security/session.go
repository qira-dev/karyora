package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// Scope is one operation class a session token carries (C2-14 §3):
// lifecycle/status operations vs data operations.
type Scope string

const (
	ScopeLifecycle Scope = "lifecycle"
	ScopeData      Scope = "data"
)

// Session lifetimes are Proposed values pending owner/technical acceptance
// (C2-14 §3); they are contract constants here, not operator-tunable
// configuration — session policy belongs to the security contract until the
// owner records a configuration surface for it.
const (
	SessionAbsoluteTTL = 24 * time.Hour
	SessionIdleTimeout = 8 * time.Hour
	// BootstrapTTL is the one-time bootstrap credential's short lifetime.
	BootstrapTTL = 5 * time.Minute
	// MaxSessions bounds the in-memory session store (single local user);
	// creation beyond the bound fails closed after expired-entry eviction.
	MaxSessions = 16
	// tokenEntropyBytes is the bearer-secret entropy (256-bit).
	tokenEntropyBytes = 32
)

// Session is one authenticated local session. TokenHash is the SHA-256 of
// the bearer secret — the plaintext token is never stored, logged, or
// returned after issuance.
type Session struct {
	TokenHash  string
	CreatedAt  time.Time
	ExpiresAt  time.Time // absolute TTL
	LastUsedAt time.Time // idle timeout basis
	Scopes     []Scope
	Instance   string // bound local Core instance (C2-14 §3)
	DataDir    string // bound data directory (C2-14 §3)
	Origin     string // origin binding of the creating bootstrap exchange
	Revoked    bool
}

func (s Session) covers(required Scope) bool {
	for _, sc := range s.Scopes {
		if sc == required {
			return true
		}
	}
	return false
}

// sessionStore is the bounded in-memory Phase 2 session store. Sessions are
// process-local by design: a Core restart fails closed (no session
// survives), which is the safe direction for a local single-user slice. A
// durable store can replace it later behind the same methods.
type sessionStore struct {
	mu       sync.Mutex
	byHash   map[string]Session
	consumed map[string]consumedBootstrap // bootstrap credential hash -> minted session
}

type consumedBootstrap struct {
	sessionHash string
	exchangedAt time.Time
}

func newSessionStore() *sessionStore {
	return &sessionStore{
		byHash:   map[string]Session{},
		consumed: map[string]consumedBootstrap{},
	}
}

// put inserts a session, evicting expired/revoked entries first and
// denying (ok=false) when the bounded store is genuinely full.
func (st *sessionStore) put(s Session) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	now := s.CreatedAt
	for h, old := range st.byHash {
		if old.Revoked || now.After(old.ExpiresAt) {
			delete(st.byHash, h)
		}
	}
	if len(st.byHash) >= MaxSessions {
		return false
	}
	st.byHash[s.TokenHash] = s
	return true
}

func (st *sessionStore) get(hash string) (Session, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	s, ok := st.byHash[hash]
	return s, ok
}

func (st *sessionStore) revoke(hash string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if s, ok := st.byHash[hash]; ok {
		s.Revoked = true
		st.byHash[hash] = s
	}
}

func (st *sessionStore) revokeAll() {
	st.mu.Lock()
	defer st.mu.Unlock()
	for h, s := range st.byHash {
		s.Revoked = true
		st.byHash[h] = s
	}
}

func (st *sessionStore) markConsumed(credHash, sessionHash string, at time.Time) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.consumed[credHash] = consumedBootstrap{sessionHash: sessionHash, exchangedAt: at}
}

func (st *sessionStore) consumption(credHash string) (consumedBootstrap, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	c, ok := st.consumed[credHash]
	return c, ok
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", errors.New("security: entropy source unavailable")
	}
	return hex.EncodeToString(b), nil
}

// BootstrapCredential is a one-time, short-lived, origin-bound credential
// issued on trusted local bootstrap (same OS user, same data directory).
// Its string form is a bearer secret: it must never travel in URLs, logs,
// or Referer values (C2-14 §5) and is exchanged exactly once.
type BootstrapCredential struct {
	Secret string
	Expiry time.Time
	Origin string
}

// SessionToken is the bearer token returned by a successful exchange. The
// holder persists it client-side; the Core keeps only the hash.
type SessionToken struct {
	Secret  string
	ID      string // hash prefix — safe for logs/audit as an identifier
	Scopes  []Scope
	Expires time.Time
}

// SessionService implements the local session contract (C2-14 §3): trusted
// local bootstrap issues a one-time bootstrap credential, which exchanges
// exactly once for a session token; replayed bootstrap credentials are
// rejected and invalidate the exchange (the minted session is revoked).
// Tokens are bound to the local Core instance and data directory, carry
// operation-class scopes, and are revocable.
type SessionService struct {
	store      *sessionStore
	InstanceID string
	DataDirID  string
	Clock      func() time.Time
}

// NewSessionService builds the service bound to one local Core instance and
// data directory. Empty bindings fail closed at issue time.
func NewSessionService(instanceID, dataDirID string) *SessionService {
	return &SessionService{
		store:      newSessionStore(),
		InstanceID: instanceID,
		DataDirID:  dataDirID,
		Clock:      time.Now,
	}
}

func (svc *SessionService) now() time.Time {
	if svc.Clock == nil {
		return time.Now()
	}
	return svc.Clock()
}

// IssueBootstrapCredential mints the one-time credential for the given
// origin (empty origin binds to the local process boundary — CLI/IPC; a
// non-empty origin binds the exchange to that HTTP origin, C2-14 §5).
func (svc *SessionService) IssueBootstrapCredential(origin string) (BootstrapCredential, error) {
	if svc.InstanceID == "" || svc.DataDirID == "" {
		return BootstrapCredential{}, errors.New("security: session service lacks instance/data-dir binding")
	}
	if len(origin) > 253 {
		return BootstrapCredential{}, errors.New("security: origin is unbounded")
	}
	secret, err := randomHex(tokenEntropyBytes)
	if err != nil {
		return BootstrapCredential{}, err
	}
	now := svc.now()
	return BootstrapCredential{
		Secret: "bootstrap-" + secret,
		Expiry: now.Add(BootstrapTTL),
		Origin: origin,
	}, nil
}

// Exchange consumes the bootstrap credential exactly once and mints a
// session token with the requested scopes. Replays are rejected and revoke
// the session the first exchange minted (C2-14 §3).
func (svc *SessionService) Exchange(cred BootstrapCredential, origin string, scopes []Scope) (SessionToken, error) {
	if len(scopes) == 0 {
		return SessionToken{}, errors.New("security: exchange requires at least one scope")
	}
	now := svc.now()
	credHash := hashToken(cred.Secret)
	if consumed, replayed := svc.store.consumption(credHash); replayed {
		// Replay detected: reject AND invalidate the original exchange.
		svc.store.revoke(consumed.sessionHash)
		return SessionToken{}, errors.New("security: bootstrap credential replayed; exchange invalidated")
	}
	if now.After(cred.Expiry) {
		return SessionToken{}, errors.New("security: bootstrap credential expired")
	}
	if subtle.ConstantTimeCompare([]byte(cred.Origin), []byte(origin)) != 1 {
		return SessionToken{}, errors.New("security: bootstrap credential origin mismatch")
	}
	secret, err := randomHex(tokenEntropyBytes)
	if err != nil {
		return SessionToken{}, err
	}
	// The stored hash covers the full issued token string (prefix
	// included), so validation hashes exactly what the holder presents.
	tokenHash := hashToken("kry1-" + secret)
	session := Session{
		TokenHash:  tokenHash,
		CreatedAt:  now,
		ExpiresAt:  now.Add(SessionAbsoluteTTL),
		LastUsedAt: now,
		Scopes:     append([]Scope(nil), scopes...),
		Instance:   svc.InstanceID,
		DataDir:    svc.DataDirID,
		Origin:     origin,
	}
	if !svc.store.put(session) {
		return SessionToken{}, errors.New("security: session store exhausted")
	}
	svc.store.markConsumed(credHash, tokenHash, now)
	return SessionToken{
		Secret:  "kry1-" + secret,
		ID:      tokenHash[:16],
		Scopes:  session.Scopes,
		Expires: session.ExpiresAt,
	}, nil
}

type tokenCheck int

const (
	tokenValid tokenCheck = iota
	tokenInvalid
	tokenExpired
	tokenRevoked
)

// validateToken resolves a presented token string to its live session.
func (svc *SessionService) validateToken(token string, required Scope) (Session, tokenCheck) {
	if len(token) < 8 || len(token) > 256 {
		return Session{}, tokenInvalid
	}
	hash := hashToken(token)
	s, ok := svc.store.get(hash)
	if !ok {
		return Session{}, tokenInvalid
	}
	now := svc.now()
	if s.Revoked {
		return s, tokenRevoked
	}
	if now.After(s.ExpiresAt) {
		return s, tokenExpired
	}
	if now.Sub(s.LastUsedAt) > SessionIdleTimeout {
		return s, tokenExpired
	}
	if !s.covers(required) {
		return s, tokenInvalid
	}
	// Touch the idle timestamp (bounded store write).
	s.LastUsedAt = now
	st := svc.store
	st.mu.Lock()
	st.byHash[hash] = s
	st.mu.Unlock()
	return s, tokenValid
}

// Validate reports whether a presented token is live for the required
// scope (used by adapters and tests).
func (svc *SessionService) Validate(token string, required Scope) bool {
	_, c := svc.validateToken(token, required)
	return c == tokenValid
}

// Rotate implements rotation on any scope elevation (C2-14 §3): the new
// token is minted first; the old token is revoked only on success, so an
// entropy failure cannot strand the caller without a session.
func (svc *SessionService) Rotate(token string, elevated []Scope) (SessionToken, error) {
	old, c := svc.validateToken(token, ScopeLifecycle)
	if c != tokenValid {
		return SessionToken{}, errors.New("security: rotation requires a live session")
	}
	now := svc.now()
	secret, err := randomHex(tokenEntropyBytes)
	if err != nil {
		return SessionToken{}, err
	}
	// The rotated session carries the union of the old scopes and the
	// elevation; the absolute TTL restarts (a fresh credential lifetime).
	scopes := append([]Scope(nil), old.Scopes...)
	for _, s := range elevated {
		found := false
		for _, have := range scopes {
			if have == s {
				found = true
				break
			}
		}
		if !found {
			scopes = append(scopes, s)
		}
	}
	session := Session{
		TokenHash:  hashToken("kry1-" + secret),
		CreatedAt:  now,
		ExpiresAt:  now.Add(SessionAbsoluteTTL),
		LastUsedAt: now,
		Scopes:     scopes,
		Instance:   old.Instance,
		DataDir:    old.DataDir,
		Origin:     old.Origin,
	}
	if !svc.store.put(session) {
		return SessionToken{}, errors.New("security: session store exhausted")
	}
	svc.store.revoke(old.TokenHash)
	return SessionToken{Secret: "kry1-" + secret, ID: session.TokenHash[:16], Scopes: scopes, Expires: session.ExpiresAt}, nil
}

// Revoke is the explicit logout path (C2-14 §3).
func (svc *SessionService) Revoke(token string) {
	if len(token) < 8 || len(token) > 256 {
		return
	}
	svc.store.revoke(hashToken(token))
}

// RevokeAll implements lock takeover (C2-08) and credential-change
// revocation: every live session of this instance is invalidated.
func (svc *SessionService) RevokeAll() {
	svc.store.revokeAll()
}

// LiveSessions reports the current live (non-revoked, non-expired) count
// for diagnostics — counts only, never token material.
func (svc *SessionService) LiveSessions() int {
	now := svc.now()
	st := svc.store
	st.mu.Lock()
	defer st.mu.Unlock()
	n := 0
	for _, s := range st.byHash {
		if !s.Revoked && !now.After(s.ExpiresAt) {
			n++
		}
	}
	return n
}

// credentialMatches reports constant-time equality of two credential
// references (used wherever a presented reference must be compared).
func credentialMatches(presented, expected string) bool {
	return hmac.Equal([]byte(presented), []byte(expected))
}
