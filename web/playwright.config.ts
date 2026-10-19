import { defineConfig } from "@playwright/test";

// Pruebas visuales (#224): lo que jsdom no puede ver porque no hace layout
// -donde parte una linea, cuanto mide una columna-. Corren en Chromium contra
// el servidor de Vite, con la API simulada en cada prueba (`page.route`): no
// hace falta backend. Fuera de `npm test` a proposito: necesitan un navegador
// instalado (`npx playwright install chromium`).
const PUERTO = 4179;

// Sin @types/node a proposito: sus globales se colarian en los tipos de src/,
// que corre en el navegador. Del entorno solo se lee CI.
declare const process: { env: { CI?: string } };

export default defineConfig({
  testDir: "e2e",
  reporter: "list",
  // Un `test.only` olvidado no deja CI en verde corriendo una sola prueba.
  forbidOnly: !!process.env.CI,
  use: {
    baseURL: `http://127.0.0.1:${PUERTO}`,
    browserName: "chromium",
    // Cifra cuenta hasta su valor; quieta, lo pintado es el importe final.
    reducedMotion: "reduce",
    // Una prueba de layout que falla sin traza no se puede diagnosticar: CI
    // sube `test-results/` cuando algo falla.
    trace: "retain-on-failure",
  },
  webServer: {
    command: `npx vite --host 127.0.0.1 --port ${PUERTO} --strictPort`,
    url: `http://127.0.0.1:${PUERTO}`,
    reuseExistingServer: false,
  },
});
