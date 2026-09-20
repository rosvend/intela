import { Link, useParams } from "react-router-dom";
import Cargando from "../Cargando";
import { useApi } from "../useApi";
import { CLAVE_DE_VUELTA_AL_CATALOGO, useVueltaAlCatalogo } from "./Catalogo";
import { formatearPorcentaje } from "./declaracion";
import { EtiquetaDeDeclaracion } from "./EtiquetaDeDeclaracion";
import { TablaDePartes, useNombresDeTitulares } from "./TablaDePartes";
import {
  esVersionDeclaracion,
  type Obra,
  type Parte,
  type VersionDeclaracion,
} from "./tipos";
import { conciliarConElHistorial, rutaDelHistorial, useObra } from "./useObra";

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
 * - las PARTES vienen de `GET /obras/{id}/declaracion/historial?limite=1`: el
 *   servidor pagina el historial desde la version mas reciente, asi que la
 *   pagina de una version es la abierta, la que marca `vigente_hasta: null`. No
 *   hay un `GET .../declaracion` que devuelva solo la vigente: el historial es la
 *   unica lectura donde estan sus partes.
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
  // La obra y sus desenlaces, de `useObra` (item 13): el preambulo que esta
  // pantalla compartia con el historial y el editor -la peticion, el 404 como
  // caso propio y la guarda de forma- vive alli, en un solo sitio. Lo que se
  // decide aqui es solo que se pinta con cada desenlace.
  const estado = useObra(id);

  if (estado.estado === "cargando")
    return <Cargando texto="Cargando la obra…" />;

  if (estado.estado === "ausente") {
    return (
      <ObraAusente
        id={estado.id}
        volver={destinoDeVuelta}
        className="detalle-obra"
        explicacion=". Si has llegado desde el catálogo, la lista y esta pantalla son dos consultas distintas: la de aquí es la que acaba de responder, y con ese identificador no encontró nada."
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
    // El texto sigue siendo el de esta pantalla -"los datos que esta pantalla
    // lee"- y no el de las otras dos: `useObra` no lo pone el porque no lo sabe,
    // y unificarlo seria cambiar lo que el administrador lee sin que nada lo
    // haya pedido.
    return (
      <p className="catalogo-error" role="alert">
        La obra no llegó con los datos que esta pantalla lee.
      </p>
    );
  }

  return (
    <FichaDeObra
      obra={estado.obra}
      volver={destinoDeVuelta}
      busqueda={busqueda}
    />
  );
}

/**
 * Lo que se ve cuando el servidor no tiene ninguna obra con ese identificador.
 *
 * Es UNO para las tres pantallas de la obra -el detalle, el historial y el
 * editor-, que hasta el item 13 tenian su copia con la misma cabecera, el mismo
 * enlace y la misma `<section>`: tres textos que podian divergir y tres sitios
 * donde arreglar el mismo defecto. Lo que cambia entre las tres no es el aviso,
 * es el PORQUE -"la lista y esta pantalla son dos consultas distintas", "no hay
 * historial que mostrar", "no hay declaracion que abrir"-, y por eso entra como
 * `explicacion` en vez de duplicarse: el hecho es el mismo y quien lo lee
 * necesita su caso.
 *
 * Vive en este modulo y no en `useObra.ts` porque ese es un modulo de hook
 * (`.ts`) y el plan del paso 13 autoriza cinco ficheros: la alternativa era una
 * `<section>` copiada en cada pantalla, que es el defecto que este paso cierra.
 * Es el mismo reparto que `Catalogo.tsx`, que ya exporta a las tres pantallas su
 * `useVueltaAlCatalogo` y su `CLAVE_DE_VUELTA_AL_CATALOGO`.
 *
 * `className` entra como parametro porque la seccion de cada pantalla es la
 * suya: el aviso no cambia de forma al cambiar de pantalla, pero si de sitio.
 */
export function ObraAusente({
  id,
  volver,
  className,
  explicacion,
}: {
  id: string;
  volver: string;
  className: string;
  explicacion: string;
}) {
  return (
    <section className={className}>
      <h1>Esa obra no está en el catálogo</h1>
      <p className="muted">
        El servidor no tiene ninguna obra con el identificador <code>{id}</code>
        {explicacion}
      </p>
      <p className="detalle-volver">
        <Link to={volver}>Volver al catálogo</Link>
      </p>
    </section>
  );
}

/**
 * Lo que se dice cuando la obra no tiene ninguna declaracion registrada.
 *
 * Es una constante porque el mismo hecho se dice en DOS sitios de esta pantalla,
 * y son el mismo hecho y no dos parecidos: la ficha, cuando `version_vigente`
 * llega en `null` -y entonces no se pide el historial, porque ya dijo lo que
 * diria-, y el bloque de partes, en el desenlace `sinVigente` de la conciliacion
 * -que esta pantalla no alcanza, pero la union trae porque el editor si pasa por
 * el-. Dos copias del texto serian dos sitios donde se puede quedar mintiendo
 * una de las dos, que es el defecto que este paso existe para no repetir.
 */
