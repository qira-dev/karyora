import { useTranslation } from "react-i18next";

/**
 * Typed translation access. Every user-facing string resolves through the
 * locale resources (G-10) — no hardcoded copy in components. The typed
 * keys come from i18next.d.ts (CustomTypeOptions over the en source).
 */
export function useT(): ReturnType<typeof useTranslation>["t"] {
  const { t } = useTranslation();
  return t;
}

/** The typed key union (strictKeyChecks) — use for dynamic keys held in
 *  data structures or props. Derived from the en source tree in keys.ts. */
export type { TranslationKey } from "./keys";

export { useTranslation };
