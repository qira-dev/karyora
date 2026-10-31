import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// P2.6 UI shell build. No environment-specific branches: one build for all
// targets (UI-03 one-app direction); the desktop transport adapter is a
// later contract (O-4 Open) and is not configured here.
export default defineConfig({
  plugins: [react()],
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
