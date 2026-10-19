import { defineConfig } from "vite";
import preact from "@preact/preset-vite";

// The app is served by gachi-server under /app/ (embedded via go:embed).
// In dev, API calls are proxied to a locally running server.
const api = process.env.GACHI_API ?? "http://localhost:8080";

export default defineConfig({
  base: "/app/",
  plugins: [preact()],
  build: {
    outDir: "../internal/webui/dist",
    emptyOutDir: true,
    target: "es2022",
  },
  server: {
    port: 5173,
    proxy: {
      "/status": api,
      "/ping": api,
      "/auth": api,
    },
  },
});
