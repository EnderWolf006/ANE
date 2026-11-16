import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: { outDir: "../internal/server/web", emptyOutDir: true },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://127.0.0.1:18080",
      "/healthz": "http://127.0.0.1:18080",
    },
  },
});
