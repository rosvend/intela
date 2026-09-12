import { useSesion } from "./sesion";
import TableroAdministrador from "./tablero/TableroAdministrador";
import TableroTitular from "./tablero/TableroTitular";
import { perfilDeTablero } from "./tablero/perfil";

/**
 * "/" no cambia de ruta segun el rol (M-5): el mockup no le da al titular un
 * item de nav propio, cambia el CONTENIDO de Inicio. Administrador y staff
 * ven el panel de control; el titular, su liquidacion. El panel de ingresos
 * (OE-6) — filtros por obra/fuente/periodo y ExplicarCifra — aterriza en
 * #ingresos dentro de TableroTitular, no en una ruta nueva.
 */
export default function Inicio() {
  const { usuario } = useSesion();
  if (!usuario) return null;

  if (perfilDeTablero(usuario.rol) === "titular") {
    return <TableroTitular usuario={usuario} />;
  }
  return <TableroAdministrador usuario={usuario} />;
}
