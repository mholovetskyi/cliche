import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The build output goes straight into the Go package that embeds it, so a plain
// `go build` bakes the latest UI into the single binary (no Node needed to ship).
// In dev, `npm run dev` proxies the API to a running `cliche serve`.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../internal/web/static",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": {
        target: "http://127.0.0.1:7878",
        changeOrigin: true,
      },
    },
  },
});
