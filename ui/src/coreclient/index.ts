export {
  createIdempotencyKey,
  createUnavailableCoreClient,
  createV3CoreClient,
  transportOutcomeToCommandState,
  type CoreClient,
  type CoreCommandKind,
  type CoreCommandResult,
  type CoreErrorEnvelope,
  type CoreOperationKind,
  type CoreQueryResult,
  type CoreRejection,
  type CoreUnknown,
  type V3CoreClient,
} from "./CoreClient";

export {
  parseCoreErrorEnvelope,
  toRecoveryPanelDescriptor,
  toUnknownOutcomeS4RecoveryDescriptor,
} from "./rfc9457";

export {
  SseStreamManager,
  type SseCursorState,
  type SseStreamEvent,
  type SseSubscriptionHandlers,
  type SseTopicKind,
} from "./sseManager";

export type * from "./types";
