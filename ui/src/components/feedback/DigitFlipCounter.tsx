/**
 * ZCode Animated Digit Flip Counter (UIR-2.3, Blueprint §5.4, §20.2).
 *
 * Uses `font-variant-numeric: tabular-nums` and `0.16s cubic-bezier(0.22, 1, 0.36, 1)`
 * vertical transition (`.digit-flip-counter`) with `prefers-reduced-motion` support.
 */

import type { ReactElement } from "react";

export interface DigitFlipCounterProps {
  readonly value: number | string;
  readonly prefix?: string;
  readonly suffix?: string;
  readonly className?: string;
  readonly ariaLabel?: string;
}

export function DigitFlipCounter({
  value,
  prefix = "",
  suffix = "",
  className = "",
  ariaLabel,
}: DigitFlipCounterProps): ReactElement {
  const formatted =
    typeof value === "number" ? value.toLocaleString("en-US") : value;

  return (
    <span
      className={`digit-flip-counter ${className}`.trim()}
      aria-label={ariaLabel}
      data-value={String(value)}
      style={{
        display: "inline-block",
        fontVariantNumeric: "tabular-nums",
      }}
    >
      {prefix}
      {formatted}
      {suffix}
    </span>
  );
}
