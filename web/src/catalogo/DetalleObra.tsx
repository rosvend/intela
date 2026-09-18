import { Link, useLocation, useParams } from "react-router-dom";
import { ApiError } from "../api";
import Cargando from "../Cargando";
import { useApi } from "../useApi";
import { CLAVE_DE_VUELTA_AL_CATALOGO } from "./Catalogo";
import {
  ROTULO_NOMBRE_EN_PADRON_ACTUAL,
  formatearPorcentaje,
} from "./declaracion";
import { EtiquetaDeDeclaracion } from "./EtiquetaDeDeclaracion";
import {
  esObra,
  esVersionDeclaracion,
  type Obra,
  type Parte,
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
  // La direccion de vuelta se resuelve UNA vez, aqui, y baja a las dos salidas
  // de la pantalla: la ficha y el aviso de que la obra ya no esta. Quien llego
  // desde el catalogo vuelve a su busqueda en los dos casos, que es lo que
  // quiere quien acaba de ver que la obra se fue.
  const destinoDeVuelta = useDestinoDeVueltaAlCatalogo();
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

  return <FichaDeObra obra={obra} volver={destinoDeVuelta} />;
}

/**
 * A donde lleva "Volver al catalogo" desde esta pantalla.
 *
 * El administrador busca, abre una obra y vuelve: ese ida y vuelta es el flujo
 * normal desde que cada fila del catalogo enlaza con su ficha. Si la vuelta
 * cayera en `/catalogo` a secas, la busqueda que acaba de escribir se perderia
 * y tendria que rehacerla entera -filtros y pagina- para seguir donde estaba.
 * Por eso el catalogo le entrega su direccion al abrir la fila
 * (`CLAVE_DE_VUELTA_AL_CATALOGO`) y esta pantalla devuelve a ELLA.
 *
 * Lo que se descarto fue `navigate(-1)`, que parece mas corto y es otra cosa:
 * retroceder el historial no es volver al catalogo, es ir a donde el navegador
 * tuviera antes. Cuando la ficha se abre por su direccion -un enlace guardado,
 * un enlace de otra pantalla, una pestaña nueva desde una fila- puede no haber
 * ninguna entrada del catalogo detras, y el router no dice si la hay:
 * `navigate(-1)` saldria de la aplicacion o no haria nada, y ninguna de las dos
 * cosas se puede explicar en pantalla. El enlace, en cambio, siempre lleva a
 * una direccion que existe.
 */
function useDestinoDeVueltaAlCatalogo(): string {
  const busqueda = busquedaDeVueltaAlCatalogo(useLocation().state);
  // Sin busqueda que devolver -o con la busqueda vacia, que es el catalogo sin
  // filtros- la vuelta es `/catalogo`: esa direccion siempre existe y siempre
  // pinta algo, asi que nadie se queda sin salida. Y no se inventa ningun
  // filtro: la ficha no afirma una busqueda que nadie hizo.
  return busqueda === "" ? "/catalogo" : `/catalogo?${busqueda}`;
}

/**
 * La busqueda que el catalogo dejo en esta entrada del historial, o "".
 *
 * El estado de una entrada lo pone quien navega y no tiene forma garantizada:
 * aqui llega `null` o `undefined` en cualquier entrada que no venga del
 * catalogo, y podria llegar otra cosa -otra pantalla que use el estado para lo
 * suyo, un `state` construido a mano-. Se lee solo si es el texto que el
 * catalogo entrega; cualquier otra forma se trata como "no vengo del catalogo",
 * en vez de colarse en la direccion de vuelta.
 */
function busquedaDeVueltaAlCatalogo(estado: unknown): string {
  if (typeof estado !== "object" || estado === null) return "";
  const valor = (estado as Record<string, unknown>)[
    CLAVE_DE_VUELTA_AL_CATALOGO
  ];
  return typeof valor === "string" ? valor : "";
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
 */
function FichaDeObra({ obra, volver }: { obra: Obra; volver: string }) {
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
          </>
        )}
      </section>
    </section>
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

  return <TablaDePartes partes={vigente.partes} />;
}

/**
 * Las partes de la version vigente, con el porcentaje de cada titular.
 *
 * La columna del nombre se rotula con `ROTULO_NOMBRE_EN_PADRON_ACTUAL`, no con
 * "Nombre" a secas: el nombre no viaja en la parte y no es un dato de la
 * version -se resuelve contra el padron de HOY, asi que un titular renombrado
 * apareceria con su nombre nuevo hasta en las versiones antiguas (D-006)-. El
 * rotulo dice de donde saldria el dato; el `titular_id` va en la primera
 * columna, visible, para poder conciliar la pantalla con la API y para
 * distinguir homonimos.
 *
 * Esa columna va vacia en esta pantalla, con el guion que el catalogo ya usa
 * para "no se conoce": resolver el nombre aqui exigiria barrer el padron
 * -`GET /titulares` se sirve paginado y no admite filtrar por identificador-, y
 * un titular que no aparezca en la pagina pedida NO prueba que no exista, asi
 * que la celda no puede decir ni el nombre ni que falte.
 */
function TablaDePartes({ partes }: { partes: readonly Parte[] }) {
  return (
    <>
      <div className="catalogo-caja">
        <table
          className="tabla-partes"
          aria-label="Partes de la declaración vigente"
        >
          <thead>
            <tr>
              <th scope="col">Titular</th>
              <th scope="col">IPI</th>
              <th scope="col">{ROTULO_NOMBRE_EN_PADRON_ACTUAL}</th>
              <th scope="col" className="tabla-partes-numero">
                Porcentaje
              </th>
            </tr>
          </thead>
          <tbody>
            {partes.map((parte) => (
              // El `titular_id` como clave: el backend rechaza una declaracion
              // con un titular repetido, asi que no hay dos filas con el mismo.
              <tr key={parte.titular_id}>
                <td className="detalle-identificador">{parte.titular_id}</td>
                <td className="detalle-identificador">{parte.ipi}</td>
                <td className="muted">—</td>
                <td className="tabla-partes-numero">
                  {formatearPorcentaje(parte.porcentaje)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="muted detalle-nota">
        Esta pantalla no busca el nombre en el padrón: se sirve por páginas y no
        admite filtrar por identificador, así que una página que no traiga al
        titular no probaría que no exista. La columna se rotula «en el padrón
        actual» porque, el día que se resuelva, el nombre será el de hoy y no el
        de la fecha de esta versión (D-006).
      </p>
    </>
  );
}
