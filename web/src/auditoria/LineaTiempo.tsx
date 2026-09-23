import { Fragment } from "react";
import { Link } from "react-router-dom";
import { formatearInstante } from "../tablero/formato";
import {
  ETIQUETA_FAMILIA,
  familiaDeHecho,
  leerPartes,
  type Asiento,
  type CambioDeParte,
  type ParteDeclarada,
} from "./tipos";

/**
 * La linea de tiempo de los asientos, en el orden en que llegan: la API los
 * sirve en orden de timeline (global) o de cadena (por obra), y aqui no se
 * reordena nada.
 *
 * Solo lectura a proposito: ni un boton de editar ni uno de borrar en toda la
 * vista. La inmutabilidad no la promete este componente -la impone la base
 * con el trigger `asientos_inmutables` (ADR 0006)-, pero la vista tampoco
 * ofrece lo que el sistema no deja hacer. Hay una prueba que lo afirma.
 */
export default function LineaTiempo({
  asientos,
  sinEnlaceA,
  cambios,
}: {
  asientos: readonly Asiento[];
  /** Obra que ya es el contexto (historia de una obra): no se enlaza a si misma. */
  sinEnlaceA?: string;
  /**
   * Diferencias entre versiones consecutivas de la declaracion, por id de
   * asiento. Las calcula quien tiene la historia completa (la vista por
   * obra); la linea solo las pinta.
   */
  cambios?: ReadonlyMap<string, CambioDeParte[]>;
}) {
  return (
    <ol className="linea-tiempo">
      {asientos.map((asiento) => (
        <li key={asiento.id} className="linea-tiempo-asiento">
          <span
            className={`linea-tiempo-punto linea-tiempo-punto-${familiaDeHecho(asiento.hecho)}`}
            aria-hidden="true"
          />
          <div className="linea-tiempo-contenido">
            <p className="linea-tiempo-cabecera">
              <time dateTime={asiento.cuando}>
                {formatearInstante(asiento.cuando)}
              </time>
              <span className="badge-estado">
                {ETIQUETA_FAMILIA[familiaDeHecho(asiento.hecho)]}
              </span>
            </p>
            <p className="linea-tiempo-hecho">{asiento.hecho}</p>
            <p className="linea-tiempo-meta">
              {asiento.actor === ""
                ? "Proceso automático"
                : `Por ${asiento.actor}`}
              {" · "}
              <Referencia asiento={asiento} sinEnlaceA={sinEnlaceA} />
            </p>
            <details className="linea-tiempo-evidencia">
              <summary>Evidencia de origen</summary>
              <DetalleAsiento
                asiento={asiento}
                cambios={cambios?.get(asiento.id)}
              />
            </details>
          </div>
        </li>
      ))}
    </ol>
  );
}

function Referencia({
  asiento,
  sinEnlaceA,
}: {
  asiento: Asiento;
  sinEnlaceA?: string;
}) {
  if (asiento.ref_tipo === "obra" && asiento.ref_id !== sinEnlaceA) {
    return (
      <Link to={`/auditoria/obra/${encodeURIComponent(asiento.ref_id)}`}>
        Historia de la obra {asiento.ref_id}
      </Link>
    );
  }
  return (
    <span>
      {asiento.ref_tipo} {asiento.ref_id}
    </span>
  );
}

/**
 * La evidencia de un asiento: el payload tal cual lo guardo el modulo que
 * asento, renderizado por familia.
 *
 * - `declaracion.guardada` trae version, estado y partes, y con `cambios` la
 *   diferencia contra la version anterior (el reparto viejo y el nuevo).
 * - `recaudo.registrado` trae la procedencia del dinero: periodo, circuito,
 *   bruto, convenio, tarifa y factura (pregunta 1 del ADR 0006).
 * - Una distribucion muestra fuente, reporte y regla primero, cuando el
 *   payload los trae: es lo que el criterio de aceptacion pide ver en cada
 *   fila de distribucion.
 * - Lo demas se lista como pares clave-valor, sin esconder nada: un asiento
 *   que no se entiende no se oculta.
 */
function DetalleAsiento({
  asiento,
  cambios,
}: {
  asiento: Asiento;
  cambios?: CambioDeParte[];
}) {
  const familia = familiaDeHecho(asiento.hecho);
  if (familia === "splits") {
    const partes = leerPartes(asiento.payload);
    if (partes !== null) {
      return (
        <DetalleSplits asiento={asiento} partes={partes} cambios={cambios} />
      );
    }
  }
  if (familia === "recaudo") {
    return <DetalleRecaudo payload={asiento.payload} />;
  }
  return <DetalleGenerico payload={asiento.payload} />;
}

