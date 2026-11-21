import i18next, { type i18n as I18n } from "i18next";
import { initReactI18next } from "react-i18next";

import { deepMergeResources } from "./mergeBundle";
import common from "./locales/en/common.json";
import nav from "./locales/en/nav.json";
import shell from "./locales/en/shell.json";
import settings from "./locales/en/settings.json";
import home from "./locales/en/home.json";
import firstrun from "./locales/en/firstrun.json";
import jobs from "./locales/en/jobs.json";
import run from "./locales/en/run.json";
import chat from "./locales/en/chat.json";
import notifications from "./locales/en/notifications.json";
import activity from "./locales/en/activity.json";
import knowledge from "./locales/en/knowledge.json";
import memory from "./locales/en/memory.json";
import context from "./locales/en/context.json";
import workflow from "./locales/en/workflow.json";
import plugin from "./locales/en/plugin.json";

/**
 * 13-Locale Internationalization Runtime (UIR-9.1, G-10, G-13, UI-18).
 *
 * - English (`en`) is the authoritative source and eager fallback (`fallbackLng: "en"`).
 * - All 12 additional locked locales (`bn`, `zh-CN`, `es`, `pt-BR`, `ja`, `ru`, `ko`,
 *   `de`, `fr`, `id`, `tr`, `vi`) are lazy-loaded via dynamic per-locale Vite chunks so
 *   initial cold-start entry JS stays well below the G-13 `<= 350 KB` gzip budget.
 * - `returnEmptyString: false` and `returnNull: false` guarantee English fallback safety.
 */

export const ACCEPTED_LOCALES = [
  "en",
  "bn",
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

export interface LocaleDescriptor {
  readonly code: AcceptedLocale;
  readonly nativeLabel: string;
  readonly englishLabel: string;
  readonly dir: "ltr";
  readonly scriptFamily: "latin" | "bengali" | "cjk" | "cyrillic" | "hangul";
}

export const LOCALE_METADATA: readonly LocaleDescriptor[] = [
  { code: "en", nativeLabel: "English", englishLabel: "English", dir: "ltr", scriptFamily: "latin" },
  { code: "bn", nativeLabel: "বাংলা", englishLabel: "Bengali", dir: "ltr", scriptFamily: "bengali" },
  { code: "zh-CN", nativeLabel: "简体中文", englishLabel: "Simplified Chinese", dir: "ltr", scriptFamily: "cjk" },
  { code: "es", nativeLabel: "Español", englishLabel: "Spanish", dir: "ltr", scriptFamily: "latin" },
  { code: "pt-BR", nativeLabel: "Português (Brasil)", englishLabel: "Brazilian Portuguese", dir: "ltr", scriptFamily: "latin" },
  { code: "ja", nativeLabel: "日本語", englishLabel: "Japanese", dir: "ltr", scriptFamily: "cjk" },
  { code: "ru", nativeLabel: "Русский", englishLabel: "Russian", dir: "ltr", scriptFamily: "cyrillic" },
  { code: "ko", nativeLabel: "한국어", englishLabel: "Korean", dir: "ltr", scriptFamily: "hangul" },
  { code: "de", nativeLabel: "Deutsch", englishLabel: "German", dir: "ltr", scriptFamily: "latin" },
  { code: "fr", nativeLabel: "Français", englishLabel: "French", dir: "ltr", scriptFamily: "latin" },
  { code: "id", nativeLabel: "Bahasa Indonesia", englishLabel: "Indonesian", dir: "ltr", scriptFamily: "latin" },
  { code: "tr", nativeLabel: "Türkçe", englishLabel: "Turkish", dir: "ltr", scriptFamily: "latin" },
  { code: "vi", nativeLabel: "Tiếng Việt", englishLabel: "Vietnamese", dir: "ltr", scriptFamily: "latin" },
] as const;

function englishResources(): Record<string, unknown> {
  return deepMergeResources(
    {},
    common,
    nav,
    shell,
    settings,
    home,
    firstrun,
    jobs,
    run,
    chat,
    notifications,
    activity,
    knowledge,
    memory,
    context,
    workflow,
    plugin,
  );
}

/**
 * Dynamically imports the code-split translation bundle for a target locale.
 */
export async function loadLocaleBundle(
  locale: AcceptedLocale,
): Promise<Record<string, unknown>> {
  switch (locale) {
    case "en":
      return englishResources();
    case "bn":
      return (await import("./bundles/bn")).default();
    case "zh-CN":
      return (await import("./bundles/zh-CN")).default();
    case "es":
      return (await import("./bundles/es")).default();
    case "pt-BR":
      return (await import("./bundles/pt-BR")).default();
    case "ja":
      return (await import("./bundles/ja")).default();
    case "ru":
      return (await import("./bundles/ru")).default();
    case "ko":
      return (await import("./bundles/ko")).default();
    case "de":
      return (await import("./bundles/de")).default();
    case "fr":
      return (await import("./bundles/fr")).default();
    case "id":
      return (await import("./bundles/id")).default();
    case "tr":
      return (await import("./bundles/tr")).default();
    case "vi":
      return (await import("./bundles/vi")).default();
  }
}

/**
 * Loads the target locale bundle on demand, registers it into i18next,
 * activates it, and updates the document's `lang` and `dir` attributes.
 */
export async function activateLocale(
  instance: I18n,
  locale: AcceptedLocale,
): Promise<AcceptedLocale> {
  if (!instance.hasResourceBundle(locale, "translation")) {
    const bundle = await loadLocaleBundle(locale);
    instance.addResourceBundle(locale, "translation", bundle, true, true);
  }
  await instance.changeLanguage(locale);
  if (typeof document !== "undefined" && document.documentElement) {
    document.documentElement.lang = locale;
    document.documentElement.dir = "ltr";
  }
  return locale;
}

export function createI18n(): I18n {
  void i18next.use(initReactI18next).init({
    lng: DEFAULT_LOCALE,
    fallbackLng: "en",
    supportedLngs: [...ACCEPTED_LOCALES],
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

export {
  RFC9457_ACTION_KEY_MAP,
  RFC9457_ERROR_SUFFIXES,
  RFC9457_REQUIRED_I18N_KEYS,
  resolveLocalizedRecoveryCopy,
} from "./rfc9457ErrorMap";
export type {
  LocalizedRecoveryCopy,
  Rfc9457ErrorSuffix,
  TranslateFn,
} from "./rfc9457ErrorMap";
