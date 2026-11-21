/**
 * RFC 9457 / D-35 Localized Machine-Error Resolver (UIR-9.1, G-08, G-10).
 *
 * Enforces the non-negotiable Karyora rule (Blueprint §6.2, §16.4, §21.2):
 * Backend error envelopes carry machine-only codes (`code`, `retryable`,
 * `correlationId`, `data`). User-facing copy is NEVER sourced from raw Go/backend
 * strings; every code resolves through localized i18n keys across all 13 locales.
 */

import type { RecoveryPanelDescriptor } from "../coreclient/types";

export const RFC9457_ERROR_SUFFIXES = [
  "validationFailed",
  "configConflict",
  "authzDenied",
  "notFound",
  "conflictState",
  "conflictIdempotencyKey",
  "singleInstanceConflict",
  "storageUnavailable",
  "migrationFailed",
  "stateCorrupt",
  "recoveryRequired",
  "rateLimited",
  "internal",
  "unknownCode",
  "unknownOutcomeS4",
] as const;

export type Rfc9457ErrorSuffix = (typeof RFC9457_ERROR_SUFFIXES)[number];

export const RFC9457_ACTION_KEY_MAP: Readonly<
  Record<RecoveryPanelDescriptor["primaryAction"], string>
> = {
  retry: "errors.actions.retry",
  "reconcile-s4": "errors.actions.reconcileS4",
  "resolve-conflict": "errors.actions.resolveConflict",
  reauthenticate: "errors.actions.reauthenticate",
  "switch-core-connection": "errors.actions.switchCoreConnection",
  "open-model-settings": "errors.actions.openModelSettings",
  dismiss: "errors.actions.dismiss",
};

/**
 * Exhaustive list of RFC 9457 i18n leaf keys that MUST exist and be non-empty
 * across all 13 locked locales (`en`, `bn`, `zh-CN`, `es`, `pt-BR`, `ja`, `ru`,
 * `ko`, `de`, `fr`, `id`, `tr`, `vi`) to pass Gate G-10.
 */
export const RFC9457_REQUIRED_I18N_KEYS: readonly string[] = [
  "errors.common.draftPreservedSafely",
  "errors.common.correlationIdLabel",
  "errors.common.errorCodeLabel",
  ...Object.values(RFC9457_ACTION_KEY_MAP),
  ...RFC9457_ERROR_SUFFIXES.flatMap((suffix) => [
    `errors.${suffix}.title`,
    `errors.${suffix}.explanation`,
  ]),
];

export type TranslateFn = (
  key: string,
  options?: Record<string, string | number>,
) => string;

export interface LocalizedRecoveryCopy {
  readonly title: string;
  readonly explanation: string;
  readonly draftSafetyNotice: string;
  readonly primaryActionLabel: string;
  readonly errorCodeLabel: string;
  readonly correlationIdLabel: string;
}

/**
 * Resolves a machine-only `RecoveryPanelDescriptor` into localized UI strings
 * using the active i18next translator, guaranteeing zero raw backend text.
 */
export function resolveLocalizedRecoveryCopy(
  t: TranslateFn,
  descriptor: RecoveryPanelDescriptor,
): LocalizedRecoveryCopy {
  const actionKey =
    RFC9457_ACTION_KEY_MAP[descriptor.primaryAction] ?? "errors.actions.dismiss";

  return {
    title: t(descriptor.titleI18nKey),
    explanation: t(descriptor.explanationI18nKey),
    draftSafetyNotice: t(descriptor.draftSafetyI18nKey),
    primaryActionLabel: t(actionKey),
    errorCodeLabel: t("errors.common.errorCodeLabel", {
      code: descriptor.errorCode,
    }),
    correlationIdLabel: t("errors.common.correlationIdLabel", {
      correlationId: descriptor.correlationId,
    }),
  };
}
