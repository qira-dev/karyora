/**
 * The client-side session holder (C-1; the P-S3-01 recorded candidate's
 * client half, as far as THIS milestone records it).
 *
 * The conservative holding rule, exactly as the milestone records it: the
 * session token is held IN MEMORY ONLY for the app session lifetime.
 * Nothing is persisted — no localStorage, no sessionStorage, no cookies,
 * no caches, no logs. A page reload discards the token and the user
 * re-establishes the session with a fresh one-time credential. The
 * client-side token-persistence question (an in-memory-only rule vs R-11
 * and any future reviewed holder design) stays its recorded OPEN item
 * with the Core/security owners — nothing here decides it.
 *
 * Surface rules honored by construction (R-3/R-11): the token never
 * enters a URL, a Referer value, a log line, or any rendered string.
 * Only the safe metadata (the Core's hash-prefix session id, the scopes,
 * the absolute expiry) is exposed for display.
 */

/** Safe display metadata — never carries the token secret. */
export interface CoreSessionMetadata {
  /** The Core's log-safe 16-hex hash-prefix session identifier. */
  readonly id: string;
  /** The operation-class scopes the Core granted. */
  readonly scopes: readonly string[];
  /** The Core-set absolute expiry (RFC 3339, as the Core encodes it). */
  readonly expires: string;
}

interface HeldSession {
  readonly secret: string;
  readonly metadata: CoreSessionMetadata;
}

let held: HeldSession | null = null;

export function holdSession(secret: string, metadata: CoreSessionMetadata): void {
  held = { secret, metadata };
}

/** The bearer secret for the Authorization header — never for display. */
export function heldSecret(): string | null {
  return held === null ? null : held.secret;
}

/** The safe metadata for display surfaces. */
export function heldSessionMetadata(): CoreSessionMetadata | null {
  return held === null ? null : held.metadata;
}

/** Discards the token from memory (expiry/revocation recovery). The
 *  metadata snapshot is retained for the truthful "no longer authorized"
 *  display; the secret is gone. */
export function discardHeldSecret(): CoreSessionMetadata | null {
  if (held === null) return null;
  const metadata = held.metadata;
  held = null;
  return metadata;
}

/** Whether a secret is currently held (never returns the secret). */
export function isSessionHeld(): boolean {
  return held !== null;
}
