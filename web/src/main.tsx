import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import { ProveedorDeSesion } from "./sesion";
import "./styles.css";

createRoot(document.getElementById("root")!).render(
  <BrowserRouter>
    <ProveedorDeSesion>
      <App />
    </ProveedorDeSesion>
  </BrowserRouter>,
);
