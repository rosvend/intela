import { Link } from "react-router-dom";
import { puedeVer } from "../navegacion";
import { Usuario } from "../sesion";
import { Tarjeta } from "./Tarjeta";
import { formatearEntero } from "./formato";
import { useDashboard } from "./useDashboard";

export default function TableroAdministrador({
  usuario,
}: {
  usuario: Usuario;
}) {
  const tablero = useDashboard(usuario.rol);

  return (
    <section className="tablero">
      <header className="tablero-cabecera">
        <div>
          <h1>Panel de control</h1>
          <p className="muted">
            Reconocimiento de obras y distribución de ingresos para REDES SGC.
          </p>
        </div>
        {puedeVer(usuario.rol, "/ingesta") && (
          <Link className="boton-primario" to="/ingesta">
            Nueva ingesta
          </Link>
        )}
      </header>

      <div className="tablero-kpis">
        <Tarjeta
          titulo="Cargas pendientes"
          descripcion="Reportes de uso en cola de ingesta"
          to={puedeVer(usuario.rol, "/ingesta") ? "/ingesta" : undefined}
          etiquetaEnlace="Ir a ingesta"
          recurso={tablero.cargasPendientes}
        >
          {(datos) => (
            <>
              <p className="tarjeta-valor">{formatearEntero(datos.total)}</p>
              <p className="muted">Por procesar</p>
            </>
          )}
        </Tarjeta>

        <Tarjeta
          titulo="Obras en reserva"
          descripcion="Declaración incompleta: se retiene el total (RD 13.1.3)"
          to={puedeVer(usuario.rol, "/catalogo") ? "/catalogo" : undefined}
          etiquetaEnlace="Ir al catálogo"
          recurso={tablero.obrasEnReserva}
        >
          {(datos) => (
            <>
              <p className="tarjeta-valor">{formatearEntero(datos.total)}</p>
              <p className="muted">Sin split al 100%</p>
            </>
          )}
        </Tarjeta>

        <Tarjeta
          titulo="ONI"
          descripcion="Obras no identificadas (RD 13.8)"
          to={puedeVer(usuario.rol, "/anomalias") ? "/anomalias" : undefined}
          etiquetaEnlace="Ir a anomalías"
          recurso={tablero.oni}
        >
          {(datos) => (
            <>
              <p className="tarjeta-valor">{formatearEntero(datos.total)}</p>
              <p className="muted">Pendientes de identificación</p>
            </>
          )}
        </Tarjeta>

        <Tarjeta
          titulo="Última corrida"
          descripcion="Estado del último proceso de reparto"
          to={
            puedeVer(usuario.rol, "/distribucion") ? "/distribucion" : undefined
          }
          etiquetaEnlace="Ir a distribución"
          recurso={tablero.ultimaCorrida}
        >
          {(datos) => (
            <>
              <p className="tarjeta-valor">{datos.etapa}</p>
              <p className="muted">
                {datos.periodo} · {datos.estado}
              </p>
            </>
          )}
        </Tarjeta>
      </div>
    </section>
  );
}
