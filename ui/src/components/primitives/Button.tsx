/**
 * 9-State Accessible Button Primitive (UIR-2.3, Blueprint §6.1, §20.3 A, G-05, G-06).
 *
 * Supports the 4 canonical button hierarchy variants (`primary`, `secondary`, `ghost`, `danger`)
 * and all 9 control states (`default`, `hover`, `focus-visible`, `active`, `disabled` with
 * `disabledReason`, `loading` in-flight, `selected`, `invalid`, `readOnly`).
 */

import type { ButtonHTMLAttributes, ReactElement, ReactNode } from "react";

export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
export type ButtonSize = "sm" | "md" | "lg";

export interface ButtonProps
  extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, "disabled"> {
  readonly variant?: ButtonVariant;
  readonly size?: ButtonSize;
  readonly disabled?: boolean;
  readonly disabledReason?: string;
  readonly loading?: boolean;
  readonly loadingLabel?: string;
  readonly selected?: boolean;
  readonly invalid?: boolean;
  readonly readOnly?: boolean;
  readonly leadingIcon?: ReactNode;
  readonly trailingIcon?: ReactNode;
}

export function Button({
  variant = "secondary",
  size = "md",
  disabled = false,
  disabledReason,
  loading = false,
  loadingLabel,
  selected = false,
  invalid = false,
  readOnly = false,
  leadingIcon,
  trailingIcon,
  className = "",
  children,
  type = "button",
  title,
  onClick,
  ...rest
}: ButtonProps): ReactElement {
  const isActionBlocked = disabled || loading || readOnly;
  const resolvedTitle =
    disabled && disabledReason ? disabledReason : title;

  const classes = [
    "ky-btn",
    `ky-btn--${variant}`,
    `ky-btn--${size}`,
    selected ? "ky-btn--selected" : "",
    invalid ? "ky-btn--invalid" : "",
    className,
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <button
      type={type}
      className={classes}
      disabled={disabled || loading}
      aria-disabled={isActionBlocked ? "true" : undefined}
      aria-busy={loading ? "true" : undefined}
      aria-pressed={selected ? "true" : undefined}
      aria-invalid={invalid ? "true" : undefined}
      title={resolvedTitle}
      onClick={(e) => {
        if (isActionBlocked) {
          e.preventDefault();
          return;
        }
        onClick?.(e);
      }}
      {...rest}
    >
      {loading ? (
        <span
          className="ky-btn__spinner"
          aria-hidden="true"
          style={{
            display: "inline-block",
            width: "12px",
            height: "12px",
            borderRadius: "50%",
            border: "2px solid currentColor",
            borderTopColor: "transparent",
          }}
        />
      ) : (
        leadingIcon
      )}
      <span>{loading && loadingLabel ? loadingLabel : children}</span>
      {!loading && trailingIcon}
    </button>
  );
}