function DetalleSplits({
  asiento,
  partes,
  cambios,
}: {
  asiento: Asiento;
  partes: ParteDeclarada[];
  cambios?: CambioDeParte[];
}) {
  const payload = asiento.payload as Record<string, unknown>;
  return (
    <div className="evidencia">
      <dl className="evidencia-datos">
        {payload["version"] !== undefined && (
          <Fragment>
            <dt>Versión</dt>
            <dd>{String(payload["version"])}</dd>
          </Fragment>
        )}
        {payload["estado"] !== undefined && (
          <Fragment>
            <dt>Estado</dt>
            <dd>{String(payload["estado"])}</dd>
          </Fragment>
        )}
      </dl>
      <table className="tabla-partes">
        <caption>Reparto declarado en esta versión</caption>
        <thead>
          <tr>
            <th>Titular</th>
            <th>IPI</th>
            <th className="tabla-partes-numero">Porcentaje</th>
          </tr>
        </thead>
        <tbody>
          {partes.map((parte) => (
            <tr key={parte.titularId}>
              <td className="detalle-identificador">{parte.titularId}</td>
              <td className="detalle-identificador">{parte.ipi}</td>
              <td className="tabla-partes-numero">{parte.porcentaje}%</td>
            </tr>
          ))}
        </tbody>
      </table>
      {cambios !== undefined && cambios.length > 0 && (
        <table className="tabla-partes">
          <caption>Contra la versión anterior</caption>
          <thead>
            <tr>
              <th>Titular</th>
              <th className="tabla-partes-numero">Antes</th>
              <th className="tabla-partes-numero">Ahora</th>
            </tr>
          </thead>
          <tbody>
            {cambios.map((cambio) => (
              <tr key={cambio.titularId}>
                <td className="detalle-identificador">{cambio.titularId}</td>
                <td className="tabla-partes-numero">
                  {cambio.antes === null ? "—" : `${cambio.antes}%`}
                </td>
                <td className="tabla-partes-numero">
                  {cambio.despues === null ? "—" : `${cambio.despues}%`}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

const CAMPOS_RECAUDO: readonly { clave: string; etiqueta: string }[] = [
  { clave: "periodo", etiqueta: "Periodo" },
  { clave: "circuito", etiqueta: "Circuito" },
  { clave: "bruto", etiqueta: "Bruto" },
  { clave: "convenio", etiqueta: "Convenio" },
  { clave: "tarifa", etiqueta: "Tarifa" },
  { clave: "factura", etiqueta: "Factura" },
  { clave: "usuario_id", etiqueta: "Usuario" },
];

function DetalleRecaudo({ payload }: { payload: unknown }) {
  const datos = payload as Record<string, unknown>;
  return (
    <dl className="evidencia-datos">
      {CAMPOS_RECAUDO.filter(({ clave }) => datos[clave] !== undefined).map(
        ({ clave, etiqueta }) => (
          <Fragment key={clave}>
            <dt>{etiqueta}</dt>
            <dd>{String(datos[clave])}</dd>
          </Fragment>
        ),
      )}
    </dl>
  );
}

/**
 * Claves que una distribucion tiene que mostrar primero, cuando existen: la
 * fuente, el reporte y la regla que produjo el monto. El resto del payload va
 * despues, sin recortes.
 */
const EVIDENCIA_PRIORITARIA = [
  "fuente",
  "reporte",
  "reporte_id",
  "regla",
  "regla_aplicada",
];

function escalar(valor: unknown): string | null {
  if (typeof valor === "string" || typeof valor === "number") {
    return String(valor);
  }
  return null;
}

function DetalleGenerico({ payload }: { payload: unknown }) {
  const datos = payload as Record<string, unknown>;
  const claves = Object.keys(datos);
  if (claves.length === 0) {
    return <p className="detalle-nota">Sin detalle adicional.</p>;
  }
  const ordenadas = [
    ...EVIDENCIA_PRIORITARIA.filter((clave) => clave in datos),
    ...claves.filter((clave) => !EVIDENCIA_PRIORITARIA.includes(clave)).sort(),
  ];
  return (
    <dl className="evidencia-datos">
      {ordenadas.map((clave) => (
        <Fragment key={clave}>
          <dt>{clave}</dt>
          <dd className="detalle-identificador">
            {escalar(datos[clave]) ?? JSON.stringify(datos[clave])}
          </dd>
        </Fragment>
      ))}
    </dl>
  );
}
