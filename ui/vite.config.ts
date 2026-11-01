import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// P2.6 UI shell build. No environment-specific branches: one build for all
// targets (UI-03 one-app direction); the desktop transport adapter is a
// later contract (O-4 Open) and is not configured here.
//
// IMPL-2 (the browser CoreClient transport): in DEVELOPMENT only, the dev
// server can forward same-origin `/v1` requests to the Core's
// loopback-bound local HTTP transport (`internal/httplocal`). The target
// is operator-provided (`KARYORA_UI_LOCAL_CORE_TARGET`, e.g. the address
// the Core's transport was started on) — no port or host is invented
// here, and when the variable is unset nothing changes: `/v1` requests
// stay on the dev server and the UI renders its truthful-unavailable
// states. The proxy is the client-side serving path ONLY; it weakens no
// server-side predicate: the Core still enforces its Host allowlist (the
// rewritten Host header is the loopback target), its Origin/CSRF
// predicates (the browser's Origin header is forwarded unchanged, which
// is exactly what the C-1 exchange's origin binding consumes), its
// credential-surface rule, and its decision port on every request.
const localCoreTarget = process.env.KARYORA_UI_LOCAL_CORE_TARGET;

export default defineConfig({
  plugins: [react()],
  server: localCoreTarget
    ? {
        proxy: {
          "/v1": {
            target: localCoreTarget,
            changeOrigin: true,
          },
        },
      }
    : {},
  build: {
    // Deterministic, reproducible artifact naming for the PP-4 proof.
    rollupOptions: {
      output: {
        entryFileNames: "assets/[name].[hash].js",
        chunkFileNames: "assets/[name].[hash].js",
        assetFileNames: "assets/[name].[hash][extname]",
      },
    },
  },
  define: {
    // Static local metadata only (read-only About surface, C-21). No
    // secret or environment value is ever injected here.
    __APP_VERSION__: JSON.stringify(process.env.npm_package_version ?? "0.0.0"),
  },
});
