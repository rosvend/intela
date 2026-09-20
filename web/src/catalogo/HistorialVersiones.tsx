import { Link, useParams } from "react-router-dom";
import Cargando from "../Cargando";
import { formatearInstante } from "../tablero/formato";
import { useApi } from "../useApi";
import { CLAVE_DE_VUELTA_AL_CATALOGO, useVueltaAlCatalogo } from "./Catalogo";
import { ObraAusente } from "./DetalleObra";
import { EtiquetaDeEstado } from "./EtiquetaDeDeclaracion";
import {
  SIN_NOMBRES,
  TablaDePartes,
  useNombresDeTitulares,
  type NombresDeTitulares,
} from "./TablaDePartes";
import {
  esVersionDeclaracion,
  type Obra,
  type VersionDeclaracion,
} from "./tipos";
import { useObra } from "./useObra";

/**
 * El historial de versiones de la Declaracion de Obra, en SOLO LECTURA (issue
 * #30, paso 7).
 *
 * De donde sale cada cosa, que es lo que decide como esta escrito todo lo de
 * abajo:
 *
 * - las VERSIONES -con su ventana de vigencia, su estado y sus partes- vienen de
 *   `GET /obras/{id}/declaracion/historial`, que las devuelve todas y ordenadas
 *   de la mas antigua a la mas reciente (api/openapi.yaml). La pantalla las
 *   pinta en ESE orden y no las reordena: el orden es del servidor y una
 *   segunda opinion del cliente sobre el registro es como el cliente empieza a
 *   discrepar de el;
 * - el TITULO de la obra -y el hecho de que exista- vienen de `GET /obras/{id}`.
 *   Hacen falta los dos: una pantalla que solo enseñara el identificador de la
 *   URL obligaria a comprobar a mano de que obra es el historial, y el historial
 *   de una obra que no existe llega como lista VACIA, no como 404 -mismo
 *   criterio que `GET /obras`-, asi que sin leer la obra no habria forma de
 *   distinguir "esta obra no tiene ninguna version" de "esa obra no esta en el
 *   catalogo".
 *
 * Lo que esta pantalla NO hace, y es lo que mas importa de todo:
 *
 * - NO calcula nada. El estado de cada version es el suyo, el que el backend
 *   calculo para ESA version (`R-04` incluido), y la suma no se recalcula aqui:
 *   no hay una suma por version en el contrato, asi que la pantalla no la
 *   inventa sumando las partes -una suma del cliente puede contradecir al
 *   estado que acaba de recibir, y el redondeo de cuatro decimales basta para
 *   que pase-;
 * - NO pinta el estado de la OBRA. `GET /obras/{id}` trae
 *   `estado_declaracion` y `version_vigente`, y se leen solo para saber si la
 *   obra existe y como se llama: el estado de una version pasada es un hecho
 *   historico, y sustituirlo por el de hoy diria de una version cerrada algo
 *   que el sistema no sostiene (D-008);
 * - NO ofrece ningun control. El contrato lo dice de si mismo: no hay forma de
 *   editar una version pasada, unicamente de abrir una nueva (`PUT
 *   /obras/{id}/declaracion`, que es el paso 8 y se hace desde la ficha). Por
 *   eso no hay boton, ni formulario, ni enlace al editor: un control que no
 *   hace nada es peor que su ausencia.
 */
export default function HistorialVersiones() {
  const { id = "" } = useParams();
  // La misma vuelta al catalogo que resuelve el detalle (paso 6): las dos
  // pantallas leen la busqueda que el catalogo dejo en su entrada y la devuelven
  // igual. Viven en una sola funcion a proposito -una copia por pantalla es lo
  // que las deja discrepando-, y `destino` se usa aqui para el unico caso en el
  // que volver a la obra no lleva a ninguna parte: que la obra no este.
  const { busqueda, destino } = useVueltaAlCatalogo();
  // La obra y sus desenlaces, de `useObra` (item 13): el preambulo que esta
  // pantalla compartia con el detalle y el editor -la peticion, el 404 como caso
  // propio y la guarda de forma- vive alli, en un solo sitio.
  const estado = useObra(id);

  if (estado.estado === "cargando")
    return <Cargando texto="Cargando el historial…" />;

  if (estado.estado === "ausente") {
    return (
      <ObraAusente
        id={estado.id}
        volver={destino}
        className="historial-versiones"
        explicacion=", así que no hay ningún historial de declaración que mostrar. El historial de una obra que sí existe y todavía no se ha declarado llega como una lista vacía, no como un error: los dos casos no son el mismo."
      />
    );
  }

  if (estado.estado === "error") {
    return (
      <p className="catalogo-error" role="alert">
        No se pudo consultar la obra: {estado.mensaje}
      </p>
    );
  }

  if (estado.estado === "ilegible") {
    // `useApi<Obra>` promete una obra, pero `T` es una promesa y no una
    // comprobacion: un 2xx con otra forma llega hasta aqui y `obra.titulo` o
    // `obra.id` -lo unico que esta pantalla lee- tumbarian el historial entero.
    // La guarda es la del tipo, la misma que usan el listado y el detalle.
    return (
      <p className="catalogo-error" role="alert">
        La obra no llegó con la forma que el contrato promete para una obra.
      </p>
    );
  }

  return <HistorialDeObra obra={estado.obra} busqueda={busqueda} />;
}

