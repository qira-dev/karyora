import { useTranslation } from "react-i18next";

import type { TranslationKey } from "./keys";

/**
 * Typed translation access. Every user-facing string resolves through the
 * locale resources (G-10) — no hardcoded copy in components. The typed
 * keys come from keys.ts (the merged en source tree).
 *
 * The return type is deliberately a SIMPLE typed function —
 * `(key: TranslationKey, options?) => string` — rather than i18next's
 * full TFunction generic. With the P2.7 key set the TFunction overload
 * machinery exceeded TypeScript's instantiation depth at some call sites
 * (TS2589) while adding no checking this project relies on: the G-10
 * discipline is the typed-key union (an unknown key fails `tsc`) and the
 * locale-parity script, both of which stay exactly as strict. All
 * resource values are strings (returnNull/returnEmptyString are false),
 * so the string return type is accurate.
 */
export type TranslateFn = (key: TranslationKey, options?: Record<string, unknown>) => string;

export function useT(): TranslateFn {
  const { t } = useTranslation();
  return t as unknown as TranslateFn;
}

/** The typed key union (strictKeyChecks) — use for dynamic keys held in
 *  data structures or props. Derived from the en source tree in keys.ts. */
export type { TranslationKey } from "./keys";

export { useTranslation };
