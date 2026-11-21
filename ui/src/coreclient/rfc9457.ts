/**
 * RFC 9457 / D-35 Machine-Only Error Envelope Parser & RecoveryPanel Mapper (UIR-1.1, G-08, G-10).
 *
 * Non-negotiable rule (Blueprint §6.2, §16.4, §21.2):
 * Backend error envelopes carry machine-only error codes (`code`, `retryable`,
 * `correlationId`, `data`). The UI NEVER displays raw backend error strings or
 * Go stack traces as user copy; it resolves localized `i18n` keys from the
 * machine `code` and builds a standardized 5-element `RecoveryPanelDescriptor`.
 */

import type { CoreWireEnvelope } from "./httpLocal/wire";
import type { RecoveryPanelDescriptor } from "./types";

const KNOWN_ERROR_CODE_I18N_MAP: Readonly<
  Record<
    string,
    {
      readonly stateKind: RecoveryPanelDescriptor["stateKind"];
      readonly i18nKeySuffix: string;
      readonly primaryAction: RecoveryPanelDescriptor["primaryAction"];
    }
  >
> = {
  VALIDATION_FAILED: {
    stateKind: "error",
    i18nKeySuffix: "validationFailed",
    primaryAction: "dismiss",
  },
  CONFIG_CONFLICT: {
    stateKind: "conflict",
    i18nKeySuffix: "configConflict",
    primaryAction: "resolve-conflict",
  },
  AUTHZ_DENIED: {
    stateKind: "permission-denied",
    i18nKeySuffix: "authzDenied",
    primaryAction: "reauthenticate",
  },
  NOT_FOUND: {
    stateKind: "error",
    i18nKeySuffix: "notFound",
    primaryAction: "dismiss",
  },
  CONFLICT_STATE: {
    stateKind: "conflict",
    i18nKeySuffix: "conflictState",
    primaryAction: "resolve-conflict",
  },
  CONFLICT_IDEMPOTENCY_KEY: {
    stateKind: "conflict",
    i18nKeySuffix: "conflictIdempotencyKey",
    primaryAction: "reconcile-s4",
  },
  SINGLE_INSTANCE_CONFLICT: {
    stateKind: "conflict",
    i18nKeySuffix: "singleInstanceConflict",
    primaryAction: "switch-core-connection",
  },
  STORAGE_UNAVAILABLE: {
    stateKind: "error",
    i18nKeySuffix: "storageUnavailable",
    primaryAction: "retry",
  },
  MIGRATION_FAILED: {
    stateKind: "error",
    i18nKeySuffix: "migrationFailed",
    primaryAction: "dismiss",
  },
  STATE_CORRUPT: {
    stateKind: "error",
    i18nKeySuffix: "stateCorrupt",
    primaryAction: "dismiss",
  },
  RECOVERY_REQUIRED: {
    stateKind: "unknown-outcome-s4",
    i18nKeySuffix: "recoveryRequired",
    primaryAction: "reconcile-s4",
  },
  RATE_LIMITED: {
    stateKind: "rate-limited",
    i18nKeySuffix: "rateLimited",
    primaryAction: "retry",
  },
  INTERNAL: {
    stateKind: "error",
    i18nKeySuffix: "internal",
    primaryAction: "retry",
  },
};

/**
 * Safely parses an unknown JSON payload into a machine-only D-35 / RFC 9457
 * `CoreWireEnvelope`, stripping any raw prose (`title`, `detail`, `message`,
 * `stack`) so untrusted or non-localized backend prose never reaches the DOM.
 */
