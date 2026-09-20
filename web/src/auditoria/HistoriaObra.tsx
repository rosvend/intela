import { Link, useParams } from "react-router-dom";
import Cargando from "../Cargando";
import { useLista } from "../catalogo/useLista";
import LineaTiempo from "./LineaTiempo";
import {
  RUTAS_AUDITORIA,
  diferenciaDePartes,
  esAsiento,
  familiaDeHecho,
  leerPartes,
  type Asiento,
  type CambioDeParte,
} from "./tipos";

/**
 * Historia de una obra: cada cambio de sus metadatos, de sus declaraciones
 * (con el reparto anterior y el nuevo) y de sus distribuciones, en el orden
 * en que ocurrieron.
 *
 * Las diferencias entre versiones consecutivas de la declaracion se calculan
 * aqui, en el cliente, a partir de los asientos `declaracion.guardada`: el
 * asiento guarda la version que dejo, no contra cual cambio. Solo lectura,
 * igual que la linea global.
 */
export default function HistoriaObra() {
  const { id } = useParams<{ id: string }>();
  const obraID = id ?? "";
  const lista = useLista(RUTAS_AUDITORIA.historialDeObra(obraID), esAsiento);

  return (
    <section className="auditoria">
      <p className="detalle-volver">
        <Link to="/auditoria">← Auditoría</Link>
      </p>
      <header className="auditoria-cabecera">
        <div>
          <h1>Historia de la obra {obraID}</h1>
          <p className="muted">
            Metadatos, declaraciones y distribuciones en el orden en que
            ocurrieron, cada uno con su evidencia de origen.
          </p>
        </div>
      </header>

      <p className="auditoria-nota">
        Libro append-only: los asientos no se pueden modificar ni eliminar
        (trigger{" "}
        <span className="detalle-identificador">asientos_inmutables</span>, ADR
        0006).
      </p>

      {lista.estado === "cargando" && <Cargando texto="Cargando historia…" />}
      {lista.estado === "error" && (
        <p className="catalogo-error" role="alert">
          {lista.mensaje}
        </p>
      )}
      {lista.estado === "ilegible" && (
        <p className="catalogo-error" role="alert">
          La historia no llegó en un formato legible.
        </p>
      )}
      {lista.estado === "ok" &&
        (lista.elementos.length === 0 ? (
          <p className="muted">Esta obra no tiene asientos todavía.</p>
        ) : (
          <LineaTiempo
            asientos={lista.elementos}
            sinEnlaceA={obraID}
            cambios={cambiosEntreVersiones(lista.elementos)}
          />
        ))}
    </section>
  );
}

/**
 * Por cada asiento `declaracion.guardada`, la diferencia contra la version
 * anterior de la misma obra. Los asientos llegan en orden de cadena, asi que
 * "la anterior" es la ultima de splits vista al recorrer.
 */
export function cambiosEntreVersiones(
  asientos: readonly Asiento[],
): Map<string, CambioDeParte[]> {
  const cambios = new Map<string, CambioDeParte[]>();
  let anteriores: ReturnType<typeof leerPartes> = null;
  for (const asiento of asientos) {
    if (familiaDeHecho(asiento.hecho) !== "splits") continue;
    const actuales = leerPartes(asiento.payload);
    if (actuales === null) {
      anteriores = null;
      continue;
    }
    if (anteriores !== null) {
      cambios.set(asiento.id, diferenciaDePartes(anteriores, actuales));
    }
    anteriores = actuales;
  }
  return cambios;
}