const SIN_DECLARACION =
  "La obra no tiene ninguna declaración registrada, así que no hay porcentajes declarados que repartir: bajo R-04 (RD 13.1.3) el importe completo de la obra queda en reserva y nunca se prorratea.";

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
          <p className="muted detalle-nota">{SIN_DECLARACION}</p>
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
            <PartesDeLaVersionVigente obra={obra} />
            <EnlaceAlHistorial obraId={obra.id} busqueda={busqueda} />
          </>
        )}

        <EnlaceAlEditor
          obraId={obra.id}
          busqueda={busqueda}
          hayDeclaracion={!sinDeclaracion}
        />
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
 * Es el enlace al historial de ESTA ficha -al que se llega tambien desde el
 * editor, que ofrece el historial para comprobar que version quedo abierta-, y
 * lleva el `id` de ESTA obra y la busqueda del catalogo -que el historial
 * devuelve al detalle-, para que el camino de vuelta no pierda la busqueda por
 * pasar por una pantalla mas.
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
 * El enlace al editor del reparto (paso 8).
 *
 * Va al final del bloque de la declaracion y SIEMPRE, con o sin declaracion, que
 * es lo que lo distingue del enlace al historial: a la ficha se llega tanto para
 * revisar un reparto vigente como para declarar una obra que nadie declaro, y en
 * los dos casos el siguiente paso es el editor -editarlo o abrirlo por primera
 * vez-. Ofrecerlo solo con declaracion dejaria la primera declaracion de una
 * obra sin ninguna puerta, que es el defecto que este paso existe para no
 * repetir: la pantalla quedaria construida y sin forma de llegar a ella.
 *
 * El texto cambia con el caso porque el caso es distinto -"Editar el reparto" y
 * "Declarar el reparto" no son lo mismo para quien lo lee- y ninguno de los dos
 * afirma un numero de version: que el guardado cierre la vigente y abra una
 * nueva lo dice el editor, que es donde se sabe cual se cierra (D-009).
 *
 * Es el unico enlace al editor en todo `web/src`, y lleva el `id` de ESTA obra
 * -un enlace que llevara al editor de otra obra pasaria un test que solo mirara
 * el texto- y la busqueda del catalogo, para que la vuelta no la pierda.
 */
function EnlaceAlEditor({
  obraId,
  busqueda,
  hayDeclaracion,
}: {
  obraId: string;
  busqueda: string;
  hayDeclaracion: boolean;
}) {
  return (
    <p className="detalle-nota">
      <Link
        to={`/catalogo/${encodeURIComponent(obraId)}/declaracion`}
        state={{ [CLAVE_DE_VUELTA_AL_CATALOGO]: busqueda }}
      >
        {hayDeclaracion ? "Editar el reparto" : "Declarar el reparto"}
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
function PartesDeLaVersionVigente({ obra }: { obra: Obra }) {
  // Basta la version abierta, y el servidor pagina el historial desde la mas
  // reciente: `limite=1` es esa. Traerlas todas para pintar las partes de una
  // sola costaba lo que midiera el historial.
  const {
    datos: historial,
    cargando,
    error,
  } = useApi<VersionDeclaracion[]>(rutaDelHistorial(obra.id, 1));

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

  // La obra dice cual es su version vigente y el historial dice cual tiene
  // abierta: son dos lecturas del mismo hecho, y si no coinciden -una
  // declaracion se guardo entre las dos peticiones, o el historial no trae
  // ninguna abierta- pintar las partes de la que el historial abre afirmaria un
  // reparto que `GET /obras/{id}` no esta sosteniendo. Se dice que no coinciden
  // y no se pinta ninguna: la pantalla no elige entre las dos.
  //
  // Quien decide eso es `conciliarConElHistorial`, en `useObra.ts`, y es la
  // UNICA expresion de esa regla: el editor del reparto comprueba exactamente lo
  // mismo con el mismo codigo desde el paso 16, en vez de con su propia copia
  // del `filter` (PM-6 del plan). Aqui solo se decide QUE se pinta con cada
  // desenlace.
  const conciliacion = conciliarConElHistorial(obra, historial);

  if (conciliacion.estado === "descuadrado") {
    return (
      <p className="catalogo-error" role="alert">
        {conciliacion.mensaje} No se muestran partes: pintar las de otra versión
        como la declaración vigente afirmaría un reparto que el sistema no está
        sosteniendo.
      </p>
    );
  }

  // La obra no declara ninguna version vigente y el historial no abre ninguna.
  // Esta pantalla NO llega aqui -la ficha no monta este bloque sin
  // `version_vigente`, y por eso no pide un historial que ya sabe vacio-, y se
  // pinta el hecho verdadero en vez de un aviso de error que seria falso: la
  // union trae este desenlace porque el EDITOR si pasa por el, que es la obra a
  // la que va a abrirle su primera version.
  if (conciliacion.estado === "sinVigente") {
    return <p className="muted detalle-nota">{SIN_DECLARACION}</p>;
  }

  if (conciliacion.vigente.partes.length === 0) {
    return (
      <p className="muted detalle-nota">
        La versión vigente no trae ninguna parte declarada.
      </p>
    );
  }

  return (
    <TablaDePartesConNombres
      partes={conciliacion.vigente.partes}
      titulo="Partes de la declaración vigente"
    />
  );
}

/**
 * La tabla de la version vigente con los nombres de sus partes resueltos.
 *
 * Los `titular_id` que viajan al padron son los de las filas que se van a
 * pintar, y en UNA peticion para la pantalla entera (item 9b): lo que no puede
 * crecer con los datos es el numero de peticiones, ni una por fila ni una por
 * columna.
 *
 * Se monta solo cuando hay partes; el bloque de arriba ya devolvio el texto de
 * "no trae ninguna parte" cuando no las hay. Es la guarda que el hook necesita y
 * no puede poner el: con una lista de identificadores vacia, `ids` no filtra
 * -el contrato dice que es la misma pregunta que no mandarlo-, asi que la
 * peticion pediria el padron entero y no podria cambiar nada de lo que se ve.
 */
function TablaDePartesConNombres({
  partes,
  titulo,
}: {
  partes: readonly Parte[];
  titulo: string;
}) {
  const nombres = useNombresDeTitulares(
    partes.map((parte) => parte.titular_id),
  );

  return <TablaDePartes partes={partes} titulo={titulo} nombres={nombres} />;
}
