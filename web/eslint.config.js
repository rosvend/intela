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

export default tseslint.config(
  { ignores: ["dist/**", "node_modules/**"] },
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
    },
  },
);
