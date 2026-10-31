import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef, useState } from "react";

import { validateBaseUrl } from "../../app/firstRun/baseUrl";
import {
  CUSTOM_API_PRESET_ID,
  PROVIDER_PRESETS,
} from "../../app/firstRun/providerPresets";
import type { ProviderDraft } from "../../app/firstRun/firstRunState";
import { useFirstRun } from "../../app/firstRun/firstRunState";
import { createUnavailableCoreClient } from "../../coreclient";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import { useT } from "../../i18n/useT";
import styles from "./ProviderSetupForm.module.css";

/**
 * The provider setup form (C-07; UI-11 LOCKED presets; UI-12 Custom API
 * field set; UI-13 capability display; UI-22 billing label; X-6 secrets).
 *
 * Shared by the first-run wizard (step 2) and the connections settings
 * page — the contract's own reuse ("this is the same provider setup
 * step"). Contract rules carried here:
 *
 * - Presets are the UI-11 LOCKED list; Custom API unlocks the UI-12
 *   field set. The API-format selector's option source is the Core's
 *   implemented adapters (SEL-4) — with the provider contracts unwired,
 *   it renders its honest unavailable state and invents no enum.
 * - Base URL client-side validation per §18.4/MB §12.5 (see baseUrl.ts):
 *   format check only — nothing here implies an endpoint was verified.
 * - The API key lives ONLY in this component's local state while the
 *   form is mounted: it never enters the app draft, localStorage,
 *   sessionStorage, URLs, logs, or telemetry (X-6; C-07). Unmounting the
 *   form discards it. The explicit reveal exists for unsaved entry only.
 * - `Test connection` (M-25) and `Save securely` (M-24) are Core-owned
 *   commands. With no transport, the CoreClient reports `unavailable` —
 *   the truthful not-submitted state (S0-with-reason) — and the outcome
 *   panel says exactly that. No fake success, no fake verification.
 * - Capability renders `Not verified` (UI-13): unknown stays unknown;
 *   nothing is inferred from provider or model names.
 * - The billing label carries the UI-22 LOCKED rule: provider-billed
 *   BYOK, separate from any product maintenance plan.
 * - After an unsuccessful save the non-secret entries are preserved and
 *   the key field is cleared (the safe-retry semantics; the key is
 *   never restored from storage because it is never stored).
 */

export interface ProviderSetupFormHandle {
  /** True when any entry (including an unsaved key) differs from the
   *  clean snapshot — the J-8 dirty signal. */
  isDirty: () => boolean;
  /** Discard back to the clean snapshot and clear the key (J-8). */
  discard: () => void;
  /** Run the save flow (J-8 guard's Save action). Validates first; on
   *  invalid input it focuses the first invalid field instead. */
  attemptSave: () => void;
  /** Whether the form currently passes client-side validation. */
  isValid: () => boolean;
}

type Outcome = "test-unavailable" | "save-unavailable" | null;

interface FieldErrors {
  baseUrl: string | null;
  modelId: string | null;
  apiKey: string | null;
  headers: string | null;
}

const NO_ERRORS: FieldErrors = { baseUrl: null, modelId: null, apiKey: null, headers: null };

