/**
 * Shared deep-merge helper for merging the 16 per-locale namespace JSON files
 * into one unified translation tree (G-10).
 */

export function deepMergeResources<T extends Record<string, unknown>>(
  target: T,
  ...sources: Record<string, unknown>[]
): T {
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
        result[key] = deepMergeResources(
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
