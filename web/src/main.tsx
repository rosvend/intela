import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import { ProveedorDeSesion } from "./sesion";
// Autoalojada en vez de @import a fonts.googleapis.com: sin eso, cada carga
// mandaba la IP de quien usa el sistema a un tercero, dependia de salida a
// internet para verse bien, y el @import dentro del CSS es serial -el
// navegador tenia que bajar y parsear styles.css antes de descubrir que
// necesitaba la fuente-. Solo el subset "latin", que cubre los acentos y la
// ene del espanol, y solo los pesos que el CSS usa de verdad (400 base, 600,
// 700); nunca se renderiza en cursiva, asi que no se trae el italic.
import "@fontsource/plus-jakarta-sans/latin-400.css";
import "@fontsource/plus-jakarta-sans/latin-600.css";
import "@fontsource/plus-jakarta-sans/latin-700.css";
import "./styles.css";

createRoot(document.getElementById("root")!).render(
  <BrowserRouter>
    <ProveedorDeSesion>
      <App />
    </ProveedorDeSesion>
  </BrowserRouter>,
);
