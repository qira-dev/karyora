/**
 * WCAG 2.2 AA Relative Luminance & Token Contrast Verification Engine (UIR-2.1, UI-25, G-03).
 *
 * Implements W3C WCAG 2.2 relative luminance and contrast ratio formulas to
 * verify all Dark and Light semantic foreground/background token pairs defined
 * in Blueprint §20.1 A.
 */

export interface TokenContrastPair {
  readonly theme: "dark" | "light";
  readonly foregroundToken: string;
  readonly foregroundHex: string;
  readonly backgroundToken: string;
  readonly backgroundHex: string;
  readonly minimumRequiredRatio: number;
}

export interface EvaluatedContrastPair extends TokenContrastPair {
  readonly calculatedRatio: number;
  readonly passesWcagAA: boolean;
  readonly passesWcagAAA: boolean;
}

export const CANONICAL_TOKEN_CONTRAST_PAIRS: readonly TokenContrastPair[] = [
  // Dark Theme (Blueprint §20.1 A)
  {
    theme: "dark",
    foregroundToken: "--text-primary",
    foregroundHex: "#f8fafc",
    backgroundToken: "--bg-canvas",
    backgroundHex: "#0b0e14",
    minimumRequiredRatio: 4.5,
  },
  {
    theme: "dark",
    foregroundToken: "--text-secondary",
    foregroundHex: "#cbd5e1",
    backgroundToken: "--bg-surface-1",
    backgroundHex: "#11161f",
    minimumRequiredRatio: 4.5,
  },
  {
    theme: "dark",
    foregroundToken: "--text-muted",
    foregroundHex: "#94a3b8",
    backgroundToken: "--bg-surface-1",
    backgroundHex: "#11161f",
    minimumRequiredRatio: 4.5,
  },
  {
    theme: "dark",
    foregroundToken: "--accent-text",
    foregroundHex: "#818cf8",
    backgroundToken: "--bg-surface-1",
    backgroundHex: "#11161f",
    minimumRequiredRatio: 4.5,
  },
  {
    theme: "dark",
    foregroundToken: "--success-text",
    foregroundHex: "#4ade80",
    backgroundToken: "--bg-surface-1",
    backgroundHex: "#11161f",
    minimumRequiredRatio: 4.5,
  },
  {
    theme: "dark",
    foregroundToken: "--warning-text",
    foregroundHex: "#fbbf24",
    backgroundToken: "--bg-surface-1",
    backgroundHex: "#11161f",
    minimumRequiredRatio: 4.5,
  },
  {
    theme: "dark",
    foregroundToken: "--danger-text",
    foregroundHex: "#f87171",
    backgroundToken: "--bg-surface-1",
    backgroundHex: "#11161f",
    minimumRequiredRatio: 4.5,
  },

  // Light Theme (Blueprint §20.1 A)
  {
    theme: "light",
    foregroundToken: "--text-primary",
    foregroundHex: "#0f172a",
    backgroundToken: "--bg-canvas",
    backgroundHex: "#f8fafc",
    minimumRequiredRatio: 4.5,
  },
  {
    theme: "light",
    foregroundToken: "--text-secondary",
    foregroundHex: "#334155",
    backgroundToken: "--bg-surface-1",
    backgroundHex: "#ffffff",
    minimumRequiredRatio: 4.5,
  },
  {
    theme: "light",
    foregroundToken: "--text-muted",
    foregroundHex: "#475569",
    backgroundToken: "--bg-surface-1",
    backgroundHex: "#ffffff",
    minimumRequiredRatio: 4.5,
  },
  {
    theme: "light",
    foregroundToken: "--accent-text",
    foregroundHex: "#3730a3",
    backgroundToken: "--bg-surface-1",
    backgroundHex: "#ffffff",
    minimumRequiredRatio: 4.5,
  },
  {
    theme: "light",
    foregroundToken: "--success-text",
    foregroundHex: "#15803d",
    backgroundToken: "--bg-surface-1",
    backgroundHex: "#ffffff",
    minimumRequiredRatio: 4.5,
  },
  {
    theme: "light",
    foregroundToken: "--warning-text",
    foregroundHex: "#b45309",
    backgroundToken: "--bg-surface-1",
    backgroundHex: "#ffffff",
    minimumRequiredRatio: 4.5,
  },
  {
    theme: "light",
    foregroundToken: "--danger-text",
    foregroundHex: "#b91c1c",
    backgroundToken: "--bg-surface-1",
    backgroundHex: "#ffffff",
    minimumRequiredRatio: 4.5,
  },
];

export function parseHexRgb(hex: string): readonly [number, number, number] {
  const cleaned = hex.trim().replace(/^#/, "");
  if (cleaned.length === 3) {
    const r = parseInt(cleaned[0] + cleaned[0], 16);
    const g = parseInt(cleaned[1] + cleaned[1], 16);
    const b = parseInt(cleaned[2] + cleaned[2], 16);
    return [r, g, b];
  }
  if (cleaned.length !== 6) {
    throw new Error(`Invalid 6-digit hex color: ${hex}`);
  }
  const r = parseInt(cleaned.slice(0, 2), 16);
  const g = parseInt(cleaned.slice(2, 4), 16);
  const b = parseInt(cleaned.slice(4, 6), 16);
  return [r, g, b];
}

export function computeRelativeLuminance(hex: string): number {
  const [r8, g8, b8] = parseHexRgb(hex);
  const transform = (channel8: number): number => {
    const sRgb = channel8 / 255;
    return sRgb <= 0.03928 ? sRgb / 12.92 : Math.pow((sRgb + 0.055) / 1.055, 2.4);
  };
  const r = transform(r8);
  const g = transform(g8);
  const b = transform(b8);
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

export function computeContrastRatio(foregroundHex: string, backgroundHex: string): number {
  const lumA = computeRelativeLuminance(foregroundHex);
  const lumB = computeRelativeLuminance(backgroundHex);
  const lighter = Math.max(lumA, lumB);
  const darker = Math.min(lumA, lumB);
  return Number(((lighter + 0.05) / (darker + 0.05)).toFixed(2));
}

export function evaluateCanonicalContrastPairs(): readonly EvaluatedContrastPair[] {
  return CANONICAL_TOKEN_CONTRAST_PAIRS.map((pair) => {
    const calculatedRatio = computeContrastRatio(
      pair.foregroundHex,
      pair.backgroundHex,
    );
    return {
      ...pair,
      calculatedRatio,
      passesWcagAA: calculatedRatio >= pair.minimumRequiredRatio,
      passesWcagAAA: calculatedRatio >= 7.0,
    };
  });
}
