import { Usuario } from "../sesion";
import { Tarjeta } from "./Tarjeta";
import { formatearEntero } from "./formato";
import { useDashboard } from "./useDashboard";

/**
 * Resumen del titular. El panel con filtros por obra/fuente/periodo y
 * "explicar esta cifra" es #42: aterriza en la seccion #ingresos de abajo,
 * no en una ruta nueva (M-5).
 */
export default function TableroTitular({ usuario }: { usuario: Usuario }) {
  const tablero = useDashboard(usuario.rol);

  return (
    <section className="tablero">
      <header className="tablero-cabecera">
        <div>
          <h1>Mi liquidación</h1>
          <p className="muted">{usuario.nombre}</p>
        </div>
      </header>

      <div className="tablero-kpis tablero-kpis-titular">
        <Tarjeta
          titulo="Mis obras"
          descripcion="Obras con participación declarada a tu nombre"
          to="#ingresos"
          etiquetaEnlace="Ver detalle"
          recurso={tablero.misObras}
        >
          {(datos) => (
            <>
              <p className="tarjeta-valor">
                {formatearEntero(datos.obras.length)}
              </p>
              <p className="muted">
                {datos.obras.length === 1
                  ? "Obra en tu repertorio"
                  : "Obras en tu repertorio"}
              </p>
            </>
          )}
        </Tarjeta>

        <Tarjeta
          titulo="Última liquidación"
          descripcion="Importe neto del último periodo de reparto"
          to="#ingresos"
          etiquetaEnlace="Ver detalle"
          recurso={tablero.ultimaLiquidacion}
        >
          {(datos) => (
            <>
              <p className="tarjeta-valor">{datos.neto}</p>
              <p className="muted">
                {datos.periodo}
                {datos.obras > 0
                  ? ` · ${formatearEntero(datos.obras)} obras`
                  : ""}
              </p>
            </>
          )}
        </Tarjeta>
      </div>

      <article className="tarjeta tarjeta-amplia" id="ingresos">
        <h2 className="tarjeta-etiqueta">Detalle por obra</h2>
        <p className="muted">
          El filtro por obra, fuente y periodo, los importes netos y «explicar
          esta cifra» llegan con el panel del titular. Mientras tanto no hay
          cifras que mostrar.
        </p>
      </article>
    </section>
  );
}
