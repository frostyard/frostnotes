/// <reference types="vitest/config" />
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// Inside nsl machines, host file edits raise no inotify events, so the
// watcher polls (docs/adr/0009-development-environment-and-gates.md).
const poll = process.env.NSL === "1";

// The web build (--mode web) is served by lodge at /app/notes/
// (docs/design/app.md#in-a-browser); the desktop app's from its root.
export default defineConfig(({ mode }) => ({
  base: mode === "web" ? "/app/notes/" : "/",
  plugins: [react(), tailwindcss()],
  clearScreen: false,
  envPrefix: ["VITE_", "TAURI_ENV_"],
  server: {
    // internal/devgw admits this origin only.
    host: "127.0.0.1",
    port: 5173,
    strictPort: true,
    watch: poll ? { usePolling: true, interval: 300 } : undefined,
  },
  build: { target: "es2022" },
  test: {
    environment: "happy-dom",
    // Tests read dates in UTC wherever they run (CI already does).
    env: { TZ: "UTC" },
    include: ["src/**/*.test.{ts,tsx}"],
    setupFiles: ["src/testing/setup.ts"],
  },
}));
