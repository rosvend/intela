import { Link, useParams } from "react-router-dom";
import { ApiError } from "../api";
import Cargando from "../Cargando";
import { useApi } from "../useApi";
import { CLAVE_DE_VUELTA_AL_CATALOGO, useVueltaAlCatalogo } from "./Catalogo";
import { formatearPorcentaje } from "./declaracion";
import { EtiquetaDeDeclaracion } from "./EtiquetaDeDeclaracion";
import { TablaDePartes } from "./TablaDePartes";
import {
  esObra,
  esVersionDeclaracion,
  type Obra,
  type VersionDeclaracion,
} from "./tipos";

/**
 * El detalle de una obra con su Declaracion de Obra vigente (issue #30, paso 6).
 *
 * De donde sale cada cosa, que es lo que decide como esta escrito todo lo de
 * abajo:
 *
 * - los METADATOS y las dos cifras de cabecera -`estado_declaracion` y
 *   `suma_porcentajes`- vienen de `GET /obras/{id}`, que es la autoridad: la
 *   pantalla los pinta tal cual y no los re-deriva de las partes. Re-derivarlos
 *   es como el cliente empieza a discrepar del backend -la suma de cuatro
 *   decimales redondea, y una parte sin IPI deja la declaracion `incompleta`
 *   con la suma en 100-, y esta pantalla es donde un administrador decide si la
 *   obra se puede repartir;
 * - las PARTES vienen de `GET /obras/{id}/declaracion/historial`, que devuelve
 *   TODAS las versiones y marca la vigente con `vigente_hasta: null`. No hay un
 *   `GET .../declaracion` que devuelva solo la vigente: el historial es la unica
 *   lectura donde estan sus partes.
 *
 * `version_vigente` en `null` quiere decir que la obra NO tiene ninguna
 * declaracion, y es el unico dato que distingue eso de "declarada y no suma
 * 100": las dos llegan como `incompleta`. Sin esa distincion la pantalla
 * pintaria "Incompleta" sobre una obra que nadie declaro (D-008), y una
 * declaracion que no existe no se puede afirmar.
 */
export default function DetalleObra() {
  const { id = "" } = useParams();
  // La direccion de vuelta al catalogo y la busqueda que hay que devolverle se
  // resuelven UNA vez, aqui, con la misma funcion que usa el historial (paso 7):
  // las dos salidas de la pantalla -la ficha y el aviso de que la obra ya no
  // esta- y el enlace al historial salen de ese unico sitio. Quien llego desde
  // el catalogo vuelve a su busqueda en los dos casos, que es lo que quiere
  // quien acaba de ver que la obra se fue.
  const { busqueda, destino: destinoDeVuelta } = useVueltaAlCatalogo();
  const {
    datos: obra,
    cargando,
    error,
  } = useApi<Obra>(`/api/obras/${encodeURIComponent(id)}`);

  if (cargando) return <Cargando texto="Cargando la obra…" />;

  if (error) {
    // El 404 no es un fallo del sistema: es el servidor diciendo que con ese
    // identificador no hay ninguna obra. El issue #30 lo tiene como caso propio
    // -una obra que se listo hace un momento y ya no esta-, y pintarlo con el
    // error generico de "no se pudo consultar" diria que algo se rompio cuando
    // lo que pasa es que esa obra ya no esta en el catalogo.
    if (error instanceof ApiError && error.status === 404) {
      return <ObraAusente id={id} volver={destinoDeVuelta} />;
    }
    return (
      <p className="catalogo-error" role="alert">
        No se pudo consultar la obra: {error.message}
      </p>
    );
  }

  // `useApi<Obra>` promete una obra, pero `T` es una promesa y no una
  // comprobacion: un 2xx con otra forma -el `{error: ...}` de un proxy, un
  // backend a medias- llega hasta aqui, y `obra.titulo` o `obra.version_vigente`
  // tumbarian la pantalla entera. Sin ErrorBoundary en `web/src`, eso la deja en
  // blanco. La guarda es la misma del listado del catalogo: los dos leen los
  // mismos campos, y una obra ilegible no se pinta a medias.
  if (!esObra(obra)) {
    return (
      <p className="catalogo-error" role="alert">
        La obra no llegó con los datos que esta pantalla lee.
      </p>
    );
  }

  return (
    <FichaDeObra obra={obra} volver={destinoDeVuelta} busqueda={busqueda} />
  );
}

