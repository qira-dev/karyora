/**
 * Accessible Select, Switch Toggle (`role="switch"`), Status Badge & Diff Counter Pill (`+A -R`)
 * (UIR-2.3, Blueprint §6.1, §8.1, §8.6, G-05, G-06).
 */

import { useId, type ReactElement, type SelectHTMLAttributes } from "react";

export interface SelectOption {
  readonly value: string;
  readonly label: string;
  readonly disabled?: boolean;
}

export interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  readonly label?: string;
  readonly options: readonly SelectOption[];
  readonly hintText?: string;
  readonly errorText?: string;
  readonly invalid?: boolean;
}

export function Select({
  label,
  options,
  hintText,
  errorText,
  invalid = false,
  id,
  className = "",
  ...rest
}: SelectProps): ReactElement {
  const generatedId = useId();
  const selectId = id ?? generatedId;
  const isInvalid = invalid || Boolean(errorText);

  return (
    <div className={`ky-field ${className}`.trim()}>
      {label && (
        <label
          htmlFor={selectId}
          style={{
            fontSize: "0.8125rem",
            fontWeight: 500,
            color: "var(--text-primary)",
          }}
        >
          {label}
        </label>
      )}
      <select
        id={selectId}
        aria-invalid={isInvalid ? "true" : undefined}
        className="ky-select"
        {...rest}
      >
        {options.map((opt) => (
          <option key={opt.value} value={opt.value} disabled={opt.disabled}>
            {opt.label}
          </option>
        ))}
      </select>
      {hintText && (
        <span style={{ fontSize: "0.75rem", color: "var(--text-muted)" }}>
          {hintText}
        </span>
      )}
      {isInvalid && errorText && (
        <span
          role="alert"
          style={{ fontSize: "0.75rem", color: "var(--danger-text)" }}
        >
          {errorText}
        </span>
      )}
    </div>
  );
}

export interface ToggleProps {
  readonly checked: boolean;
  readonly onChange: (nextChecked: boolean) => void;
  readonly label: string;
  readonly description?: string;
  readonly disabled?: boolean;
  readonly disabledReason?: string;
  readonly id?: string;
}

export function Toggle({
  checked,
  onChange,
  label,
  description,
  disabled = false,
  disabledReason,
  id,
}: ToggleProps): ReactElement {
  const generatedId = useId();
  const toggleId = id ?? generatedId;

  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        gap: "var(--space-4)",
      }}
    >
      <div style={{ display: "flex", flexDirection: "column" }}>
        <label
          htmlFor={toggleId}
          style={{
            fontSize: "0.875rem",
            fontWeight: 500,
            color: "var(--text-primary)",
            cursor: disabled ? "not-allowed" : "pointer",
          }}
        >
          {label}
        </label>
        {description && (
          <span style={{ fontSize: "0.75rem", color: "var(--text-muted)" }}>
            {description}
          </span>
        )}
      </div>
      <button
        id={toggleId}
        type="button"
        role="switch"
        aria-checked={checked ? "true" : "false"}
        aria-label={label}
        disabled={disabled}
        title={disabled && disabledReason ? disabledReason : undefined}
        className="ky-toggle"
        onClick={() => {
          if (!disabled) {
            onChange(!checked);
          }
        }}
      >
        <span className="ky-toggle__thumb" aria-hidden="true" />
      </button>
    </div>
  );
}

export type BadgeTone = "neutral" | "accent" | "success" | "warning" | "danger" | "info";

export interface BadgeProps {
  readonly tone?: BadgeTone;
  readonly label: string;
  readonly dot?: boolean;
  readonly className?: string;
}

const TONE_STYLES: Readonly<Record<BadgeTone, { bg: string; fg: string }>> = {
  neutral: { bg: "var(--bg-surface-2)", fg: "var(--text-secondary)" },
  accent: { bg: "var(--bg-surface-3)", fg: "var(--accent-text)" },
  success: { bg: "var(--success-bg)", fg: "var(--success-text)" },
  warning: { bg: "var(--warning-bg)", fg: "var(--warning-text)" },
  danger: { bg: "var(--danger-bg)", fg: "var(--danger-text)" },
  info: { bg: "var(--info-bg)", fg: "var(--info-text)" },
};

export function Badge({
  tone = "neutral",
  label,
  dot = false,
  className = "",
}: BadgeProps): ReactElement {
  const colors = TONE_STYLES[tone];
  return (
    <span
      className={`ky-badge ${className}`.trim()}
      style={{ backgroundColor: colors.bg, color: colors.fg }}
    >
      {dot && (
        <span
          aria-hidden="true"
          style={{
            width: "6px",
            height: "6px",
            borderRadius: "50%",
            backgroundColor: colors.fg,
          }}
        />
      )}
      <span>{label}</span>
    </span>
  );
}

/**
 * ZCode `+A -R` Diff Summary Pill (Screenshots #75–#77, #84).
 */
export interface DiffCountPillProps {
  readonly additions: number;
  readonly deletions: number;
  readonly filesCount?: number;
}

export function DiffCountPill({
  additions,
  deletions,
  filesCount,
}: DiffCountPillProps): ReactElement {
  return (
    <span
      className="ky-diff-pill digit-flip-counter"
      aria-label={`${filesCount !== undefined ? `${filesCount} files changed, ` : ""}${additions} additions, ${deletions} deletions`}
    >
      {typeof filesCount === "number" && (
        <span style={{ color: "var(--text-secondary)" }}>{filesCount} files</span>
      )}
      {additions > 0 && (
        <span className="ky-diff-pill__added">+{additions}</span>
      )}
      {deletions > 0 && (
        <span className="ky-diff-pill__removed">-{deletions}</span>
      )}
      {additions === 0 && deletions === 0 && (
        <span style={{ color: "var(--text-muted)" }}>±0</span>
      )}
    </span>
  );
}
