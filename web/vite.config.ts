import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        // Igual que nginx: /api/afiliaciones llega a la API como /afiliaciones.
        rewrite: (path) => path.replace(/^\/api/, ""),
      },
      "/health": "http://localhost:8080",
    },
  },
  test: {
    environment: "jsdom",
  },
});
