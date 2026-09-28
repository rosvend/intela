import { esAsiento, RUTAS_AUDITORIA, type Asiento } from "../auditoria/tipos";
import Cargando from "../Cargando";
import { useLista } from "../catalogo/useLista";
import { iniciales } from "../iniciales";
import { formatearInstante } from "../tablero/formato";
import { HECHO_ASIGNADA, leerResolucionAsentada } from "./tipos";

/**
 * Historial de resoluciones manuales de UNA obra (plano de #39, paso 6): los
 * asientos `identificacion.asignada` de su bitacora -las veces que alguien
 * vinculo a mano un registro de uso con esta obra-, del mas reciente al mas
 * antiguo.
 *
 * Se monta al final de la ficha de obra (`DetalleObra.tsx`): la bitacora es
 * append-only (ADR 0006) y esta seccion es solo lectura, como
 * `auditoria/HistoriaObra.tsx`, de quien reutiliza `useLista` y `esAsiento` -
 * esta pantalla no inventa su propio guarda de tipo para el mismo `Asiento`.
 *
 * A diferencia de la historia completa de la obra (que pinta CUALQUIER
 * asiento con `ref_tipo: "obra"`), aqui solo importan los de identificacion:
 * el resto de la bitacora de la obra -declaraciones, distribuciones- ya tiene
 * su propia pantalla en `/auditoria/obra/{id}`.
 */
export default function HistorialResoluciones({ obraId }: { obraId: string }) {
  const lista = useLista(RUTAS_AUDITORIA.historialDeObra(obraId), esAsiento);

  return (
    <section className="detalle-declaracion historial-resoluciones">
      <h2>Historial de resoluciones manuales</h2>
      <p className="muted">
        Decisiones que vincularon registros de uso con esta obra.
      </p>

      {lista.estado === "cargando" && (
        <Cargando texto="Cargando el historial de resoluciones…" />
      )}
      {lista.estado === "error" && (
        <p className="catalogo-error" role="alert">
          No se pudo consultar el historial de resoluciones: {lista.mensaje}
        </p>
      )}
      {lista.estado === "ilegible" && (
        <p className="catalogo-error" role="alert">
          El historial de resoluciones no llegó en un formato legible.
        </p>
      )}
      {lista.estado === "ok" && (
        <CuerpoDelHistorial asientos={lista.elementos} />
      )}
    </section>
  );
}

/**
 * Solo los asientos de una asignacion manual, del mas reciente al mas
 * antiguo. Se ordena por `cuando` en vez de confiar en el orden que trae la
 * API -que en `auditoria/HistoriaObra.tsx` es de cadena, ascendente- porque
 * esta seccion promete explicitamente ese orden (plano seccion 5) y no debe
 * depender de un detalle del servidor que no forma parte del contrato.
 */
function resolucionesOrdenadas(asientos: readonly Asiento[]): Asiento[] {
  return asientos
    .filter((asiento) => asiento.hecho === HECHO_ASIGNADA)
    .sort((a, b) => b.cuando.localeCompare(a.cuando));
}

function CuerpoDelHistorial({ asientos }: { asientos: readonly Asiento[] }) {
  const resoluciones = resolucionesOrdenadas(asientos);

  if (resoluciones.length === 0) {
    return (
      <div className="bandeja-vacia">
        <p>Esta obra no tiene resoluciones manuales</p>
        <p className="muted">
          Las decisiones futuras aparecerán aquí con su nota y referencia de
          origen.
        </p>
      </div>
    );
  }

  return (
    <>
      <p className="muted historial-resoluciones-contador">
        {resoluciones.length} resoluciones
      </p>
      <ol className="historial-resoluciones-lista">
        {resoluciones.map((asiento) => (
          <li key={asiento.id} className="historial-resoluciones-item">
            <EntradaDeResolucion asiento={asiento} />
          </li>
        ))}
      </ol>
    </>
  );
}

/**
 * Una entrada del historial. El payload del asiento se lee con
 * `leerResolucionAsentada` (identificacion/tipos.ts, contrato provisional de
 * #175): si no trae los campos que esa funcion exige, el asiento NO se
 * esconde -es justo lo que el plano pide-, se pinta igual con
 * "Resolución manual con un detalle que no se pudo leer.".
 *
 * El nombre del actor para el avatar y la cabecera sale de `actor_nombre` del
 * payload cuando se pudo leer. Con un payload ilegible no hay `actor_nombre`
 * que mostrar -es uno de los campos que la lectura exige-, asi que la
 * cabecera cae al `actor` del propio asiento (el identificador que pone la
 * bitacora, no un nombre para mostrar): sigue siendo mejor que dejar la
 * entrada sin ningun indicio de quien decidio.
 */
function EntradaDeResolucion({ asiento }: { asiento: Asiento }) {
  const resolucion = leerResolucionAsentada(asiento.payload);
  const nombre = resolucion?.actorNombre ?? asiento.actor;

  return (
    <div className="historial-resoluciones-entrada">
      <header className="historial-resoluciones-cabecera">
        <span className="avatar" aria-hidden="true">
          {iniciales(nombre)}
        </span>
        <div>
          <p className="historial-resoluciones-actor">{nombre}</p>
          <time
            dateTime={asiento.cuando}
            title={asiento.cuando}
            className="muted"
          >
            {formatearInstante(asiento.cuando)}
          </time>
        </div>
      </header>
      {resolucion === null ? (
        <p>Resolución manual con un detalle que no se pudo leer.</p>
      ) : (
        <>
          <p>
            Asignó “{resolucion.titulo}” de {resolucion.fuente},{" "}
            {resolucion.periodo}, a esta obra.
          </p>
          <p className="detalle-nota">{resolucion.nota}</p>
          <p className="muted detalle-identificador">
            {resolucion.usoId} · {resolucion.reporteId}
          </p>
        </>
      )}
    </div>
  );
}