/** Lo que se ve cuando el servidor no tiene ninguna obra con ese identificador. */
function ObraAusente({ id, volver }: { id: string; volver: string }) {
  return (
    <section className="detalle-obra">
      <h1>Esa obra no está en el catálogo</h1>
      <p className="muted">
        El servidor no tiene ninguna obra con el identificador <code>{id}</code>
        . Si has llegado desde el catálogo, la lista y esta pantalla son dos
        consultas distintas: la de aquí es la que acaba de responder, y con ese
        identificador no encontró nada.
      </p>
      <p className="detalle-volver">
        <Link to={volver}>Volver al catálogo</Link>
      </p>
    </section>
  );
}

/**
 * La obra ya legible: sus metadatos y su declaracion vigente.
 *
 * El estado y la suma se pintan tal cual llegan -los calcula el servidor-, y lo
 * unico que decide el cliente es SI HAY declaracion, que es lo que dice
 * `version_vigente`.
 *
 * `busqueda` no se pinta: viaja al historial -paso 7- para que el camino de
 * vuelta conserve la busqueda del catalogo aunque el administrador pase por
 * una pantalla mas.
 */
function FichaDeObra({
  obra,
  volver,
  busqueda,
}: {
  obra: Obra;
  volver: string;
  busqueda: string;
}) {
  const versionVigente = obra.version_vigente;
  const sinDeclaracion = versionVigente === null;

  return (
    <section className="detalle-obra">
      <p className="detalle-volver">
        <Link to={volver}>← Volver al catálogo</Link>
      </p>

      <header className="detalle-cabecera">
        <h1>{obra.titulo}</h1>
        {/* Una ficha con rotulo por dato y no una linea corrida: son cuatro
            clasificaciones del reglamento (el tipo es `RD 9.1.1`, el IDA
            identifica la obra ante REDES), y cada una tiene que poder citarse
            por su nombre desde una auditoria. */}
        <dl className="detalle-ficha">
          <div>
            <dt>Género</dt>
            <dd>{obra.genero}</dd>
          </div>
          <div>
            <dt>Año</dt>
            {/* Sin agrupar miles, por eso no pasa por `formatearEntero`:
                "1.991" no es un anio. */}
            <dd>{obra.anio}</dd>
          </div>
          <div>
            <dt>Tipo</dt>
            {/* El valor del enum, tal cual: es la clasificacion del reglamento
                y no hay traduccion de la casa que no invente. */}
            <dd>{obra.tipo}</dd>
          </div>
          <div>
            <dt>IDA</dt>
            <dd className="detalle-identificador">
              {obra.ida !== undefined && obra.ida !== "" ? obra.ida : "—"}
            </dd>
          </div>
          <div>
            <dt>Identificador de la obra</dt>
            <dd className="detalle-identificador">{obra.id}</dd>
          </div>
        </dl>
      </header>

      <section className="detalle-declaracion">
        <h2>Declaración vigente</h2>
        <p className="muted detalle-nota">
          El estado y la suma los calcula el servidor; esta pantalla los muestra
          tal como llegan, sin recalcularlos a partir de las partes.
        </p>

        <dl className="detalle-ficha">
          <div>
            <dt>Estado</dt>
            <dd>
              <EtiquetaDeDeclaracion obra={obra} />
            </dd>
          </div>
          <div>
            <dt>Suma de los porcentajes declarados</dt>
            {/* La suma va tambien sin declaracion: es lo que el backend manda
                en `suma_porcentajes` y ahi vale cero. Quien distingue los dos
                casos es la etiqueta, no un hueco. */}
            <dd className="detalle-cifra">
              {formatearPorcentaje(obra.suma_porcentajes)}
            </dd>
          </div>
          <div>
            <dt>Versión vigente</dt>
            <dd className="detalle-cifra">
              {sinDeclaracion ? "—" : versionVigente}
            </dd>
          </div>
        </dl>

        {sinDeclaracion ? (
          <p className="muted detalle-nota">
            La obra no tiene ninguna declaración registrada, así que no hay
            porcentajes declarados que repartir: bajo R-04 (RD 13.1.3) el
            importe completo de la obra queda en reserva y nunca se prorratea.
          </p>
        ) : (
          <>
            {obra.estado_declaracion === "incompleta" && (
              <p className="muted detalle-nota">
                {/* Un total por debajo de 100 es un estado valido del negocio,
                    no un error de quien declaro: por eso va en ambar y no en
                    rojo, y por eso se explica la consecuencia en vez del
                    defecto. Y el texto NO dice "no suma 100": `incompleta` es
                    tambien una parte sin IPI o con un porcentaje no positivo,
                    asi que el estado no prueba nada sobre la suma -por eso el
                    contrato manda los dos campos y no uno-. */}
                Una declaración incompleta no es un error: bajo R-04 (RD 13.1.3)
                no se reparte nada de esta obra y el importe completo queda en
                reserva, nunca se prorratea.
              </p>
            )}
            <PartesDeLaVersionVigente
              obraId={obra.id}
              versionVigente={versionVigente}
            />
            <EnlaceAlHistorial obraId={obra.id} busqueda={busqueda} />
          </>
        )}
      </section>
    </section>
  );
}