export const ProviderSetupForm = forwardRef<ProviderSetupFormHandle>(
  function ProviderSetupForm(_props, ref) {
    const t = useT();
    const { providerDraft, setProviderDraft } = useFirstRun();

    const [presetId, setPresetId] = useState(providerDraft.presetId);
    const [baseUrl, setBaseUrl] = useState(providerDraft.baseUrl);
    const [modelId, setModelId] = useState(providerDraft.modelId);
    const [headers, setHeaders] = useState(providerDraft.headers);
    // The secret NEVER enters the app draft or any storage (X-6): it
    // lives only here, in component state, for the lifetime of the form.
    const [apiKey, setApiKey] = useState("");
    const [revealSecret, setRevealSecret] = useState(false);
    const [errors, setErrors] = useState<FieldErrors>(NO_ERRORS);
    const [submitAttempted, setSubmitAttempted] = useState(false);
    const [outcome, setOutcome] = useState<Outcome>(null);
    const [pending, setPending] = useState(false);

    // The clean snapshot this form instance started from (J-8: unchanged
    // data is never re-prompted; discard returns exactly here).
    const cleanSnapshot = useRef<ProviderDraft>({ ...providerDraft });
    const baseUrlRef = useRef<HTMLInputElement>(null);
    const modelIdRef = useRef<HTMLInputElement>(null);
    const apiKeyRef = useRef<HTMLInputElement>(null);
    const headersRef = useRef<HTMLTextAreaElement>(null);

    const isCustom = presetId === CUSTOM_API_PRESET_ID;

    // Persist the NON-SECRET draft so wizard back-navigation and re-entry
    // preserve entries (§8.13; C-06 draft rule). The key is excluded by
    // construction (it is not part of ProviderDraft).
    useEffect(() => {
      setProviderDraft({ presetId, baseUrl, modelId, headers });
    }, [presetId, baseUrl, modelId, headers, setProviderDraft]);

    const parsedHeaders = useCallback((): { ok: boolean } => {
      const lines = headers.split("\n");
      for (const line of lines) {
        const trimmed = line.trim();
        if (trimmed.length === 0) continue;
        if (!/^[!#$%&'*+\-.^_`|~0-9A-Za-z]+:\s?.+$/.test(trimmed)) {
          return { ok: false };
        }
      }
      return { ok: true };
    }, [headers]);

    const computeErrors = useCallback((): FieldErrors => {
      const next: FieldErrors = { ...NO_ERRORS };
      if (isCustom) {
        const baseUrlCheck = validateBaseUrl(baseUrl);
        if (!baseUrlCheck.valid) {
          // Static key lookup — typed keys (G-10) admit no dynamic keys.
          const reasonKey: Record<
            "invalid" | "scheme" | "insecure" | "embedded-credentials",
            "provider.baseUrlInvalid" | "provider.baseUrlInvalidScheme" | "provider.baseUrlInsecure" | "provider.baseUrlEmbeddedCredentials"
          > = {
            invalid: "provider.baseUrlInvalid",
            scheme: "provider.baseUrlInvalidScheme",
            insecure: "provider.baseUrlInsecure",
            "embedded-credentials": "provider.baseUrlEmbeddedCredentials",
          };
          next.baseUrl = t(reasonKey[baseUrlCheck.reason]);
        }
      }
      if (modelId.trim().length === 0) {
        next.modelId = t("provider.modelIdRequired");
      }
      if (apiKey.length === 0) {
        next.apiKey = t("provider.apiKeyRequired");
      }
      if (!parsedHeaders().ok) {
        next.headers = t("provider.headersInvalid");
      }
      return next;
    }, [isCustom, baseUrl, modelId, apiKey, parsedHeaders, t]);

    const isValidNow = useCallback((): boolean => {
      const e = computeErrors();
      return e.baseUrl === null && e.modelId === null && e.apiKey === null && e.headers === null;
    }, [computeErrors]);

    // Live re-validation after the first submit attempt (§8.2: no
    // pre-interaction errors; after a failed submit errors track input).
    useEffect(() => {
      if (!submitAttempted) return;
      setErrors(computeErrors());
    }, [submitAttempted, computeErrors]);

    const focusFirstInvalid = useCallback((e: FieldErrors) => {
      if (e.baseUrl) baseUrlRef.current?.focus();
      else if (e.modelId) modelIdRef.current?.focus();
      else if (e.apiKey) apiKeyRef.current?.focus();
      else if (e.headers) headersRef.current?.focus();
    }, []);

    const runCommand = useCallback(
      async (kind: "provider-connection-test" | "provider-connection-save") => {
        setSubmitAttempted(true);
        const e = computeErrors();
        setErrors(e);
        const invalid =
          e.baseUrl !== null || e.modelId !== null || e.apiKey !== null || e.headers !== null;
        if (invalid) {
          focusFirstInvalid(e);
          return;
        }
        // No duplicate submit (§8.1): the pending window is closed to a
        // second attempt. No pending UI is fabricated: the unavailable
        // instance resolves immediately, honestly.
        setPending(true);
        try {
          const client = createUnavailableCoreClient();
          const result = await client.command(kind);
          // The only reachable result in this milestone is
          // `unavailable` (never submitted). Rejection/unknown/accepted
          // would arrive only from a live transport; rendering them
          // here would fabricate a submission that did not happen, so
          // they fall through to the same truthful not-submitted panel.
          if (result.kind === "unavailable") {
            setOutcome(kind === "provider-connection-test" ? "test-unavailable" : "save-unavailable");
            if (kind === "provider-connection-save") {
              // Safe-retry semantics (§8.12 Save-provider row): the key
              // is cleared and never restored from storage.
              setApiKey("");
              setRevealSecret(false);
            }
          }
        } finally {
          setPending(false);
        }
      },
      [computeErrors, focusFirstInvalid],
    );

    useImperativeHandle(
      ref,
      () => ({
        isDirty: () => {
          const snapshot = cleanSnapshot.current;
          const fieldsChanged =
            presetId !== snapshot.presetId ||
            baseUrl !== snapshot.baseUrl ||
            modelId !== snapshot.modelId ||
            headers !== snapshot.headers;
          return fieldsChanged || apiKey.length > 0;
        },
        discard: () => {
          const snapshot = cleanSnapshot.current;
          setPresetId(snapshot.presetId);
          setBaseUrl(snapshot.baseUrl);
          setModelId(snapshot.modelId);
          setHeaders(snapshot.headers);
          setApiKey("");
          setRevealSecret(false);
          setErrors(NO_ERRORS);
          setSubmitAttempted(false);
          setOutcome(null);
          setProviderDraft({ ...snapshot });
        },
        attemptSave: () => {
          void runCommand("provider-connection-save");
        },
        isValid: isValidNow,
      }),
      [presetId, baseUrl, modelId, headers, apiKey, runCommand, isValidNow, setProviderDraft],
    );

    const errorCount =
      (errors.baseUrl ? 1 : 0) + (errors.modelId ? 1 : 0) + (errors.apiKey ? 1 : 0) + (errors.headers ? 1 : 0);

    return (
      <div className={styles.form}>
        <p className={styles.emptyState}>{t("provider.emptyState")}</p>

        <div className={styles.field}>
          <label htmlFor="provider-preset" className={styles.label}>
            {t("provider.presetLabel")}
          </label>
          <select
            id="provider-preset"
            className={styles.select}
            value={presetId}
            onChange={(event) => setPresetId(event.target.value)}
          >
            <option value="">{t("provider.presetPrompt")}</option>
            {PROVIDER_PRESETS.map((preset) => (
              <option key={preset.id} value={preset.id}>
                {preset.name}
              </option>
            ))}
            <option value={CUSTOM_API_PRESET_ID}>{t("provider.presetCustom")}</option>
          </select>
          <p className={styles.help}>{t("provider.presetHelp")}</p>
        </div>

        {isCustom ? (
          <>
            <div className={styles.field}>
              <label htmlFor="provider-api-format" className={styles.label}>
                {t("provider.apiFormatLabel")}
              </label>
              <select id="provider-api-format" className={styles.select} disabled aria-disabled="true">
                <option>{t("states.unavailable")}</option>
              </select>
              <p className={styles.help}>{t("provider.apiFormatUnavailable")}</p>
            </div>

            <div className={styles.field}>
              <label htmlFor="provider-base-url" className={styles.label}>
                {t("provider.baseUrlLabel")}
              </label>
              <input
                ref={baseUrlRef}
                id="provider-base-url"
                className={styles.input}
                type="text"
                value={baseUrl}
                onChange={(event) => setBaseUrl(event.target.value)}
                autoComplete="off"
                spellCheck={false}
                aria-invalid={errors.baseUrl ? true : undefined}
                aria-describedby={errors.baseUrl ? "provider-base-url-error" : "provider-base-url-help"}
              />
              {errors.baseUrl ? (
                <p id="provider-base-url-error" className={styles.error}>
                  {errors.baseUrl}
                </p>
              ) : (
                <p id="provider-base-url-help" className={styles.help}>
                  {t("provider.baseUrlHelp")}
                </p>
              )}
            </div>
          </>
        ) : null}

        <div className={styles.field}>
          <label htmlFor="provider-model-id" className={styles.label}>
            {t("provider.modelIdLabel")}
          </label>
          <input
            ref={modelIdRef}
            id="provider-model-id"
            className={styles.input}
            type="text"
            value={modelId}
            onChange={(event) => setModelId(event.target.value)}
            autoComplete="off"
            spellCheck={false}
            aria-invalid={errors.modelId ? true : undefined}
            aria-describedby={errors.modelId ? "provider-model-id-error" : "provider-model-id-help"}
          />
          {errors.modelId ? (
            <p id="provider-model-id-error" className={styles.error}>
              {errors.modelId}
            </p>
          ) : (
            <p id="provider-model-id-help" className={styles.help}>
              {t("provider.modelIdHelp")}
            </p>
          )}
        </div>

        <div className={styles.field}>
          <span className={styles.label} id="provider-api-key-label">
            {t("provider.apiKeyLabel")}
          </span>
          <div className={styles.secretRow}>
            <input
              ref={apiKeyRef}
              id="provider-api-key"
              className={styles.input}
              type={revealSecret ? "text" : "password"}
              value={apiKey}
              onChange={(event) => setApiKey(event.target.value)}
              autoComplete="off"
              spellCheck={false}
              aria-labelledby="provider-api-key-label"
              aria-invalid={errors.apiKey ? true : undefined}
              aria-describedby={errors.apiKey ? "provider-api-key-error" : "provider-api-key-help"}
            />
            <button
              type="button"
              className={styles.revealButton}
              aria-pressed={revealSecret}
              onClick={() => setRevealSecret((value) => !value)}
            >
              {revealSecret ? t("provider.hideSecret") : t("provider.showSecret")}
            </button>
          </div>
          {errors.apiKey ? (
            <p id="provider-api-key-error" className={styles.error}>
              {errors.apiKey}
            </p>
          ) : (
            <p id="provider-api-key-help" className={styles.help}>
              {t("provider.apiKeyHelp")}
            </p>
          )}
        </div>

        {isCustom ? (
          <div className={styles.field}>
            <label htmlFor="provider-headers" className={styles.label}>
              {t("provider.headersLabel")}
            </label>
            <textarea
              ref={headersRef}
              id="provider-headers"
              className={styles.textarea}
              value={headers}
              onChange={(event) => setHeaders(event.target.value)}
              rows={3}
              spellCheck={false}
              aria-invalid={errors.headers ? true : undefined}
              aria-describedby={errors.headers ? "provider-headers-error" : "provider-headers-help"}
            />
            {errors.headers ? (
              <p id="provider-headers-error" className={styles.error}>
                {errors.headers}
              </p>
            ) : (
              <p id="provider-headers-help" className={styles.help}>
                {t("provider.headersHelp")}
              </p>
            )}
          </div>
        ) : null}

        <div className={styles.notice} data-kind="billing">
          <p className={styles.noticeTitle}>{t("provider.billingLabel")}</p>
          <p className={styles.noticeBody}>{t("provider.billingNote")}</p>
        </div>

        <div className={styles.notice} data-kind="handling">
          <p className={styles.noticeTitle}>{t("provider.dataHandlingTitle")}</p>
          <p className={styles.noticeBody}>{t("provider.dataHandlingBody")}</p>
        </div>

        <div className={styles.actionRow}>
          <button
            type="button"
            className={styles.secondaryButton}
            disabled={pending}
            onClick={() => void runCommand("provider-connection-test")}
          >
            {t("provider.test")}
          </button>
          <button
            type="button"
            className={styles.primaryButton}
            disabled={pending}
            onClick={() => void runCommand("provider-connection-save")}
          >
            {t("provider.save")}
          </button>
        </div>
        <p className={styles.help}>{t("provider.testHelp")} {t("provider.saveHelp")}</p>

        {submitAttempted && errorCount > 1 ? (
          <p className={styles.error} role="status">
            {t("provider.validationSummary")}
          </p>
        ) : null}

        <div className={styles.capability}>
          <p className={styles.capabilityRow}>
            <span className={styles.capabilityLabel}>{t("provider.capabilityTitle")}: </span>
            <span className={styles.capabilityValue}>{t("provider.capabilityNotVerified")}</span>
          </p>
          <p className={styles.help}>{t("provider.capabilityNote")}</p>
        </div>

        {outcome === "test-unavailable" ? (
          <RecoveryPanel
            title={t("provider.testUnavailableTitle")}
            body={t("provider.testUnavailableBody")}
            note={t("provider.testUnavailableNote")}
            actions={[
              { label: t("provider.retry"), onAction: () => void runCommand("provider-connection-test") },
              { label: t("provider.dismiss"), onAction: () => setOutcome(null) },
            ]}
          />
        ) : null}
        {outcome === "save-unavailable" ? (
          <RecoveryPanel
            title={t("provider.saveUnavailableTitle")}
            body={t("provider.saveUnavailableBody")}
            note={t("provider.saveUnavailableNote")}
            actions={[
              { label: t("provider.dismiss"), onAction: () => setOutcome(null) },
            ]}
          />
        ) : null}
      </div>
    );
  },
);
