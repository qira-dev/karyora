import type common from "./locales/en/common.json";
import type nav from "./locales/en/nav.json";
import type shell from "./locales/en/shell.json";
import type settings from "./locales/en/settings.json";
import type home from "./locales/en/home.json";

/**
 * The typed translation-key union (G-10's second, type-level parity
 * check; P1.9 §6.4). The English source resources are the key schema:
 * the five namespace files merge into one translation tree at load time
 * (src/i18n/index.ts) and this type flattens the same merged shape, so a
 * key that exists in no resource file fails BOTH this typecheck and the
 * locale-parity script.
 */

export type SourceTree = typeof common &
  typeof nav &
  typeof shell &
  typeof settings &
  typeof home;

/** Dot-joined LEAF key paths of a nested resource object (the keys `t()`
 *  resolves to strings; branch paths are not translatable keys). */
type FlattenKeys<T> = T extends object
  ? {
      [K in keyof T & string]: NonNullable<T[K]> extends object
        ? T[K] extends readonly unknown[]
          ? K
          : `${K}.${FlattenKeys<T[K]>}`
        : K;
    }[keyof T & string]
  : never;

export type TranslationKey = FlattenKeys<SourceTree>;
