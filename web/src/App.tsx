import type { ReactElement } from "react";
import { Route, Routes } from "react-router-dom";
import EnConstruccion from "./EnConstruccion";
import Estado from "./Estado";
import Inicio from "./Inicio";
import Layout from "./Layout";
import Login from "./Login";
import NoEncontrado from "./NoEncontrado";
import RutaProtegida from "./RutaProtegida";
import Catalogo from "./catalogo/Catalogo";
import DetalleObra from "./catalogo/DetalleObra";
import Ingesta from "./ingesta/Ingesta";
import { RUTAS } from "./navegacion";

/**
 * Las pantallas reales de los modulos de `RUTAS`, por ruta. Un modulo que no
 * esta aqui monta <EnConstruccion>. Esta tabla es lo unico que toca el PR de
 * cada pantalla para pasar del placeholder al componente de verdad (issue
 * #19: "so the feature screens are pure additions").
 */
const PANTALLAS: Partial<Record<string, ReactElement>> = {
  "/ingesta": <Ingesta />,
  "/catalogo": <Catalogo />,
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
 * Las dos ultimas son del plan, no de este paso: el historial es el paso 7 y el
 * editor el 8. Se declaran ya para que su direccion sea la que D-007 fijo -un
 * paso posterior solo cambia el elemento-, y mientras tanto montan
 * <EnConstruccion>, que dice lo unico cierto: esa pantalla todavia no existe.
 *
 * No hay ningun enlace a ellas en esta pantalla: un enlace que no lleva a
 * ninguna parte es peor que su ausencia, que es la misma razon por la que el
 * catalogo no dibuja el alta de obras.
 */
const SUBRUTAS_DEL_DETALLE: readonly { path: string; element: ReactElement }[] =
  [
    { path: "/catalogo/:id", element: <DetalleObra /> },
    {
      path: "/catalogo/:id/historial",
      element: <EnConstruccion titulo="Historial de la declaración" />,
    },
    {
      path: "/catalogo/:id/declaracion",
      element: <EnConstruccion titulo="Declaración de la obra" />,
    },
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
 * detalle van aparte, en `SUBRUTAS_DEL_DETALLE`.
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
          {SUBRUTAS_DEL_DETALLE.map((subruta) => (
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
