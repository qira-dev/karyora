import { useEffect, useRef, useState } from "react";

import { Dialog, useDialogLabelId } from "../../components/Dialog";
import { useT } from "../../i18n/useT";
import type { WireConnectionHeader, WireConnectionView } from "../../coreclient/httpLocal/wire";
import { CUSTOM_PROTOCOL_FORMATS, PROVIDER_PRESETS } from "../../app/firstRun/providerPresets";
import styles from "./ConnectionsPage.module.css";

/**
 * The connection-record rename and edit dialogs (IMPL-17 — the adopted
 * S8 supply's P-S8-01/P-S8-02 working candidates over the delivered
 * Core operations).
 *
 * The rename dialog (the M-27 register row's rename portion): the
 * display name ONLY — the confirmation states the binding rule before
 * the command (never the connection identity, never the Job identity,
 * never the execution history). The value is prefilled from the record's
 * current display name; the Core's law trims and bounds it.
 *
 * The edit dialog (the non-secret field update): EXACTLY the coded
 * create shape's non-secret fields (provider, base URL, model ID,
 * custom headers). The provider option set carries the locked preset
 * register (UI-11) AND the Core's coded custom connection protocols
 * (C3-29/C3-30 — IMPL-18's per-connection construction), so every
 * provider the Core can construct is selectable (SEL-4 end to end); the
 * record's own provider is always offered, never silently coerced. The
 * secret is NEVER re-displayed and NO secret field exists here (P3.3
 * §4.2/§4.4) — a credential change rides the Replace key / Re-enter key
 * flows, and the dialog states that. Client-side field checks are
 * FEEDBACK ONLY (P3.3 §8's enforcement rule): the Core's law decides,
 * and its typed refusal renders verbatim.
 *
 * Both commands ride the concurrent-edit guard: the caller passes the
 * `updated_at` marker it read the record under; a stale marker is the
 * Core's typed CONFLICT_STATE — the surface renders it verbatim and
 * performs NO overwrite and NO merge (the P1.6 §5 interim rule).
 * Escape closes and mutates nothing; focus returns to the trigger; the
 * pending window closes the confirm (no duplicate submit).
 */

/** One connection-header line's shape as parsed from the edit dialog's
 *  textarea ("Name: value" per line — the provider setup form's own
 *  parse grammar, feedback only). */
export interface ParsedConnectionHeaders {
  readonly ok: boolean;
  readonly headers: { name: string; value: string }[];
}

const HEADER_LINE = /^[!#$%&'*+\-.^_`|~0-9A-Za-z]+:\s?.+$/;

export function parseConnectionHeaders(raw: string): ParsedConnectionHeaders {
  const out: { name: string; value: string }[] = [];
  for (const line of raw.split("\n")) {
    const trimmed = line.trim();
    if (trimmed.length === 0) continue;
    if (!HEADER_LINE.test(trimmed)) {
      return { ok: false, headers: out };
    }
    const idx = trimmed.indexOf(":");
    out.push({ name: trimmed.slice(0, idx).trim(), value: trimmed.slice(idx + 1).trim() });
  }
  return { ok: true, headers: out };
}

export interface ConnectionRenameDialogProps {
  readonly open: boolean;
  readonly connection: WireConnectionView | null;
  readonly onConfirm: (connectionID: string, displayName: string, expectedUpdatedAt: string) => void;
  readonly onClose: () => void;
  readonly returnFocusRef?: React.RefObject<HTMLElement | null>;
  readonly pending: boolean;
}

export function ConnectionRenameDialog({ open, connection, onConfirm, onClose, returnFocusRef, pending }: ConnectionRenameDialogProps) {
  const t = useT();
  const labelId = useDialogLabelId();
  const [name, setName] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (open) {
      setName(connection?.record.display_name ?? "");
      inputRef.current?.focus();
    }
    // The prefill rides the OPENING connection; the connection object is
    // deliberately not a dependency (a re-render must not clobber input).
  }, [open]);

  const trimmed = name.trim();
  const canConfirm = trimmed.length > 0 && connection !== null && !pending;

  const confirm = () => {
    if (!canConfirm || connection === null) return;
    onConfirm(connection.record.connection_id, trimmed, connection.record.updated_at);
  };

  return (
    <Dialog labelId={labelId} title={t("connections.renameTitle")} open={open} onClose={onClose} returnFocusRef={returnFocusRef}>
      <div className={styles.dialogBody}>
        <p className={styles.dialogBodyText}>{t("connections.renameBody", { connectionId: connection?.record.connection_id ?? "" })}</p>
        <p className={styles.dialogEffects}>{t("connections.renameEffects")}</p>
        <div className={styles.dialogField}>
          <label htmlFor="connection-rename-input" className={styles.dialogLabel}>
            {t("connections.renameInputLabel")}
          </label>
          <input
            ref={inputRef}
            id="connection-rename-input"
            className={styles.dialogInput}
            type="text"
            value={name}
            maxLength={256}
            onChange={(event) => setName(event.target.value)}
            autoComplete="off"
            spellCheck={false}
          />
        </div>
        <div className={styles.dialogActions}>
          <button
            type="button"
            className={styles.primaryButton}
            disabled={!canConfirm}
            aria-disabled={!canConfirm ? "true" : undefined}
            onClick={confirm}
          >
            {t("connections.renameConfirm")}
          </button>
          <button type="button" className={styles.secondaryButton} onClick={onClose}>
            {t("connections.cancel")}
          </button>
        </div>
        <p className={styles.dialogHint}>{t("connections.dialogDurableNote")}</p>
      </div>
    </Dialog>
  );
}

