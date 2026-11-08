import type common from "./locales/en/common.json";
import type nav from "./locales/en/nav.json";
import type shell from "./locales/en/shell.json";
import type settings from "./locales/en/settings.json";
import type home from "./locales/en/home.json";
import type firstrun from "./locales/en/firstrun.json";
import type jobs from "./locales/en/jobs.json";
import type run from "./locales/en/run.json";
import type chat from "./locales/en/chat.json";
import type notifications from "./locales/en/notifications.json";
import type activity from "./locales/en/activity.json";
import type knowledge from "./locales/en/knowledge.json";
import type memory from "./locales/en/memory.json";
import type context from "./locales/en/context.json";
import type workflow from "./locales/en/workflow.json";
import type plugin from "./locales/en/plugin.json";

/**
 * The typed translation-key union (G-10's second, type-level parity
 * check; P1.9 §6.4). The English source resources are the key schema:
 * the namespace files merge into one translation tree at load time
 * (src/i18n/index.ts) and this module mirrors that merge at the type
 * level, so a key that exists in no resource file fails BOTH this
 * typecheck and the locale-parity script.
 *
 * The merge mirrors the runtime deepMerge exactly: keys present in both
 * objects merge recursively; otherwise the later object's value wins.
 * The resource trees currently share only branch names (`groups`,
 * `shortcuts`) with disjoint leaves, so the merged type equals the
 * merged runtime tree; if two files ever claim the same LEAF path, the
 * parity check's merged-key rule and this type both keep working, and
 * the collision becomes a review item rather than a silent one.
 */

/** Runtime deepMerge at the type level (see src/i18n/index.ts). */
type Merge<A, B> = {
  [K in keyof A | keyof B]: K extends keyof B
    ? K extends keyof A
      ? A[K] extends object
        ? B[K] extends object
          ? Merge<A[K], B[K]>
          : B[K]
        : B[K]
      : B[K]
    : K extends keyof A
      ? A[K]
      : never;
};

export type SourceTree = Merge<
  Merge<
    Merge<
      Merge<Merge<Merge<typeof common, typeof nav>, typeof shell>, typeof settings>,
      typeof home
    >,
    typeof firstrun
  >,
  Merge<
    Merge<Merge<Merge<typeof jobs, typeof run>, typeof chat>, typeof notifications>,
    Merge<
      Merge<Merge<typeof activity, typeof knowledge>, Merge<typeof memory, typeof context>>,
      Merge<typeof workflow, typeof plugin>
    >
  >
>;

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
