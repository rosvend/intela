import type { ReactElement } from "react";
import { Route, Routes } from "react-router-dom";
import EnConstruccion from "./EnConstruccion";
import Estado from "./Estado";
import Inicio from "./Inicio";
import Layout from "./Layout";
import Login from "./Login";
import NoEncontrado from "./NoEncontrado";
import RutaProtegida from "./RutaProtegida";
import Auditoria from "./auditoria/Auditoria";
import HistoriaObra from "./auditoria/HistoriaObra";
import Catalogo from "./catalogo/Catalogo";
import DetalleObra from "./catalogo/DetalleObra";
import EditorReparto from "./catalogo/EditorReparto";
import HistorialVersiones from "./catalogo/HistorialVersiones";
import Ingesta from "./ingesta/Ingesta";
import { RUTAS } from "./navegacion";
import PanelCorridas from "./reparto/PanelCorridas";
import TableroAnomalias from "./reparto/TableroAnomalias";

/**
 * Las pantallas reales de los modulos de `RUTAS`, por ruta. Un modulo que no
 * esta aqui monta <EnConstruccion>. Esta tabla es lo unico que toca el PR de
 * cada pantalla para pasar del placeholder al componente de verdad (issue
 * #19: "so the feature screens are pure additions").
 */
const PANTALLAS: Partial<Record<string, ReactElement>> = {
  "/ingesta": <Ingesta />,
  "/catalogo": <Catalogo />,
  "/distribucion": <PanelCorridas />,
  "/anomalias": <TableroAnomalias />,
  "/auditoria": <Auditoria />,
};

/**
 * Las sub-rutas del detalle de obra, anidadas a mano y NO como entradas de
 * `RUTAS` (D-007).
 *
 * `/catalogo/:id` no es un modulo del mockup: es una vista de detalle. Meterla
 * en `RUTAS` la pondria en la barra lateral y obligaria a decidir su
 * visibilidad por rol en cada entrada; como ruta hija, el detalle conserva el
 * item de nav activo y hereda el guard de rol sin tocar el shell, porque
 * `Layout.tsx` resuelve el modulo por PREFIJO. El precio, aceptado: `App.tsx`
 * deja de ser un mapeo plano de `RUTAS`.
 *
 * Las tres vistas son de #30 y las tres tienen ya pantalla: el detalle (paso 6),
 * el historial (paso 7) y el editor de reparto (paso 8). Se declaran todas aqui
 * para que su direccion sea la que D-007 fijo, y cada paso solo cambio el
 * elemento -el ultimo, el editor, dejo de montar <EnConstruccion> cuando su
 * pantalla existio: un placeholder sobre una ruta que ya funciona dice lo
 * contrario de lo que pasa-.
 *
 * Quien enlaza a cada una: el detalle se abre desde la fila de su obra en el
 * catalogo, el historial desde el detalle -al final de su declaracion vigente-,
 * y el editor desde el detalle tambien, en el bloque de la declaracion. Ninguna
 * de las tres es alcanzable solo escribiendo su URL.
 */
const SUBRUTAS_DEL_DETALLE: readonly { path: string; element: ReactElement }[] =
  [
    { path: "/catalogo/:id", element: <DetalleObra /> },
    { path: "/catalogo/:id/historial", element: <HistorialVersiones /> },
    { path: "/catalogo/:id/declaracion", element: <EditorReparto /> },
  ];

/**
 * La historia de una obra, anidada a mano y NO como entrada de `RUTAS`
 * (D-007, igual que el detalle del catalogo): no es un modulo del mockup,
 * es la vista de detalle de la auditoria. Se abre desde la linea de tiempo
 * o desde el buscador de la pantalla.
 */
const SUBRUTAS_AUDITORIA: readonly { path: string; element: ReactElement }[] = [
  { path: "/auditoria/obra/:id", element: <HistoriaObra /> },
];

/**
 * Shell del tablero.
 *
 * La navegacion usa <Link>/<NavLink>, no <a href>. Con <a href> cada clic
 * recarga la pagina entera y pierde el estado; y sin `try_files` en nginx
 * -que hasta ahora tampoco estaba- devuelve 404 directamente.
 *
 * Las rutas de `RUTAS` (Sprint 3-5) salen de `PANTALLAS`, o de
 * <EnConstruccion> mientras su pantalla no exista. Las tres sub-rutas del
 * detalle van aparte, en `SUBRUTAS_DEL_DETALLE`, y la historia de una obra
 * en `SUBRUTAS_AUDITORIA`.
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
          {SUBRUTAS_DEL_DETALLE.map((subruta) => (
            <Route
              key={subruta.path}
              path={subruta.path}
              element={subruta.element}
            />
          ))}
          {SUBRUTAS_AUDITORIA.map((subruta) => (
            <Route
              key={subruta.path}
              path={subruta.path}
              element={subruta.element}
            />
          ))}
        </Route>
      </Route>
      <Route path="*" element={<NoEncontrado />} />
    </Routes>
  );
}