/**
 * La obra ya legible: su historial de versiones.
 *
 * `busqueda` no se pinta: viaja de vuelta al detalle, que es a donde lleva
 * "Volver a la obra", para que el camino de regreso al catalogo conserve la
 * busqueda aunque el administrador haya pasado por dos pantallas.
 */
function HistorialDeObra({ obra, busqueda }: { obra: Obra; busqueda: string }) {
  return (
    <section className="historial-versiones">
      <p className="detalle-volver">
        {/* La vuelta es a la OBRA, que es de donde se llega a esta pantalla, y
            no al catalogo a secas: el catalogo queda a un clic desde la ficha y
            el administrador vuelve al sitio del que salio. La busqueda del
            catalogo se le entrega al detalle para que la siga devolviendo. */}
        <Link
          to={`/catalogo/${encodeURIComponent(obra.id)}`}
          state={{ [CLAVE_DE_VUELTA_AL_CATALOGO]: busqueda }}
        >
          ← Volver a la obra
        </Link>
      </p>

      <header className="detalle-cabecera">
        <h1>Historial de la declaración</h1>
        <p className="muted detalle-nota">
          {obra.titulo} · {obra.id}
        </p>
        {/* De que se trata esta pantalla, dicho una vez y sin prometer nada que
            el contrato no diga: el contrato dice que la version anterior no se
            borra ni se modifica y que solo se abre una nueva. El estado de cada
            version es el que el backend calculo para ella, y por eso aqui abajo
            puede no coincidir con el de hoy: es lo que regia entonces. */}
        <p className="muted detalle-nota">
          Cada versión con el reparto que regía en su momento. El servidor no
          permite editar una versión pasada: la anterior no se borra ni se
          modifica, solo se abre una nueva, y por eso esta pantalla es de solo
          lectura. El estado de cada versión lo calcula el servidor para esa
          versión y se muestra tal como llega.
        </p>
      </header>

      <VersionesDeLaObra obraId={obra.id} />
    </section>
  );
}

/**
 * Las versiones de la declaracion, tal como llegan.
 *
 * Se monta SOLO cuando la obra se pudo leer: pedir el historial de una obra que
 * no esta -o que no se pudo comprobar- seria una peticion que no puede cambiar
 * nada de lo que se ve, y su 404 no diria mas que el de la obra.
 */
function VersionesDeLaObra({ obraId }: { obraId: string }) {
  const {
    datos: historial,
    cargando,
    error,
  } = useApi<VersionDeclaracion[]>(
    `/api/obras/${encodeURIComponent(obraId)}/declaracion/historial`,
  );

  if (cargando) return <Cargando texto="Cargando las versiones…" />;

  if (error) {
    return (
      <p className="catalogo-error" role="alert">
        No se pudo consultar el historial de la declaración: {error.message}
      </p>
    );
  }

  // `useApi<VersionDeclaracion[]>` promete una lista y no la comprueba: un 2xx
  // con otra forma llega hasta aqui, y cada version pasa por
  // `esVersionDeclaracion`, que es quien conoce los campos que lee esta pantalla
  // -incluidos `vigente_desde` y `estado`, que hasta este paso no leia nadie-.
  // Una version mal formada deja el historial entero en error a proposito:
  // saltarse el bloque malo escondería una version -y su reparto, y su estado-
  // sin decirlo.
  if (!Array.isArray(historial) || !historial.every(esVersionDeclaracion)) {
    return (
      <p className="catalogo-error" role="alert">
        El historial no llegó como una lista de versiones legibles.
      </p>
    );
  }

  // Una obra sin ninguna declaracion devuelve una lista vacia, no un 404, y no
  // es un fallo: se dice el hecho y se dice que no es un error, que es lo que
  // distingue este estado del de arriba. Ni `role="alert"` -no hay nada roto-
  // ni el rojo que esta reservado a lo que de verdad falla.
  if (historial.length === 0) {
    return (
      <div className="catalogo-vacio">
        <p>Esta obra no tiene ninguna versión declarada.</p>
        <p className="muted">
          El servidor devuelve una lista vacía, no un error, para una obra sin
          ninguna declaración: no es un fallo de esta pantalla.
        </p>
      </div>
    );
  }

  return (
    <>
      <p className="muted detalle-nota">
        {historial.length === 1
          ? "El historial trae 1 versión."
          : `El historial trae ${historial.length} versiones.`}{" "}
        Vienen del servidor de la más antigua a la más reciente, en ese orden:
        esta pantalla no las reordena. La que rige hoy es la que llega sin
        cerrar.
      </p>
      {/* Una version sin partes no monta tabla, asi que si NINGUNA trae partes no
          hay un solo nombre que resolver -y el hook no sabe pedir "ninguno":
          `ids` vacio es, en el contrato, el padron entero-. Quien decide cual de
          los dos caminos se toma es esta pantalla, que ya conoce el historial;
          por eso son dos componentes y no una condicion dentro de uno. */}
      {historial.every((version) => version.partes.length === 0) ? (
        <BloquesDeVersiones versiones={historial} nombres={SIN_NOMBRES} />
      ) : (
        <BloquesDeVersionesConNombres versiones={historial} />
      )}
    </>
  );
}