export function parseCoreErrorEnvelope(
  raw: unknown,
  fallbackCorrelationId = "unreported",
): CoreWireEnvelope | null {
  if (raw === null || typeof raw !== "object" || Array.isArray(raw)) {
    return null;
  }
  const obj = raw as Record<string, unknown>;
  const rawEnvelope =
    obj.error !== null && typeof obj.error === "object" && !Array.isArray(obj.error)
      ? (obj.error as Record<string, unknown>)
      : obj;

  const code =
    typeof rawEnvelope.code === "string" && rawEnvelope.code.trim().length > 0
      ? rawEnvelope.code.trim()
      : null;

  if (!code) {
    return null;
  }

  const retryable = Boolean(rawEnvelope.retryable);
  const correlationId =
    typeof rawEnvelope.correlationId === "string" &&
    rawEnvelope.correlationId.trim().length > 0
      ? rawEnvelope.correlationId.trim()
      : typeof rawEnvelope.correlation_id === "string" &&
          rawEnvelope.correlation_id.trim().length > 0
        ? rawEnvelope.correlation_id.trim()
        : fallbackCorrelationId;

  const rawData =
    rawEnvelope.data !== null &&
    typeof rawEnvelope.data === "object" &&
    !Array.isArray(rawEnvelope.data)
      ? (rawEnvelope.data as Record<string, unknown>)
      : {};

  // Strip any accidental secret or raw prose keys from bounded data
  const sanitizedData: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(rawData)) {
    const lower = k.toLowerCase();
    if (
      lower.includes("secret") ||
      lower.includes("token") ||
      lower.includes("password") ||
      lower.includes("apikey") ||
      lower.includes("api_key") ||
      lower === "detail" ||
      lower === "message" ||
      lower === "stack"
    ) {
      continue;
    }
    sanitizedData[k] = v;
  }

  return {
    code,
    retryable,
    correlationId,
    data: sanitizedData,
  };
}

/**
 * Maps a `CoreWireEnvelope` (outcome S3) into the standardized 5-element
 * `RecoveryPanelDescriptor` (Blueprint §21.2).
 */
export function toRecoveryPanelDescriptor(
  envelope: CoreWireEnvelope,
  httpStatus?: number,
): RecoveryPanelDescriptor {
  const mapped = KNOWN_ERROR_CODE_I18N_MAP[envelope.code];
  const suffix = mapped ? mapped.i18nKeySuffix : "unknownCode";

  const sessionReason =
    typeof envelope.data.sessionReason === "string"
      ? envelope.data.sessionReason
      : undefined;

  const isAuthRequired =
    sessionReason === "SESSION_INVALID" ||
    sessionReason === "SESSION_EXPIRED" ||
    sessionReason === "SESSION_REVOKED";

  const stateKind: RecoveryPanelDescriptor["stateKind"] = isAuthRequired
    ? "auth-required"
    : mapped
      ? mapped.stateKind
      : "error";

  const primaryAction: RecoveryPanelDescriptor["primaryAction"] = isAuthRequired
    ? "reauthenticate"
    : mapped
      ? mapped.primaryAction
      : envelope.retryable
        ? "retry"
        : "dismiss";

  return {
    stateKind,
    titleI18nKey: `errors.${suffix}.title`,
    explanationI18nKey: `errors.${suffix}.explanation`,
    draftSafetyI18nKey: "errors.common.draftPreservedSafely",
    primaryAction,
    errorCode: envelope.code,
    retryable: envelope.retryable,
    correlationId: envelope.correlationId,
    httpStatus,
  };
}

/**
 * Builds the standardized S4 Unknown-Outcome `RecoveryPanelDescriptor`
 * when a network drop or lost response leaves a mutation outcome uncertain
 * (Blueprint §6.2, §21.2).
 */
export function toUnknownOutcomeS4RecoveryDescriptor(
  correlationId = "unreported",
): RecoveryPanelDescriptor {
  return {
    stateKind: "unknown-outcome-s4",
    titleI18nKey: "errors.unknownOutcomeS4.title",
    explanationI18nKey: "errors.unknownOutcomeS4.explanation",
    draftSafetyI18nKey: "errors.common.draftPreservedSafely",
    primaryAction: "reconcile-s4",
    errorCode: "UNKNOWN_OUTCOME_S4",
    retryable: false,
    correlationId,
  };
}
