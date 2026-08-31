import { Navigate, Outlet, useLocation } from "react-router-dom";
import Cargando from "./Cargando";
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
    // Este es el unico uso a pantalla completa de <Cargando>: aqui no existe
    // Layout todavia, asi que sin este envoltorio el spinner queda arriba de
    // la pagina en vez de centrado. Estado.tsx no lo usa: ahi ya vive dentro
    // del shell y forzar el viewport completo lo desalinearia.
    return (
      <div className="pantalla-carga">
        <Cargando />
      </div>
    );
  }
  if (!usuario) {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }
  return <Outlet />;
}
