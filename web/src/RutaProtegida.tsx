import { Navigate, Outlet, useLocation } from "react-router-dom";
import { token } from "./api";
import { useSesion } from "./sesion";

/**
 * Sin token: redirige de inmediato, no hay nada que esperar.
 *
 * Con token: mientras `ProveedorDeSesion` resuelve GET /auth/session, se
 * muestra un estado de carga -nunca una pantalla en blanco, que es justo lo
 * que pide el criterio de aceptacion del issue para el camino del 401. Si al
 * terminar de cargar no hay usuario (token vencido, revocado o invalido), se
 * redirige igual que si nunca hubiera habido token.
 */
export default function RutaProtegida() {
  const { usuario, cargando } = useSesion();
  const location = useLocation();

  if (!token()) {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }
  if (cargando) {
    return <p className="cargando">Cargando…</p>;
  }
  if (!usuario) {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }
  return <Outlet />;
}
