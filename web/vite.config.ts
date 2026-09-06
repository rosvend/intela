import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      // Sin `rewrite`, /api/ready llega a :8080/api/ready y da 404: Vite no
      // quita el prefijo por defecto. Produccion hace exactamente esta misma
      // sustitucion con la barra final de `proxy_pass http://api/;` en
      // deploy/nginx.conf; dev tiene que comportarse igual.
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, ""),
      },
    },
  },
  test: {
    environment: "jsdom",
  },
});
