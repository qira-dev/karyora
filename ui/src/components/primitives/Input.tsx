/**
 * 9-State Input, Write-Only Secret Input (D-37), and Byte-Counted Textarea Primitives
 * (UIR-2.3, Blueprint §6.1, §8.2, §13.1 (3), §13.2 (8), G-05, G-11).
 */

import {
  useId,
  useState,
  type InputHTMLAttributes,
  type ReactElement,
  type TextareaHTMLAttributes,
} from "react";

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  readonly label?: string;
  readonly hintText?: string;
  readonly errorText?: string;
  readonly invalid?: boolean;
  /**
   * When true, renders a write-only credential field (D-37):
   * - Reveal toggle is ONLY enabled while the user is typing unsaved text.
   * - Already-saved secrets in the OS keyring are shown only as a masked placeholder
   *   and can never be revealed or echoed back from the Core (G-11).
   */
  readonly writeOnlySecret?: boolean;
  readonly hasStoredKeyringSecret?: boolean;
}

export function Input({
  label,
  hintText,
  errorText,
  invalid = false,
  writeOnlySecret = false,
  hasStoredKeyringSecret = false,
  id,
  className = "",
  type = "text",
  value,
  disabled,
  readOnly,
  ...rest
}: InputProps): ReactElement {
  const generatedId = useId();
  const inputId = id ?? generatedId;
  const errorId = `${inputId}-error`;
  const hintId = `${inputId}-hint`;
  const [revealUnsaved, setRevealUnsaved] = useState(false);

  const isInvalid = invalid || Boolean(errorText);
  const hasUnsavedText =
    typeof value === "string" && value.length > 0;
  const canRevealSecret = writeOnlySecret && hasUnsavedText && !disabled && !readOnly;

  const effectiveType = writeOnlySecret
    ? canRevealSecret && revealUnsaved
      ? "text"
      : "password"
    : type;

  const describedBy = [
    hintText ? hintId : null,
    isInvalid && errorText ? errorId : null,
  ]
    .filter(Boolean)
    .join(" ") || undefined;

  return (
    <div className={`ky-field ${className}`.trim()}>
      {label && (
        <label
          htmlFor={inputId}
          style={{
            fontSize: "0.8125rem",
            fontWeight: 500,
            color: "var(--text-primary)",
          }}
        >
          {label}
        </label>
      )}
      <div style={{ position: "relative", display: "flex", alignItems: "center" }}>
        <input
          id={inputId}
          type={effectiveType}
          value={value}
          disabled={disabled}
          readOnly={readOnly}
          aria-invalid={isInvalid ? "true" : undefined}
          aria-describedby={describedBy}
          placeholder={
            writeOnlySecret && hasStoredKeyringSecret && !hasUnsavedText
              ? "•••••••••••••••• (Stored in OS keyring)"
              : rest.placeholder
          }
          className="ky-input"
          autoComplete={writeOnlySecret ? "off" : rest.autoComplete}
          spellCheck={writeOnlySecret ? false : rest.spellCheck}
          {...rest}
        />
        {writeOnlySecret && (
          <button
            type="button"
            className="ky-btn ky-btn--ghost ky-btn--sm"
            disabled={!canRevealSecret}
            aria-label={
              revealUnsaved ? "Hide unsaved secret" : "Reveal unsaved secret"
            }
            title={
              canRevealSecret
                ? "Toggle visibility of unsaved text"
                : "Saved OS keyring secrets are write-only and cannot be revealed"
            }
            onClick={() => {
              if (canRevealSecret) {
                setRevealUnsaved((prev) => !prev);
              }
            }}
            style={{
              position: "absolute",
              insetInlineEnd: "4px",
            }}
          >
            {canRevealSecret && revealUnsaved ? "Hide" : "Show"}
          </button>
        )}
      </div>
      {hintText && (
        <span
          id={hintId}
          style={{ fontSize: "0.75rem", color: "var(--text-muted)" }}
        >
          {hintText}
        </span>
      )}
      {isInvalid && errorText && (
        <span
          id={errorId}
          role="alert"
          style={{ fontSize: "0.75rem", color: "var(--danger-text)" }}
        >
          {errorText}
        </span>
      )}
    </div>
  );
}

export interface TextareaProps
  extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  readonly label?: string;
  readonly hintText?: string;
  readonly errorText?: string;
  readonly invalid?: boolean;
  /**
   * Optional byte budget counter (e.g. `102400` for `0 KB / 100 KB` in
   * `Settings → Personalization` Screenshot #23).
   */
  readonly maxBytes?: number;
}

export function formatByteCounter(currentBytes: number, maxBytes: number): string {
  const currentKb = (currentBytes / 1024).toFixed(currentBytes === 0 ? 0 : 1);
  const maxKb = Math.round(maxBytes / 1024);
  return `${currentKb} KB / ${maxKb} KB`;
}

export function Textarea({
  label,
  hintText,
  errorText,
  invalid = false,
  maxBytes,
  id,
  className = "",
  value,
  ...rest
}: TextareaProps): ReactElement {
  const generatedId = useId();
  const textareaId = id ?? generatedId;
  const errorId = `${textareaId}-error`;
  const hintId = `${textareaId}-hint`;

  const textValue = typeof value === "string" ? value : "";
  const currentBytes =
    typeof TextEncoder !== "undefined"
      ? new TextEncoder().encode(textValue).length
      : textValue.length;
  const overBudget = typeof maxBytes === "number" && currentBytes > maxBytes;
  const isInvalid = invalid || overBudget || Boolean(errorText);

  return (
    <div className={`ky-field ${className}`.trim()}>
      {(label || typeof maxBytes === "number") && (
        <div
          style={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
          }}
        >
          {label && (
            <label
              htmlFor={textareaId}
              style={{
                fontSize: "0.8125rem",
                fontWeight: 500,
                color: "var(--text-primary)",
              }}
            >
              {label}
            </label>
          )}
          {typeof maxBytes === "number" && (
            <span
              data-testid="textarea-byte-counter"
              style={{
                fontSize: "0.75rem",
                fontVariantNumeric: "tabular-nums",
                color: overBudget ? "var(--danger-text)" : "var(--text-muted)",
              }}
            >
              {formatByteCounter(currentBytes, maxBytes)}
            </span>
          )}
        </div>
      )}
      <textarea
        id={textareaId}
        value={value}
        aria-invalid={isInvalid ? "true" : undefined}
        aria-describedby={
          [hintText ? hintId : null, isInvalid && errorText ? errorId : null]
            .filter(Boolean)
            .join(" ") || undefined
        }
        className="ky-textarea"
        {...rest}
      />
      {hintText && (
        <span
          id={hintId}
          style={{ fontSize: "0.75rem", color: "var(--text-muted)" }}
        >
          {hintText}
        </span>
      )}
      {isInvalid && errorText && (
        <span
          id={errorId}
          role="alert"
          style={{ fontSize: "0.75rem", color: "var(--danger-text)" }}
        >
          {errorText}
        </span>
      )}
    </div>
  );
}