/**
 * Los bloques de version de la pantalla, con los nombres ya resueltos.
 *
 * Es lo que la pantalla monta SIEMPRE -con o sin nombres-: las versiones se
 * pintan una por una en el orden en que llegaron, y cada bloque decide si su
 * tabla existe (una version sin partes no tiene ninguna).
 */
function BloquesDeVersiones({
  versiones,
  nombres,
}: {
  versiones: readonly VersionDeclaracion[];
  nombres: NombresDeTitulares;
}) {
  return (
    <>
      {versiones.map((version) => (
        <BloqueDeVersion
          key={version.version}
          version={version}
          nombres={nombres}
        />
      ))}
    </>
  );
}

/**
 * Los mismos bloques, con los nombres de sus partes resueltos en UNA sola
 * peticion para la pantalla entera.
 *
 * Los identificadores que viajan son los de las partes de TODAS las versiones, y
 * sin repetir: `ids` es un filtro por lista, asi que el titular que aparece en
 * tres versiones se pide una vez. Es lo que hace que el numero de peticiones no
 * crezca con los datos: una version mas con los mismos titulares no anade una
 * consulta, anade filas a la que ya se hace, y una version con un titular nuevo
 * anade un elemento a la misma lista.
 */
function BloquesDeVersionesConNombres({
  versiones,
}: {
  versiones: readonly VersionDeclaracion[];
}) {
  const nombres = useNombresDeTitulares(
    versiones.flatMap((version) =>
      version.partes.map((parte) => parte.titular_id),
    ),
  );

  return <BloquesDeVersiones versiones={versiones} nombres={nombres} />;
}

/**
 * Una version: su numero, su estado, su ventana de vigencia y sus partes.
 *
 * La ventana se pinta en los dos casos con su nombre delante -"Vigente desde" y
 * "Vigente hasta"-, y una version que sigue abierta lo dice con palabras: el
 * contrato manda `vigente_hasta` en `null` para la que rige ahora mismo, y un
 * hueco o un guion ahi dejarian al lector sin saber si el dato no llego o si la
 * version no se ha cerrado. Es lo unico que distingue la version vigente de las
 * cerradas, y por eso la palabra no es un adorno.
 */
function BloqueDeVersion({
  version,
  nombres,
}: {
  version: VersionDeclaracion;
  nombres: NombresDeTitulares;
}) {
  return (
    <article className="historial-version">
      <h2>Versión {version.version}</h2>

      <dl className="detalle-ficha">
        <div>
          <dt>Estado</dt>
          <dd>
            {/* El estado de ESTA version, no el de la obra: es el que el
                backend calculo para este reparto, y una version cerrada que
                declaraba de menos lo sigue siendo aunque hoy la obra declare
                completo. */}
            <EtiquetaDeEstado estado={version.estado} />
          </dd>
        </div>
        <div>
          <dt>Vigente desde</dt>
          <dd>
            {/* El instante exacto que mando el servidor va en `dateTime` y en
                `title`, sin recortar: el texto legible baja a minutos, y dos
                versiones abiertas el mismo dia se distinguen por microsegundos
                (`RFC3339Nano`). El numero de version las separa igual, pero el
                dato no se pierde por enseñarlo mas corto. */}
            <time
              dateTime={version.vigente_desde}
              title={version.vigente_desde}
            >
              {formatearInstante(version.vigente_desde)}
            </time>
          </dd>
        </div>
        <div>
          <dt>Vigente hasta</dt>
          <dd>
            {version.vigente_hasta === null ? (
              <span className="historial-abierta">Sin cerrar</span>
            ) : (
              <time
                dateTime={version.vigente_hasta}
                title={version.vigente_hasta}
              >
                {formatearInstante(version.vigente_hasta)}
              </time>
            )}
          </dd>
        </div>
      </dl>

      {version.partes.length === 0 ? (
        // Una version sin partes es posible -el contrato lo admite-, y una tabla
        // con solo el encabezado se lee como "no hay titulares" sin decir si el
        // reparto llego vacio o no llego.
        <p className="muted detalle-nota">
          Esta versión no trae ninguna parte declarada.
        </p>
      ) : (
        <TablaDePartes
          partes={version.partes}
          // El nombre accesible dice de que version es la tabla: esta pantalla
          // pinta una por version y sin el no se podria decir cual se esta
          // leyendo -ni un lector de pantalla ni un test-.
          titulo={`Partes de la versión ${version.version}`}
          // Los nombres son los MISMOS para todas las tablas de la pantalla: se
          // resolvieron una vez, arriba, por la union de los identificadores de
          // todas las versiones (item 9b).
          nombres={nombres}
        />
      )}
    </article>
  );
}
