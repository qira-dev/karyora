/**
 * The browser HTTP transport for the typed CoreClient boundary (IMPL-2).
 * See wire.ts for the honest typing boundary and transport.ts for the
 * truthfulness rules. The session holder (session.ts) keeps the token in
 * memory only — never persisted.
 */
export {
  createHttpLocalTransport,
  type ExchangeOutcome,
  type HttpLocalTransport,
  type ProbeResult,
  type TransportOutcome,
} from "./transport";
export {
  discardHeldSecret,
  heldSessionMetadata,
  isSessionHeld,
  type CoreSessionMetadata,
} from "./session";
export type {
  CoreWireBoundaryReason,
  CoreWireCode,
  CoreWireEnvelope,
  CoreWireRetryClass,
  CoreWireSessionReason,
  WireAttempt,
  WireEventRecord,
  WireExchangeRequest,
  WireJob,
  WireJobRequirements,
  WireReplayOutcome,
  WireReplayResult,
  WireSessionResult,
  WireStatusResult,
  WireSubmitRequest,
  WireSubmitResult,
} from "./wire";
