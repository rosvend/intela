import { ReactElement } from "react";
import type { ReactElement } from "react";
import { Route, Routes } from "react-router-dom";
import EnConstruccion from "./EnConstruccion";
import Estado from "./Estado";
import Inicio from "./Inicio";
import Layout from "./Layout";
import Login from "./Login";
import NoEncontrado from "./NoEncontrado";
import RutaProtegida from "./RutaProtegida";
import Ingesta from "./ingesta/Ingesta";
import { RUTAS } from "./navegacion";
import PanelCorridas from "./reparto/PanelCorridas";
import TableroAnomalias from "./reparto/TableroAnomalias";

/**
 * Pantallas reales que sustituyen el placeholder de Sprint 3-5. Quien
 * aterrice un modulo nuevo solo toca esta tabla (issue #19).
 */
const PANTALLAS: Record<string, ReactElement> = {
  "/distribucion": <PanelCorridas />,
  "/anomalias": <TableroAnomalias />,
};

/**
 * Las pantallas reales de los modulos de `RUTAS`, por ruta. Un modulo que no
 * esta aqui monta <EnConstruccion>. Esta tabla es lo unico que toca el PR de
 * cada pantalla para pasar del placeholder al componente de verdad (issue
 * #19: "so the feature screens are pure additions").
 */
const PANTALLAS: Partial<Record<string, ReactElement>> = {
  "/ingesta": <Ingesta />,
};

/**
 * Shell del tablero.
 *
 * La navegacion usa <Link>/<NavLink>, no <a href>. Con <a href> cada clic
 * recarga la pagina entera y pierde el estado; y sin `try_files` en nginx
 * -que hasta ahora tampoco estaba- devuelve 404 directamente.
 *
 * Las rutas de `RUTAS` (Sprint 3-5) salen de `PANTALLAS`, o de
 * <EnConstruccion> mientras su pantalla no exista.
 */
export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route element={<RutaProtegida />}>
        <Route element={<Layout />}>
          <Route index element={<Inicio />} />
          <Route path="/estado" element={<Estado />} />
          {RUTAS.filter((ruta) => ruta.to !== "/").map((ruta) => (
            <Route
              key={ruta.to}
              path={ruta.to}
              element={
                PANTALLAS[ruta.to] ?? <EnConstruccion titulo={ruta.label} />
              }
            />
          ))}
          <Route path="/distribucion/:id" element={<PanelCorridas />} />
        </Route>
      </Route>
      <Route path="*" element={<NoEncontrado />} />
    </Routes>
  );
}