/**
 * El enlace al historial completo de la declaracion (paso 7).
 *
 * Donde va, y por que: al final de la seccion de la declaracion, despues del
 * reparto vigente. El historial es el registro de ESA declaracion -las versiones
 * anteriores con el reparto que estaba vigente entonces (S4 del issue #30)-, asi
 * que el enlace pertenece al bloque donde se acaba de leer el numero de version
 * vigente y sus partes; es la continuacion natural de lo que se esta mirando. No
 * va arriba, junto a "Volver al catalogo": dos enlaces de navegacion compitiendo
 * en la misma linea dejan al administrador sin saber cual es la vuelta.
 *
 * Se ofrece SOLO cuando la obra tiene una declaracion, que es cuando esta rama
 * se pinta: sin ella la ficha ya dice que no hay ninguna version, y el historial
 * diria otra vez el mismo hecho. Es la misma razon por la que esta pantalla no
 * pide un historial que ya sabe vacio.
 *
 * Es el unico enlace que lleva al historial en todo `web/src`, y lleva el `id`
 * de ESTA obra y la busqueda del catalogo -que el historial devuelve al
 * detalle-, para que el camino de vuelta no pierda la busqueda por pasar por una
 * pantalla mas.
 */
function EnlaceAlHistorial({
  obraId,
  busqueda,
}: {
  obraId: string;
  busqueda: string;
}) {
  return (
    <p className="detalle-nota">
      <Link
        to={`/catalogo/${encodeURIComponent(obraId)}/historial`}
        state={{ [CLAVE_DE_VUELTA_AL_CATALOGO]: busqueda }}
      >
        Ver el historial completo
      </Link>
    </p>
  );
}

/**
 * Las partes de la version vigente.
 *
 * Se monta SOLO cuando la obra tiene una version vigente. Con
 * `version_vigente` en `null` la obra ya dijo que no tiene ninguna declaracion
 * -que es exactamente lo que diria un historial vacio, el mismo hecho dicho
 * una vez-, asi que pedir el historial para volver a oirlo seria una peticion
 * que no puede cambiar nada de lo que se ve.
 */
function PartesDeLaVersionVigente({
  obraId,
  versionVigente,
}: {
  obraId: string;
  versionVigente: number;
}) {
  const {
    datos: historial,
    cargando,
    error,
  } = useApi<VersionDeclaracion[]>(
    `/api/obras/${encodeURIComponent(obraId)}/declaracion/historial`,
  );

  if (cargando) return <Cargando texto="Cargando la declaración vigente…" />;

  if (error) {
    return (
      <p className="catalogo-error" role="alert">
        No se pudo consultar el historial de la declaración: {error.message}
      </p>
    );
  }

  if (!Array.isArray(historial) || !historial.every(esVersionDeclaracion)) {
    return (
      <p className="catalogo-error" role="alert">
        El historial no llegó como una lista de versiones legibles.
      </p>
    );
  }

  const abiertas = historial.filter(
    (version) => version.vigente_hasta === null,
  );
  const vigente = abiertas.at(0);

  // La obra dice cual es su version vigente y el historial dice cual tiene
  // abierta: son dos lecturas del mismo hecho, y si no coinciden -una
  // declaracion se guardo entre las dos peticiones, o el historial no trae
  // ninguna- pintar las partes de la que el historial abre afirmaria un reparto
  // que `GET /obras/{id}` no esta sosteniendo. Se dice que no coinciden y no se
  // pinta ninguna: la pantalla no elige entre las dos.
  if (!vigente || abiertas.length > 1 || vigente.version !== versionVigente) {
    return (
      <p className="catalogo-error" role="alert">
        El servidor declara vigente la versión {versionVigente}, pero el
        historial no trae esa versión como única versión abierta. No se muestran
        partes: pintar las de otra versión como la declaración vigente afirmaría
        un reparto que el sistema no está sosteniendo.
      </p>
    );
  }

  if (vigente.partes.length === 0) {
    return (
      <p className="muted detalle-nota">
        La versión vigente no trae ninguna parte declarada.
      </p>
    );
  }

  return (
    <TablaDePartes
      partes={vigente.partes}
      titulo="Partes de la declaración vigente"
    />
  );
}
