import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef, useState } from "react";

import { validateBaseUrl } from "../../app/firstRun/baseUrl";
import {
  CUSTOM_API_PRESET_ID,
  CUSTOM_PROTOCOL_FORMATS,
  PROVIDER_PRESETS,
} from "../../app/firstRun/providerPresets";
import type { ProviderDraft } from "../../app/firstRun/firstRunState";
import { useFirstRun } from "../../app/firstRun/firstRunState";
import { useCoreConnection } from "../../app/coreConnectionState";
import type { CoreWireEnvelope } from "../../coreclient/httpLocal/wire";
import type {
  WireCapabilityDescriptor,
  WireCapabilitySet,
  WireConnectionTestReport,
  WireConnectionView,
} from "../../coreclient/httpLocal/wire";
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
 *   field set. The API-format selector lists the Core's coded custom
 *   connection protocols (C3-29/C3-30 — the ids are the coded adapter
 *   register's ids, hand-aligned here by the preset-ids rule; never
 *   claimed as a live Core enumeration). Its value is the wire provider
 *   id the Custom API save/test sends: the Core constructs each custom
 *   executor from the connection record's own posture (IMPL-18), and its
 *   typed refusal decides any mismatch (SEL-4's own rule).
 * - Base URL client-side validation per §18.4/MB §12.5 (see baseUrl.ts):
 *   format check only — nothing here implies an endpoint was verified.
 * - The API key lives ONLY in this component's local state while the
 *   form is mounted: it never enters the app draft, localStorage,
 *   sessionStorage, URLs, logs, or telemetry (X-6; C-07). Unmounting the
 *   form discards it. The explicit reveal exists for unsaved entry only.
 * - `Test connection` (M-25) and `Save securely` (M-24) are the Core's
 *   connection commands (IMPL-8). A command is sent ONLY while a held
 *   session authorizes this page's origin (the connected phase);
 *   otherwise the outcome is the truthful not-submitted state
 *   (S0-with-reason). The four-outcome discipline binds every command:
 *   the Core-confirmed result, the typed refusal verbatim (the stable
 *   code + the correlation id), and a lost response = S4 unknown — never
 *   proof of anything, never auto-retried. The save's S4 recovery offers
 *   the explicit SAME-KEY re-send (the Supplied create idempotency: the
 *   same key returns the original connection, never a second one); the
 *   test is a read-only safe re-issue. A saved key is NEVER re-displayed
 *   and the key field clears after every settled test/save outcome (the
 *   post-save clearing rule; the key is never restored from storage
 *   because it is never stored).
 * - Capability renders from what the Core actually reports (UI-13):
 *   supported/unsupported/unknown stay DISTINCT — unknown is `Not
 *   verified`; nothing is inferred from provider or model names. Two
 *   Core sources, one truth, explicit provenance: the per-connection
 *   test report (M-25 — the probe-bearing discovery surface) and, before
 *   any test, the F2 standing capability descriptor (IMPL-22 — the
 *   Core's OWN coded, permission-filtered discovery/status read; no
 *   probe; ADDITIVE to the test, never a replacement). The standing
 *   read's facts ride the SAME distinct-state mapping; the note names
 *   the source so no standing fact masquerades as a tested one. Where
 *   the Core supplies neither (unavailable, unwired), the panel renders
 *   its honest `Not verified` — never an invented capability.
 * - The billing label carries the UI-22 LOCKED rule: provider-billed
 *   BYOK, separate from any product maintenance plan. The data-handling
 *   notice renders ONLY as the Core supplies it (the notice below states
 *   the record's own notice rules; per-provider facts come from the Core
 *   as it supplies them — absent facts stay absent).
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

type TestOutcome =
  | { readonly kind: "ok"; readonly report: WireConnectionTestReport }
  | { readonly kind: "failed"; readonly report: WireConnectionTestReport }
  | { readonly kind: "unavailable" }
  | { readonly kind: "unknown" }
  | null;

/** The F2 standing capability descriptor read (IMPL-22): idle/loading /
 *  the Core's descriptor / the truthful unavailable (the panel falls back
 *  to its honest Not-verified state — never an invented capability). */
type DescriptorRead =
  | { readonly kind: "idle" }
  | { readonly kind: "loading" }
  | { readonly kind: "data"; readonly descriptor: WireCapabilityDescriptor }
  | { readonly kind: "unavailable" };

type SaveOutcome =
  | { readonly kind: "confirmed"; readonly connection: WireConnectionView }
  | { readonly kind: "refused"; readonly envelope: CoreWireEnvelope }
  | { readonly kind: "unavailable" }
  | { readonly kind: "unknown"; readonly pendingKey: string }
  | null;

interface FieldErrors {
  apiFormat: string | null;
  baseUrl: string | null;
  modelId: string | null;
  apiKey: string | null;
  headers: string | null;
}

const NO_ERRORS: FieldErrors = { apiFormat: null, baseUrl: null, modelId: null, apiKey: null, headers: null };

/** The idempotency key is minted at save time (the Run-submission
 *  pattern); a save without a unique key must never be sent. */
function newIdempotencyKey(): string {
  if (typeof crypto === "undefined" || typeof crypto.randomUUID !== "function") {
    throw new Error("idempotency key source unavailable");
  }
  return crypto.randomUUID();
}

export const ProviderSetupForm = forwardRef<ProviderSetupFormHandle>(
  function ProviderSetupForm(_props, ref) {
    const t = useT();
    const { providerDraft, setProviderDraft } = useFirstRun();
    const { transport, phase } = useCoreConnection();

    const [presetId, setPresetId] = useState(providerDraft.presetId);
    const [apiFormat, setApiFormat] = useState(providerDraft.apiFormat);
    const [baseUrl, setBaseUrl] = useState(providerDraft.baseUrl);
    const [modelId, setModelId] = useState(providerDraft.modelId);
    const [headers, setHeaders] = useState(providerDraft.headers);
    // The secret NEVER enters the app draft or any storage (X-6): it
    // lives only here, in component state, for the lifetime of the form.
    const [apiKey, setApiKey] = useState("");
    const [revealSecret, setRevealSecret] = useState(false);
    const [errors, setErrors] = useState<FieldErrors>(NO_ERRORS);
    const [submitAttempted, setSubmitAttempted] = useState(false);
    const [testOutcome, setTestOutcome] = useState<TestOutcome>(null);
    const [saveOutcome, setSaveOutcome] = useState<SaveOutcome>(null);
    const [pending, setPending] = useState<null | "test" | "save">(null);
    // The pending save's minted key: the S4 same-key re-send re-uses it
    // verbatim (the Supplied idempotency — the same key returns the
    // original connection); it is cleared when the attempt settles.
    const pendingSaveKey = useRef<string | null>(null);

    // The clean snapshot this form instance started from (J-8: unchanged
    // data is never re-prompted; discard returns exactly here).
    const cleanSnapshot = useRef<ProviderDraft>({ ...providerDraft });
    const apiFormatRef = useRef<HTMLSelectElement>(null);
    const baseUrlRef = useRef<HTMLInputElement>(null);
    const modelIdRef = useRef<HTMLInputElement>(null);
    const apiKeyRef = useRef<HTMLInputElement>(null);
    const headersRef = useRef<HTMLTextAreaElement>(null);

    const isCustom = presetId === CUSTOM_API_PRESET_ID;

    // The F2 standing capability descriptor read (IMPL-22, the adopted
    // S10 supply's P-S10-02 class): issued when the effective provider id
    // changes (the same id vocabulary the save/test commands send — the
    // coded adapter register's ids); read-sequence-guarded; runs NO
    // probe and mutates nothing. A refusal (an unwired id is the Core's
    // typed NOT_FOUND) or an unavailable Core leaves the panel on its
    // honest Not-verified state — never an invented capability, never a
    // name-inferred one (UI-13). The per-connection test (M-25) remains
    // the probe-bearing verification and takes precedence in the panel.
    const descriptorSeqRef = useRef(0);
    const [descriptorRead, setDescriptorRead] = useState<DescriptorRead>({ kind: "idle" });
    const readDescriptor = useCallback(
      (providerId: string) => {
        if (providerId === "") {
          setDescriptorRead({ kind: "idle" });
          return;
        }
        const seq = ++descriptorSeqRef.current;
        setDescriptorRead({ kind: "loading" });
        void transport.providerCapabilities(providerId).then((outcome) => {
          if (seq !== descriptorSeqRef.current) return;
          if (outcome.kind === "data") {
            setDescriptorRead({ kind: "data", descriptor: outcome.data.capability_descriptor });
          } else {
            setDescriptorRead({ kind: "unavailable" });
          }
        });
      },
      [transport],
    );

    // Persist the NON-SECRET draft so wizard back-navigation and re-entry
    // preserve entries (§8.13; C-06 draft rule). The key is excluded by
    // construction (it is not part of ProviderDraft).
    useEffect(() => {
      setProviderDraft({ presetId, apiFormat, baseUrl, modelId, headers });
    }, [presetId, apiFormat, baseUrl, modelId, headers, setProviderDraft]);

    useEffect(() => {
      readDescriptor(isCustom ? apiFormat : presetId);
    }, [readDescriptor, isCustom, apiFormat, presetId]);

    const parsedHeaders = useCallback((): { ok: boolean; headers: { name: string; value: string }[] } => {
      const out: { name: string; value: string }[] = [];
      const lines = headers.split("\n");
      for (const line of lines) {
        const trimmed = line.trim();
        if (trimmed.length === 0) continue;
        if (!/^[!#$%&'*+\-.^_`|~0-9A-Za-z]+:\s?.+$/.test(trimmed)) {
          return { ok: false, headers: out };
        }
        const idx = trimmed.indexOf(":");
        out.push({ name: trimmed.slice(0, idx).trim(), value: trimmed.slice(idx + 1).trim() });
      }
      return { ok: true, headers: out };
    }, [headers]);

    const computeErrors = useCallback((): FieldErrors => {
      const next: FieldErrors = { ...NO_ERRORS };
      if (isCustom && apiFormat.trim().length === 0) {
        next.apiFormat = t("provider.apiFormatRequired");
      }
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
    }, [isCustom, apiFormat, baseUrl, modelId, apiKey, parsedHeaders, t]);

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
      if (e.apiFormat) apiFormatRef.current?.focus();
      else if (e.baseUrl) baseUrlRef.current?.focus();
      else if (e.modelId) modelIdRef.current?.focus();
      else if (e.apiKey) apiKeyRef.current?.focus();
      else if (e.headers) headersRef.current?.focus();
    }, []);

    const clearSecret = useCallback(() => {
      // The post-save/post-test clearing rule: the typed secret is never
      // restored (it is never stored) — the next save opens a fresh
      // empty write-only field.
      setApiKey("");
      setRevealSecret(false);
    }, []);

    const runTest = useCallback((): void => {
      setSubmitAttempted(true);
      const e = computeErrors();
      setErrors(e);
      const invalid =
        e.apiFormat !== null || e.baseUrl !== null || e.modelId !== null || e.apiKey !== null || e.headers !== null;
      if (invalid) {
        focusFirstInvalid(e);
        return;
      }
      // No duplicate submit (§8.1); the truthful not-submitted state when
      // no authorized transport answers this origin.
      if (phase !== "connected") {
        setTestOutcome({ kind: "unavailable" });
        return;
      }
      const parsed = parsedHeaders();
      setPending("test");
      void transport
        .connectionTest({
          provider: isCustom ? apiFormat : presetId,
          ...(isCustom && baseUrl.trim() !== "" ? { base_url: baseUrl.trim() } : {}),
          model_id: modelId.trim(),
          ...(parsed.headers.length > 0 ? { custom_headers: parsed.headers } : {}),
          ...(apiKey.length > 0 ? { secret: apiKey } : {}),
        })
        .then((outcome) => {
          if (outcome.kind === "data") {
            setTestOutcome(outcome.data.report.ok
              ? { kind: "ok", report: outcome.data.report }
              : { kind: "failed", report: outcome.data.report });
          } else if (outcome.kind === "rejection") {
            // A typed refusal on a READ is surfaced verbatim (the stable
            // code + the correlation id); it is not an unknown.
            setTestOutcome({
              kind: "failed",
              report: {
                ok: false,
                provider: isCustom ? apiFormat : presetId,
                model_id: modelId.trim(),
                error_kind: outcome.envelope.code,
                error_detail: undefined,
                correlation_id: outcome.envelope.correlationId,
              },
            });
          } else if (outcome.kind === "unknown") {
            setTestOutcome({ kind: "unknown" });
          } else {
            setTestOutcome({ kind: "unavailable" });
          }
          // The typed secret is transient: cleared after the settled
          // outcome, never restored.
          clearSecret();
        })
        .finally(() => setPending(null));
    }, [computeErrors, focusFirstInvalid, phase, transport, presetId, isCustom, apiFormat, baseUrl, modelId, apiKey, parsedHeaders, clearSecret]);

    const runSave = useCallback((): void => {
      setSubmitAttempted(true);
      const e = computeErrors();
      setErrors(e);
      const invalid =
        e.apiFormat !== null || e.baseUrl !== null || e.modelId !== null || e.apiKey !== null || e.headers !== null;
      if (invalid) {
        focusFirstInvalid(e);
        return;
      }
      if (phase !== "connected") {
        setSaveOutcome({ kind: "unavailable" });
        return;
      }
      let key: string;
      try {
        key = newIdempotencyKey();
      } catch {
        setSaveOutcome({
          kind: "refused",
          envelope: {
            code: "VALIDATION_FAILED",
            retryable: false,
            correlationId: "",
            data: { reason: "idempotency key source unavailable" },
          },
        });
        return;
      }
      pendingSaveKey.current = key;
      const parsed = parsedHeaders();
      const body = {
        idempotency_key: key,
        provider: isCustom ? apiFormat : presetId,
        ...(isCustom && baseUrl.trim() !== "" ? { base_url: baseUrl.trim() } : {}),
        model_id: modelId.trim(),
        ...(parsed.headers.length > 0 ? { custom_headers: parsed.headers } : {}),
        ...(apiKey.length > 0 ? { secret: apiKey } : {}),
      };
      setPending("save");
      void transport
        .connectionSave(body)
        .then((outcome) => {
          if (outcome.kind === "data") {
            // The 200 IS the Core confirmation (the Supplied idempotency's
            // replayed flag rides honestly); the confirmed state renders
            // from what the Core returned and the connections list
            // re-reads its own authoritative state.
            setSaveOutcome({ kind: "confirmed", connection: outcome.data.connection });
            pendingSaveKey.current = null;
            clearSecret();
          } else if (outcome.kind === "rejection") {
            setSaveOutcome({ kind: "refused", envelope: outcome.envelope });
            pendingSaveKey.current = null;
            clearSecret();
          } else if (outcome.kind === "unknown") {
            // S4: the response was lost or uninterpretable — never proof
            // of a save or a failure. The minted key stays held so the
            // explicit same-key re-send resolves the Core's replay.
            setSaveOutcome({ kind: "unknown", pendingKey: key });
          } else {
            setSaveOutcome({ kind: "unavailable" });
            pendingSaveKey.current = null;
          }
        })
        .finally(() => setPending(null));
    }, [computeErrors, focusFirstInvalid, phase, transport, presetId, isCustom, apiFormat, baseUrl, modelId, apiKey, parsedHeaders, clearSecret]);

    /** The Supplied-idempotency recovery: an explicit, user-initiated
     *  re-issue of the SAME key and the EXACT same body — the Core's
     *  coded semantics return the original connection, never a second
     *  one. */
    const resendSameKey = useCallback((): void => {
      const key = pendingSaveKey.current;
      if (key === null || pending === "save") return;
      const parsed = parsedHeaders();
      setPending("save");
      void transport
        .connectionSave({
          idempotency_key: key,
          provider: isCustom ? apiFormat : presetId,
          ...(isCustom && baseUrl.trim() !== "" ? { base_url: baseUrl.trim() } : {}),
          model_id: modelId.trim(),
          ...(parsed.headers.length > 0 ? { custom_headers: parsed.headers } : {}),
          ...(apiKey.length > 0 ? { secret: apiKey } : {}),
        })
        .then((outcome) => {
          if (outcome.kind === "data") {
            setSaveOutcome({ kind: "confirmed", connection: outcome.data.connection });
            pendingSaveKey.current = null;
            clearSecret();
          } else if (outcome.kind === "rejection") {
            setSaveOutcome({ kind: "refused", envelope: outcome.envelope });
            pendingSaveKey.current = null;
            clearSecret();
          } else if (outcome.kind === "unknown") {
            setSaveOutcome({ kind: "unknown", pendingKey: key });
          } else {
            setSaveOutcome({ kind: "unavailable" });
            pendingSaveKey.current = null;
          }
        })
        .finally(() => setPending(null));
    }, [pending, transport, presetId, isCustom, apiFormat, baseUrl, modelId, apiKey, parsedHeaders, clearSecret]);

    useImperativeHandle(
      ref,
      () => ({
        isDirty: () => {
          const snapshot = cleanSnapshot.current;
          const fieldsChanged =
            presetId !== snapshot.presetId ||
            apiFormat !== snapshot.apiFormat ||
            baseUrl !== snapshot.baseUrl ||
            modelId !== snapshot.modelId ||
            headers !== snapshot.headers;
          return fieldsChanged || apiKey.length > 0;
        },
        discard: () => {
          const snapshot = cleanSnapshot.current;
          setPresetId(snapshot.presetId);
          setApiFormat(snapshot.apiFormat);
          setBaseUrl(snapshot.baseUrl);
          setModelId(snapshot.modelId);
          setHeaders(snapshot.headers);
          setApiKey("");
          setRevealSecret(false);
          setErrors(NO_ERRORS);
          setSubmitAttempted(false);
          setTestOutcome(null);
          setSaveOutcome(null);
          setProviderDraft({ ...snapshot });
        },
        attemptSave: () => {
          runSave();
        },
        isValid: isValidNow,
      }),
      [presetId, apiFormat, baseUrl, modelId, headers, apiKey, runSave, isValidNow, setProviderDraft],
    );

    const errorCount =
      (errors.apiFormat ? 1 : 0) +
      (errors.baseUrl ? 1 : 0) + (errors.modelId ? 1 : 0) + (errors.apiKey ? 1 : 0) + (errors.headers ? 1 : 0);

    // The capability panel's truth: rendered from the Core's test report
    // The capability panel's truth: rendered from what the Core actually
    // reports (UI-13) — two Core sources, one truth, explicit provenance.
    // The per-connection test report (M-25) takes precedence (the
    // probe-bearing verification); before any test, the F2 standing
    // capability descriptor (IMPL-22 — the Core's OWN coded discovery/
    // status read, no probe) supplies the same coded axes through the
    // SAME distinct-state mapping. The coded Feature axes render
    // DISTINCTLY — verified (the Core reports supported), unsupported
    // (its own distinct state), and Not verified (unknown) — never a
    // green collapse of distinct states (P3.3 §7; UI-13). Where the Core
    // supplies neither, the panel renders its honest Not verified.
    const testedCapability: WireCapabilitySet | undefined =
      testOutcome?.kind === "ok" ? (testOutcome.report.capabilities as WireCapabilitySet | undefined) : undefined;
    const descriptorCapability: WireCapabilitySet | undefined =
      descriptorRead.kind === "data" ? descriptorRead.descriptor.capabilities : undefined;
    const capabilitySource: "tested" | "descriptor" =
      testedCapability !== undefined ? "tested" : "descriptor";
    const capability: WireCapabilitySet | undefined = testedCapability ?? descriptorCapability;
    const capabilityRows: readonly { readonly label: string; readonly stateKey: "provider.capabilityVerified" | "provider.capabilityUnsupported" | "provider.capabilityNotVerified" }[] = capability
      ? [
          { label: t("provider.capabilityChat"), stateKey: capabilityStateKey(capability.chat) },
          { label: t("provider.capabilityStreaming"), stateKey: capabilityStateKey(capability.streaming) },
          { label: t("provider.capabilityTools"), stateKey: capabilityStateKey(capability.tools) },
          { label: t("provider.capabilityStructured"), stateKey: capabilityStateKey(capability.structured_output) },
          { label: t("provider.capabilityVision"), stateKey: capabilityStateKey(capability.vision) },
          { label: t("provider.capabilityEmbeddings"), stateKey: capabilityStateKey(capability.embeddings) },
        ]
      : [];

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
              <select
                ref={apiFormatRef}
                id="provider-api-format"
                className={styles.select}
                value={apiFormat}
                onChange={(event) => setApiFormat(event.target.value)}
                aria-invalid={errors.apiFormat ? true : undefined}
                aria-describedby={errors.apiFormat ? "provider-api-format-error" : "provider-api-format-help"}
              >
                <option value="">{t("provider.apiFormatPrompt")}</option>
                {CUSTOM_PROTOCOL_FORMATS.map((format) => (
                  <option key={format.id} value={format.id}>
                    {t(format.labelKey)}
                  </option>
                ))}
              </select>
              {errors.apiFormat ? (
                <p id="provider-api-format-error" className={styles.error}>
                  {errors.apiFormat}
                </p>
              ) : null}
              <p id="provider-api-format-help" className={styles.help}>
                {t("provider.apiFormatHelp")}
              </p>
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
          <div className={styles.credentialInputRow}>
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
            disabled={pending !== null}
            onClick={runTest}
          >
            {t("provider.test")}
          </button>
          <button
            type="button"
            className={styles.primaryButton}
            disabled={pending !== null}
            onClick={runSave}
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
          {capabilityRows.length === 0 ? (
            <>
              <p className={styles.capabilityRow}>
                <span className={styles.capabilityLabel}>{t("provider.capabilityTitle")}: </span>
                <span className={styles.capabilityValue}>{t("provider.capabilityNotVerified")}</span>
              </p>
              <p className={styles.help}>{t("provider.capabilityNote")}</p>
            </>
          ) : (
            <div role="list" aria-label={t("provider.capabilityTitle")}>
              {capabilityRows.map((row) => (
                <p key={row.label} role="listitem" className={styles.capabilityRow}>
                  <span className={styles.capabilityLabel}>{row.label}: </span>
                  <span className={styles.capabilityValue}>{t(row.stateKey)}</span>
                </p>
              ))}
              <p className={styles.help}>
                {capabilitySource === "tested"
                  ? t("provider.capabilityTestedNote")
                  : t("provider.capabilityDescriptorNote")}
              </p>
            </div>
          )}
        </div>

        {testOutcome?.kind === "unavailable" ? (
          <RecoveryPanel
            title={t("provider.testUnavailableTitle")}
            body={t("provider.testUnavailableBody")}
            note={t("provider.testUnavailableNote")}
            actions={[
              { label: t("provider.retry"), onAction: runTest },
              { label: t("provider.dismiss"), onAction: () => setTestOutcome(null) },
            ]}
          />
        ) : null}
        {testOutcome?.kind === "unknown" ? (
          <RecoveryPanel
            title={t("provider.testUnknownTitle")}
            body={t("provider.testUnknownBody")}
            note={t("provider.testUnknownNote")}
            actions={[
              { label: t("provider.retry"), onAction: runTest },
              { label: t("provider.dismiss"), onAction: () => setTestOutcome(null) },
            ]}
          />
        ) : null}
        {testOutcome?.kind === "ok" ? (
          <RecoveryPanel
            title={t("provider.testOkTitle")}
            body={t("provider.testOkBody", {
              endpoint: testOutcome.report.base_url ?? testOutcome.report.provider,
              model: testOutcome.report.model_id,
            })}
            note={t("provider.testOkNote", { correlationId: testOutcome.report.correlation_id })}
            actions={[{ label: t("provider.dismiss"), onAction: () => setTestOutcome(null) }]}
          />
        ) : null}
        {testOutcome?.kind === "failed" ? (
          <RecoveryPanel
            title={t("provider.testFailedTitle")}
            body={t("provider.testFailedBody", {
              errorKind: testOutcome.report.error_kind ?? "",
            })}
            note={t("provider.testFailedNote", { correlationId: testOutcome.report.correlation_id })}
            actions={[
              { label: t("provider.retry"), onAction: runTest },
              { label: t("provider.dismiss"), onAction: () => setTestOutcome(null) },
            ]}
          />
        ) : null}

        {saveOutcome?.kind === "confirmed" ? (
          <RecoveryPanel
            title={t("provider.saveConfirmedTitle")}
            body={t("provider.saveConfirmedBody", { connectionId: saveOutcome.connection.record.connection_id })}
            note={t("provider.saveConfirmedNote")}
            actions={[{ label: t("provider.dismiss"), onAction: () => setSaveOutcome(null) }]}
          />
        ) : null}
        {saveOutcome?.kind === "refused" ? (
          <RecoveryPanel
            title={t("provider.saveRefusedTitle")}
            body={t("provider.saveRefusedBody", { code: saveOutcome.envelope.code })}
            note={t("provider.saveRefusedNote", { correlationId: saveOutcome.envelope.correlationId })}
            actions={[{ label: t("provider.dismiss"), onAction: () => setSaveOutcome(null) }]}
          />
        ) : null}
        {saveOutcome?.kind === "unavailable" ? (
          <RecoveryPanel
            title={t("provider.saveUnavailableTitle")}
            body={t("provider.saveUnavailableBody")}
            note={t("provider.saveUnavailableNote")}
            actions={[{ label: t("provider.dismiss"), onAction: () => setSaveOutcome(null) }]}
          />
        ) : null}
        {saveOutcome?.kind === "unknown" ? (
          <RecoveryPanel
            title={t("provider.saveUnknownTitle")}
            body={t("provider.saveUnknownBody")}
            note={t("provider.saveUnknownNote")}
            actions={[
              { label: t("provider.saveResendSameKey"), onAction: resendSameKey },
              { label: t("provider.dismiss"), onAction: () => setSaveOutcome(null) },
            ]}
          />
        ) : null}
      </div>
    );
  },
);

/** The coded CapabilityState renderer (UI-13's distinct display): the
 *  Core's `supported` resolves the verified key, `unsupported` its own
 *  distinct key, and `unknown` the Not-verified key — never a green
 *  collapse of distinct states. */
function capabilityStateKey(
  state: unknown,
): "provider.capabilityVerified" | "provider.capabilityUnsupported" | "provider.capabilityNotVerified" {
  if (state === "supported") return "provider.capabilityVerified";
  if (state === "unsupported") return "provider.capabilityUnsupported";
  return "provider.capabilityNotVerified";
}
