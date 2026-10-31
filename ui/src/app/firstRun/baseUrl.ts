/**
 * Base URL validation for the Custom API provider fields (C-07; the
 * §18.4 / MB §12.5 network rule).
 *
 * Rules carried exactly from the frozen contract:
 * - Only http and https URLs are accepted (unsafe schemes blocked).
 * - TLS is required for NETWORK endpoints; plain http is allowed only
 *   for explicitly supported LOCAL endpoints, read here as loopback
 *   (localhost, 127.0.0.0/8, ::1) — the conservative reading of
 *   "explicitly supported local endpoints".
 * - URLs with embedded credentials (user:pass@host) are blocked
 *   outright (§18.4: embedded credentials blocked or warned — blocked
 *   here, the safer arm).
 *
 * The validation is CLIENT-SIDE FORMAT CHECKING ONLY. It is a usability
 * guard for the form; real endpoint policy enforcement is Core-side
 * (MB §12.5), and nothing here implies an endpoint was verified — a
 * syntactically valid URL is still untested until `Test connection`
 * runs through the Core.
 */

export type BaseUrlValidation =
  | { readonly valid: true; readonly isLocalEndpoint: boolean }
  | {
      readonly valid: false;
      readonly reason: "invalid" | "scheme" | "insecure" | "embedded-credentials";
    };

/** Loopback hosts where plain http is permitted. */
function isLoopbackHost(hostname: string): boolean {
  const host = hostname.toLowerCase().replace(/^\[|\]$/g, "");
  if (host === "localhost" || host === "::1" || host === "[::1]") return true;
  if (/^127\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(host)) return true;
  return false;
}

export function validateBaseUrl(raw: string): BaseUrlValidation {
  const value = raw.trim();
  if (value.length === 0) {
    return { valid: false, reason: "invalid" };
  }

  let parsed: URL;
  try {
    parsed = new URL(value);
  } catch {
    return { valid: false, reason: "invalid" };
  }

  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    return { valid: false, reason: "scheme" };
  }

  // Embedded credentials (user:pass@host) are blocked outright.
  if (parsed.username.length > 0 || parsed.password.length > 0) {
    return { valid: false, reason: "embedded-credentials" };
  }

  const local = isLoopbackHost(parsed.hostname);
  if (parsed.protocol === "http:" && !local) {
    return { valid: false, reason: "insecure" };
  }

  return { valid: true, isLocalEndpoint: local };
}
