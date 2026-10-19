// Configuracion plana de ESLint (v9).
//
// Lo que importa aqui son las reglas de hooks: un useEffect sin dependencias
// correctas produce peticiones repetidas o datos rancios, y en un tablero que
// muestra cifras de liquidacion "rancio" significa ensenar un numero que ya
// no es el que hay.
import js from "@eslint/js";
import globals from "globals";
import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";

const MENSAJE_SONDEO =
  "Un sondeo va por useSondeo: con setInterval sostiene la sesion desatendida (ADR 0025, #223).";

export default tseslint.config(
  { ignores: ["dist/**", "node_modules/**", "src/contrato.d.ts"] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ["**/*.{ts,tsx}"],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
    },
    plugins: { "react-hooks": reactHooks },
    rules: {
      ...reactHooks.configs.recommended.rules,
      "@typescript-eslint/no-unused-vars": [
        "error",
        { argsIgnorePattern: "^_", varsIgnorePattern: "^_" },
      ],
      // El tablero muestra dinero: un `any` que se cuela aqui es una cifra
      // sin tipo que nadie comprueba.
      "@typescript-eslint/no-explicit-any": "error",
      eqeqeq: ["error", "always"],
      "no-console": ["warn", { allow: ["warn", "error"] }],
      // Vite compila para Safari 14, que no tiene lookbehind: esbuild lo
      // vuelve `new RegExp(...)` y ese constructor lanza SyntaxError al
      // cargar el bundle, que deja la app entera en blanco (#224).
      "no-restricted-syntax": [
        "error",
        {
          selector: "Literal[regex.pattern=/\\(\\?<[=!]/]",
          message:
            "Lookbehind (?<= / ?<!) rompe Safari < 16.4, el target del build. Reescribe el regex con lookahead.",
        },
      ],
    },
  },
  {
    // Cada peticion con el token corre la inactividad de la sesion en el
    // servidor: un sondeo con `setInterval` directo sostiene una pestana
    // desatendida hasta SESION_TTL. Todo sondeo va por `useSondeo`
    // (ADR 0025, #223). Las pruebas quedan fuera: no piden al servidor.
    files: ["src/**/*.{ts,tsx}"],
    ignores: ["src/useSondeo.ts", "src/**/*.test.{ts,tsx}"],
    rules: {
      "no-restricted-globals": [
        "error",
        { name: "setInterval", message: MENSAJE_SONDEO },
      ],
      "no-restricted-properties": [
        "error",
        { object: "window", property: "setInterval", message: MENSAJE_SONDEO },
        {
          object: "globalThis",
          property: "setInterval",
          message: MENSAJE_SONDEO,
        },
      ],
    },
  },
);
