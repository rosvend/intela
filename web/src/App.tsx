import { Route, Routes } from "react-router-dom";
import EnConstruccion from "./EnConstruccion";
import Estado from "./Estado";
import Inicio from "./Inicio";
import Layout from "./Layout";
import Login from "./Login";
import NoEncontrado from "./NoEncontrado";
import RutaProtegida from "./RutaProtegida";
import { RUTAS } from "./navegacion";

/**
 * Shell del tablero.
 *
 * La navegacion usa <Link>/<NavLink>, no <a href>. Con <a href> cada clic
 * recarga la pagina entera y pierde el estado; y sin `try_files` en nginx
 * -que hasta ahora tampoco estaba- devuelve 404 directamente.
 *
 * Las rutas de `RUTAS` (Sprint 3-5) entran aqui como placeholder: la pantalla
 * real llega con su propio PR, y esta tabla es lo unico que ese PR toca para
 * pasar de <EnConstruccion> al componente de verdad (issue #19: "so the
 * feature screens are pure additions").
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
              element={<EnConstruccion titulo={ruta.label} />}
            />
          ))}
        </Route>
      </Route>
      <Route path="*" element={<NoEncontrado />} />
    </Routes>
  );
}
