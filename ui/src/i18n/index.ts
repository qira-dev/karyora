import i18next, { type i18n as I18n } from "i18next";
import { initReactI18next } from "react-i18next";

import common from "./locales/en/common.json";
import nav from "./locales/en/nav.json";
import shell from "./locales/en/shell.json";
import settings from "./locales/en/settings.json";
import home from "./locales/en/home.json";

/**
 * Localization (G-10; UI-18 LOCKED set; P1.9 §6.5 / P1.10 A-7 APPROVED
 * stack: i18next + react-i18next with per-locale JSON resources).
 *
 * - English (`en`) is the source and default locale.
 * - `fallbackLng: "en"` is configured EXPLICITLY: i18next's default
 *   fallback is `dev`, so this line is a mandatory setup item.
 * - `returnEmptyString: false` keeps the English fallback a correctness
 *   property for every locale file added later.
 * - The accepted 12-locale set is en, zh-CN, es, pt-BR, ja, ru, ko, de,
 *   fr, id, tr, vi. Only the English source resources exist today: no
 *   translation content exists (the human linguistic review is the
 *   owner's standing item), so no stub file is fabricated. Adding a
 *   locale later means adding `locales/<locale>/*.json` files and
 *   NOTHING else (no code or CSS branch — P2.4 §4.6).
 * - Namespaces are merged into ONE translation tree at load time and
 *   `nsSeparator` is disabled: keys are unique dotted paths across the
 *   merged tree (leaf collisions were checked; none exist), and the
 *   typed keys (i18next.d.ts) are generated from the same merged shape,
 *   so `strictKeyChecks` holds (G-10's second, type-level check).
 * - No user-facing copy is hardcoded in components: every string
 *   resolves through these resources.
 */

export const ACCEPTED_LOCALES = [
  "en",
  "zh-CN",
  "es",
  "pt-BR",
  "ja",
  "ru",
  "ko",
  "de",
  "fr",
  "id",
  "tr",
  "vi",
] as const;

export type AcceptedLocale = (typeof ACCEPTED_LOCALES)[number];

export const DEFAULT_LOCALE: AcceptedLocale = "en";

/** Deep-merge resource objects (later keys win only where absent). */
function deepMerge<T extends Record<string, unknown>>(target: T, ...sources: Record<string, unknown>[]): T {
  const result: Record<string, unknown> = { ...target };
  for (const source of sources) {
    for (const [key, value] of Object.entries(source)) {
      const existing = result[key];
      if (
        existing !== null &&
        value !== null &&
        typeof existing === "object" &&
        typeof value === "object" &&
        !Array.isArray(existing) &&
        !Array.isArray(value)
      ) {
        result[key] = deepMerge(
          existing as Record<string, unknown>,
          value as Record<string, unknown>,
        );
      } else {
        result[key] = value;
      }
    }
  }
  return result as T;
}

function englishResources(): Record<string, unknown> {
  return deepMerge({}, common, nav, shell, settings, home);
}

export function createI18n(): I18n {
  void i18next.use(initReactI18next).init({
    lng: DEFAULT_LOCALE,
    fallbackLng: "en",
    returnEmptyString: false,
    defaultNS: "translation",
    nsSeparator: false,
    resources: {
      en: { translation: englishResources() },
    },
    interpolation: {
      // React already escapes rendered strings; keep values raw here.
      escapeValue: false,
    },
    returnNull: false,
  });
  return i18next;
}