export interface ConnectionEditDialogProps {
  readonly open: boolean;
  readonly connection: WireConnectionView | null;
  readonly onConfirm: (connectionID: string, fields: { provider: string; base_url: string; model_id: string; custom_headers: WireConnectionHeader[] }, expectedUpdatedAt: string) => void;
  readonly onClose: () => void;
  readonly returnFocusRef?: React.RefObject<HTMLElement | null>;
  readonly pending: boolean;
}

export function ConnectionEditDialog({ open, connection, onConfirm, onClose, returnFocusRef, pending }: ConnectionEditDialogProps) {
  const t = useT();
  const labelId = useDialogLabelId();
  const [provider, setProvider] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [modelId, setModelId] = useState("");
  const [headers, setHeaders] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (open && connection !== null) {
      setProvider(connection.record.provider);
      setBaseUrl(connection.record.base_url ?? "");
      setModelId(connection.record.model_id);
      setHeaders((connection.record.custom_headers ?? []).map((h) => `${h.name}: ${h.value}`).join("\n"));
      inputRef.current?.focus();
    }
    // The prefill rides the OPENING connection; the connection object is
    // deliberately not a dependency (a re-render must not clobber input).
  }, [open]);

  const parsed = parseConnectionHeaders(headers);
  const canConfirm = provider.trim().length > 0 && modelId.trim().length > 0 && parsed.ok && connection !== null && !pending;

  const confirm = () => {
    if (!canConfirm || connection === null) return;
    onConfirm(
      connection.record.connection_id,
      { provider: provider.trim(), base_url: baseUrl.trim(), model_id: modelId.trim(), custom_headers: parsed.headers },
      connection.record.updated_at,
    );
  };

  // The provider option set: the locked preset register (UI-11) plus the
  // Core's coded custom connection protocols (C3-29/C3-30 — the ids are
  // the coded adapter register's ids, hand-aligned by the preset-ids
  // rule; their labels are locale copy), and the record's own provider is
  // always offered (never silently coerced to an option value) — the
  // Core's wired-provider law decides the truth.
  const providerOptions = (() => {
    const options: readonly { readonly id: string; readonly label: string }[] = [
      ...PROVIDER_PRESETS.map((p) => ({ id: p.id, label: p.name })),
      ...CUSTOM_PROTOCOL_FORMATS.map((f) => ({ id: f.id, label: t(f.labelKey) })),
    ];
    if (!options.some((o) => o.id === provider)) {
      return [{ id: provider, label: provider }, ...options];
    }
    return options;
  })();

  return (
    <Dialog labelId={labelId} title={t("connections.editTitle")} open={open} onClose={onClose} returnFocusRef={returnFocusRef}>
      <div className={styles.dialogBody}>
        <p className={styles.dialogBodyText}>{t("connections.editBody", { connectionId: connection?.record.connection_id ?? "" })}</p>
        <p className={styles.dialogEffects}>{t("connections.editEffects")}</p>
        <div className={styles.dialogField}>
          <label htmlFor="connection-edit-provider" className={styles.dialogLabel}>
            {t("connections.editProviderLabel")}
          </label>
          <select
            id="connection-edit-provider"
            className={styles.dialogSelect}
            value={provider}
            onChange={(event) => setProvider(event.target.value)}
          >
            {providerOptions.map((p) => (
              <option key={p.id} value={p.id}>
                {p.label}
              </option>
            ))}
          </select>
        </div>
        <div className={styles.dialogField}>
          <label htmlFor="connection-edit-base-url" className={styles.dialogLabel}>
            {t("connections.editBaseUrlLabel")}
          </label>
          <input
            id="connection-edit-base-url"
            className={styles.dialogInput}
            type="text"
            value={baseUrl}
            maxLength={2048}
            onChange={(event) => setBaseUrl(event.target.value)}
            autoComplete="off"
            spellCheck={false}
          />
        </div>
        <div className={styles.dialogField}>
          <label htmlFor="connection-edit-model" className={styles.dialogLabel}>
            {t("connections.editModelLabel")}
          </label>
          <input
            id="connection-edit-model"
            className={styles.dialogInput}
            type="text"
            value={modelId}
            maxLength={256}
            onChange={(event) => setModelId(event.target.value)}
            autoComplete="off"
            spellCheck={false}
          />
        </div>
        <div className={styles.dialogField}>
          <label htmlFor="connection-edit-headers" className={styles.dialogLabel}>
            {t("connections.editHeadersLabel")}
          </label>
          <textarea
            id="connection-edit-headers"
            className={styles.dialogTextarea}
            value={headers}
            onChange={(event) => setHeaders(event.target.value)}
            spellCheck={false}
            aria-invalid={!parsed.ok ? true : undefined}
            aria-describedby={!parsed.ok ? "connection-edit-headers-error" : "connection-edit-headers-help"}
          />
          <p id="connection-edit-headers-help" className={styles.dialogHint}>
            {t("connections.editHeadersHelp")}
          </p>
          {!parsed.ok ? (
            <p id="connection-edit-headers-error" className={styles.dialogHint} role="alert">
              {t("connections.editHeadersInvalid")}
            </p>
          ) : null}
        </div>
        <div className={styles.dialogActions}>
          <button
            type="button"
            className={styles.primaryButton}
            disabled={!canConfirm}
            aria-disabled={!canConfirm ? "true" : undefined}
            onClick={confirm}
          >
            {t("connections.editConfirm")}
          </button>
          <button type="button" className={styles.secondaryButton} onClick={onClose}>
            {t("connections.cancel")}
          </button>
        </div>
        <p className={styles.dialogHint}>{t("connections.dialogDurableNote")}</p>
      </div>
    </Dialog>
  );
}
