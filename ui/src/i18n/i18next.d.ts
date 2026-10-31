import type { SourceTree } from "./keys";

/**
 * i18next module augmentation for typed keys (P1.9 §6.4): the merged
 * English source tree is the `translation` namespace's resource type, so
 * `t(...)` accepts exactly the keys that exist in the source resources.
 */
declare module "i18next" {
  interface CustomTypeOptions {
    defaultNS: "translation";
    resources: {
      translation: SourceTree;
    };
    enableSelector: false;
  }
}
