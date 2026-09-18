import { useId, useState, type ReactElement } from "react";
import { Link, useLocation, useSearchParams } from "react-router-dom";
import Cargando from "../Cargando";
import { useApi } from "../useApi";
import { formatearPorcentaje } from "./declaracion";
import { EtiquetaDeDeclaracion } from "./EtiquetaDeDeclaracion";
import { esObra, type Obra } from "./tipos";

/**
 * Cuantas obras se piden por pagina. El servidor tiene su propio tope por
 * defecto (100, api/openapi.yaml) y `limite` no admite mas de 500, pero se
 * manda explicito: asi la pagina que se ve es la que dice la URL y un enlace
 * compartido abre lo mismo que vio quien lo copio.
 */
const LIMITE_POR_PAGINA = 20;

/**
 * La clave con la que el catalogo le entrega su direccion al detalle de una obra.
 *
 * Buscar, abrir una obra y volver es el camino de ida y vuelta normal del
 * administrador, y volver a `/catalogo` a secas le tiraria la busqueda que
 * acaba de escribir: tendria que teclearla otra vez y llegar de nuevo a la
 * pagina en la que estaba. Para devolverle ESA direccion, el detalle necesita
 * saber de cual se vino, y de eso se encarga esta clave.
 *
 * Viaja en el estado de la entrada del historial y no en la direccion de la
 * ficha, porque los filtros y la pagina son parametros de ESTA pantalla: en la
 * direccion del detalle serian unos parametros que esa pantalla no lee, y un
 * enlace copiado desde ahi los arrastraria como si significaran algo del
 * detalle.
 *
 * Se exporta desde aqui porque el catalogo es el dueno de su propia direccion;
 * `DetalleObra` la importa para leerla.
 */
export const CLAVE_DE_VUELTA_AL_CATALOGO = "catalogoDeOrigen";

/**
 * Lo que el catalogo dejo en esta entrada del historial: su busqueda, y la
 * direccion de vuelta ya resuelta a partir de ella.
 *
 * El administrador busca, abre una obra y vuelve: ese ida y vuelta es el flujo
 * normal desde que cada fila del catalogo enlaza con su ficha. Si la vuelta
 * cayera en `/catalogo` a secas, la busqueda que acaba de escribir se perderia
 * y tendria que rehacerla entera -filtros y pagina- para seguir donde estaba.
 * Por eso el catalogo entrega su direccion al abrir la fila
 * (`CLAVE_DE_VUELTA_AL_CATALOGO`) y esta funcion la devuelve tal como llego.
 *
 * Las dos mitades hacen falta y por eso se devuelven juntas: `destino` para
 * volver al catalogo, y `busqueda` para entregarsela a la pantalla siguiente
 * -el detalle se la pasa al historial, que se la devuelve al detalle-, de modo
 * que el camino de vuelta conserve la busqueda pase por donde pase.
 *
 * Vive aqui, y no en cada pantalla, porque son DOS las pantallas que la
 * reciben -el detalle de la obra y el historial de su declaracion, pasos 6 y
 * 7- y una copia por pantalla es exactamente lo que las deja discrepando: el
 * dia que una sola cambie, una conservara la busqueda y la otra no. El catalogo
 * es el dueno de su propia direccion y de como se vuelve a ella.
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
export function useVueltaAlCatalogo(): {
  busqueda: string;
  destino: string;
} {
  const busqueda = busquedaDeVueltaAlCatalogo(useLocation().state);
  // Sin busqueda que devolver -o con la busqueda vacia, que es el catalogo sin
  // filtros- la vuelta es `/catalogo`: esa direccion siempre existe y siempre
  // pinta algo, asi que nadie se queda sin salida. Y no se inventa ningun
  // filtro: la ficha no afirma una busqueda que nadie hizo.
  return {
    busqueda,
    destino: busqueda === "" ? "/catalogo" : `/catalogo?${busqueda}`,
  };
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

/**
 * Los cuatro filtros de busqueda, con los nombres EXACTOS de los parametros
 * que acepta `GET /obras` (`titulo` parcial sin distinguir mayusculas, `genero`
 * exacto, `anio` entero positivo, `ipi` exacto). Son tambien los nombres de los
 * parametros de la URL: una sola forma de nombrarlos evita la traduccion que se
 * desvia.
 */
const FILTROS = ["titulo", "genero", "anio", "ipi"] as const;
type Filtro = (typeof FILTROS)[number];

/**
 * La forma de un anio que el backend acepta: un entero positivo. Es la misma
 * regla de `buscarObras` (`strconv.Atoi` y `anio <= 0` = 400).
 *
 * Existe solo para no mandar una consulta que se sabe rechazada -teclear un
 * anio pasa por formas a medias-, no para sustituir al backend: la autoridad
 * para aceptar o rechazar sigue siendo el servidor, y si su regla cambia, su
 * mensaje es el que se ve en pantalla.
 */
const FORMA_ANIO = /^\d+$/;

function esAnioAplicable(texto: string): boolean {
  return FORMA_ANIO.test(texto) && Number(texto) > 0;
}

/**
 * El desplazamiento que se pide, leido de la URL. Un valor que no sea un entero
 * no negativo -lo unico que acepta `leerPaginacion`- se lee como 0 en vez de
 * mandarse: la URL la puede escribir cualquiera a mano.
 */
function leerDesplazamiento(bruto: string | null): number {
  const n = Number(bruto);
  return Number.isInteger(n) && n > 0 ? n : 0;
}

/**
 * El dato de cabecera del catalogo.
 *
 * El mockup ponia "Periodo 2024-S2" aqui. Se sustituye a proposito: el contrato
 * no tiene ningun concepto de periodo para una obra -los filtros son titulo,
 * genero, anio e IPI, y una obra no esta acotada a un semestre; lo que tiene
 * vigencia es su declaracion- asi que ese texto sugeriria un ambito temporal
 * que la API no puede acotar (D-010). Lo que se dice es lo que el sistema sabe:
 * cuantas obras trae la pagina y cuantos filtros estan puestos.
 *
 * "En esta pagina" no es un adorno: la respuesta NO trae el total de
 * coincidencias, solo la pagina pedida, asi que decir "N obras registradas"
 * seria afirmar un total que nadie conto.
 */
function textoDelResumen(obras: number, filtrosActivos: number): string {
  const cuantas =
    obras === 1 ? "1 obra en esta página" : `${obras} obras en esta página`;
  const activos =
    filtrosActivos === 0
      ? "sin filtros activos"
      : filtrosActivos === 1
        ? "1 filtro activo"
        : `${filtrosActivos} filtros activos`;
  return `${cuantas} · ${activos}`;
}

/** El tramo de resultados de la pagina, contado desde el desplazamiento pedido. */
function textoDelRango(desplazamiento: number, cuantas: number): string {
  return `Obras ${desplazamiento + 1} a ${desplazamiento + cuantas}`;
}

/**
 * Catalogo de obras con busqueda (issue #30, S1). Solo administrador: el grupo
 * entero `/obras` del servidor esta bajo `requiereRol(RolAdministrador)`
 * (`internal/infraestructura/httpapi/server.go`), y la entrada de `RUTAS` se
 * restringio igual para no ofrecer una pantalla que el servidor rechaza (D-013).
 *
 * La pantalla NO calcula el estado ni la suma de la declaracion: los dos vienen
 * del backend en `estado_declaracion` y `suma_porcentajes` y se pintan tal
 * cual. Re-derivarlos del reparto es como el cliente empieza a discrepar de la
 * autoridad -el redondeo de la suma basta para que pase- y el listado del
 * catalogo es justo donde un administrador decide que obras se pueden repartir.
 */
export default function Catalogo() {
  const [searchParams, setSearchParams] = useSearchParams();

  // Los filtros los manda la URL: es lo que hace que recargar la pagina, volver
  // atras o compartir el enlace conserven la busqueda, y lo que deja a `useApi`
  // pedir sola la consulta cuando cambia el path.
  const filtros: Record<Filtro, string> = {
    titulo: searchParams.get("titulo") ?? "",
    genero: searchParams.get("genero") ?? "",
    anio: searchParams.get("anio") ?? "",
    ipi: searchParams.get("ipi") ?? "",
  };
  const filtrosActivos = FILTROS.filter(
    (filtro) => filtros[filtro] !== "",
  ).length;
  const desplazamiento = leerDesplazamiento(searchParams.get("desplazamiento"));

  const params = new URLSearchParams();
  for (const filtro of FILTROS) {
    const valor = filtros[filtro];
    if (valor !== "") params.set(filtro, valor);
  }
  params.set("limite", String(LIMITE_POR_PAGINA));
  if (desplazamiento > 0) {
    params.set("desplazamiento", String(desplazamiento));
  }
  const {
    datos: obras,
    cargando,
    error,
  } = useApi<Obra[]>(`/api/obras?${params.toString()}`);

  /**
   * El texto del campo del anio y el ultimo valor que la URL tenia de el.
   *
   * El campo no puede ser un reflejo de la URL como los otros tres: un valor
   * que no sea un anio entero positivo NO se manda al servidor -lo rechaza con
   * 400 en vez de ignorarlo-, asi que la URL se queda sin `anio` mientras el
   * campo muestra lo que se tecleo. Si el campo se resincronizara con esa URL
   * en cada tecla, borraria lo que se acaba de escribir. `anioVisto` marca el
   * valor del que ya se sincronizo: al escribir algo no aplicable se pone al
   * dia a la vez que la URL, y la resincronizacion queda para cuando la URL
   * cambie por fuera del campo -el enlace de la navegacion, el boton de atras-,
   * que es cuando si hay que seguirla.
   */
  const anioAplicado = filtros.anio;
  const [textoAnio, setTextoAnio] = useState(anioAplicado);
  const [anioVisto, setAnioVisto] = useState(anioAplicado);
  if (anioAplicado !== anioVisto) {
    setAnioVisto(anioAplicado);
    setTextoAnio(anioAplicado);
  }

  const idTitulo = useId();
  const idGenero = useId();
  const idIpi = useId();
  const idAnio = useId();
  const idAyudaTitulo = useId();
  const idAyudaGenero = useId();
  const idAyudaIpi = useId();
  const idAyudaAnio = useId();

  const anioAplicable = textoAnio === "" || esAnioAplicable(textoAnio);

  /**
   * Pone un filtro en la URL. Cambiar cualquier filtro vuelve a la primera
   * pagina: seguir en el desplazamiento 40 de un resultado que ya es otro deja
   * la pantalla vacia por una razon que nada en pantalla explica.
   *
   * `replace` para no llenar el historial con una entrada por tecla; el boton
   * de atras sigue saliendo de la pantalla, que es lo util.
   */
  function aplicarFiltro(filtro: Filtro, valor: string) {
    setSearchParams(
      (previos) => {
        const siguientes = new URLSearchParams(previos);
        if (valor !== "") siguientes.set(filtro, valor);
        else siguientes.delete(filtro);
        siguientes.delete("desplazamiento");
        return siguientes;
      },
      { replace: true },
    );
  }

  function cambiarAnio(valor: string) {
    setTextoAnio(valor);
    const aplicable = esAnioAplicable(valor);
    if (!aplicable) setAnioVisto("");
    aplicarFiltro("anio", aplicable ? valor : "");
  }

  function limpiarFiltros() {
    setSearchParams(new URLSearchParams(), { replace: true });
  }

  /** Mueve la pagina. Aqui si se empuja al historial: "atras" vuelve a la anterior. */
  function irA(nuevoDesplazamiento: number) {
    setSearchParams((previos) => {
      const siguientes = new URLSearchParams(previos);
      if (nuevoDesplazamiento > 0) {
        siguientes.set("desplazamiento", String(nuevoDesplazamiento));
      } else {
        siguientes.delete("desplazamiento");
      }
      return siguientes;
    });
  }

  // El tipo de `useApi` promete una lista, pero `T` es una promesa y no una
  // comprobacion: un 2xx con un JSON que no es la lista prometida -el `{error}`
  // de un proxy, un backend que cambie de forma- llega hasta aqui, y
  // `obras.length` / `obras.map` tumbarian la pantalla entera. Y no basta con
  // que sea una lista: cada elemento pasa por `esObra`, que es quien conoce los
  // campos que lee la tabla. Un elemento mal formado deja el catalogo entero en
  // error a proposito, como en el listado de cargas de #29: saltarse la fila
  // mala escondería una obra -y su estado- sin decirlo, y aqui toda cifra se
  // explica hasta su origen.
  let contenido: ReactElement;
  if (cargando) {
    contenido = <Cargando texto="Cargando el catálogo…" />;
  } else if (error) {
    contenido = (
      <p className="catalogo-error" role="alert">
        No se pudo consultar el catálogo: {error.message}
      </p>
    );
  } else if (!Array.isArray(obras) || !obras.every(esObra)) {
    contenido = (
      <p className="catalogo-error" role="alert">
        El catálogo no llegó como una lista de obras legibles.
      </p>
    );
  } else {
    contenido = (
      <>
        <p className="catalogo-resumen muted">
          {textoDelResumen(obras.length, filtrosActivos)}
        </p>
        <div className="catalogo-caja">
          {obras.length === 0 ? (
            <VacioCatalogo
              conFiltros={filtrosActivos > 0}
              enPrimeraPagina={desplazamiento === 0}
              onLimpiar={limpiarFiltros}
            />
          ) : (
            <TablaCatalogo obras={obras} busqueda={searchParams.toString()} />
          )}
          {obras.length > 0 && (
            <div className="catalogo-pie">
              <span>{textoDelRango(desplazamiento, obras.length)}</span>
              <div className="catalogo-pie-botones">
                <button
                  type="button"
                  className="catalogo-pagina"
                  disabled={desplazamiento === 0}
                  onClick={() =>
                    irA(Math.max(0, desplazamiento - LIMITE_POR_PAGINA))
                  }
                >
                  Anterior
                </button>
                {/* Sin `total` en la respuesta no se puede decir "pagina 2 de
                    7": el boton se ofrece cuando la pagina vino entera -si el
                    servidor devolvio menos de `limite`, no hay mas filas- y una
                    pagina vacia despues lo dice sin afirmar que el catalogo
                    este vacio. */}
                <button
                  type="button"
                  className="catalogo-pagina"
                  disabled={obras.length < LIMITE_POR_PAGINA}
                  onClick={() => irA(desplazamiento + LIMITE_POR_PAGINA)}
                >
                  Siguiente
                </button>
              </div>
            </div>
          )}
        </div>
      </>
    );
  }

  return (
    <section className="catalogo">
      <header className="catalogo-cabecera">
        <h1>Catálogo de obras</h1>
        <p className="muted">
          El estado de la declaración vigente y su suma los calcula el servidor.
          Una declaración incompleta no es un error: bajo R-04 (RD 13.1.3) no se
          reparte nada de esa obra, el importe completo queda en reserva y nunca
          se prorratea.
        </p>
      </header>

      {/* Sin `<form>`: los filtros se aplican al escribir, asi que un submit no
          tendria nada que hacer y pulsar Enter recargaria la pagina. */}
      <div
        className="catalogo-filtros"
        role="search"
        aria-label="Filtros del catálogo"
      >
        <div className="catalogo-campos">
          <div className="catalogo-campo">
            <label htmlFor={idTitulo}>Título</label>
            <input
              id={idTitulo}
              type="text"
              autoComplete="off"
              placeholder="Buscar por título…"
              value={filtros.titulo}
              onChange={(e) => aplicarFiltro("titulo", e.target.value)}
              aria-describedby={idAyudaTitulo}
            />
            <p id={idAyudaTitulo} className="catalogo-ayuda">
              Coincidencia parcial, sin distinguir mayúsculas.
            </p>
          </div>

          <div className="catalogo-campo">
            <label htmlFor={idGenero}>Género</label>
            <input
              id={idGenero}
              type="text"
              autoComplete="off"
              placeholder="Drama"
              value={filtros.genero}
              onChange={(e) => aplicarFiltro("genero", e.target.value)}
              aria-describedby={idAyudaGenero}
            />
            <p id={idAyudaGenero} className="catalogo-ayuda">
              Coincidencia exacta, tal como se declaró.
            </p>
          </div>

          <div className="catalogo-campo">
            <label htmlFor={idIpi}>IPI de coautor</label>
            <input
              id={idIpi}
              type="text"
              autoComplete="off"
              placeholder="IPI-00000001"
              value={filtros.ipi}
              onChange={(e) => aplicarFiltro("ipi", e.target.value)}
              aria-describedby={idAyudaIpi}
            />
            <p id={idAyudaIpi} className="catalogo-ayuda">
              IPI exacto de uno de los coautores de la obra.
            </p>
          </div>

          <div className="catalogo-campo">
            <label htmlFor={idAnio}>Año</label>
            {/* Campo de texto y no `type="number"`: un `number` descarta lo que
                no parsea sin decirlo y su valor llega vacio, con lo que la
                pantalla no podria explicar por que no esta filtrando. */}
            <input
              id={idAnio}
              type="text"
              inputMode="numeric"
              autoComplete="off"
              placeholder="1991"
              value={textoAnio}
              onChange={(e) => cambiarAnio(e.target.value)}
              aria-describedby={idAyudaAnio}
            />
            <p id={idAyudaAnio} className="catalogo-ayuda">
              {anioAplicable
                ? "Año de producción exacto."
                : "No se filtra por año mientras el valor no sea un año entero positivo."}
            </p>
          </div>
        </div>

        {filtrosActivos > 0 && (
          <button
            type="button"
            className="catalogo-limpiar"
            onClick={limpiarFiltros}
          >
            Limpiar filtros
          </button>
        )}
      </div>

      {contenido}
    </section>
  );
}

/** El estado del catalogo cuando la consulta no devolvio ninguna obra. */
function VacioCatalogo({
  conFiltros,
  enPrimeraPagina,
  onLimpiar,
}: {
  conFiltros: boolean;
  enPrimeraPagina: boolean;
  onLimpiar: () => void;
}) {
  // Tres casos distintos, y el texto no puede confundirlos: con filtros puestos
  // no hay coincidencias; sin filtros y en la primera pagina el catalogo esta
  // vacio; sin filtros pero en una pagina avanzada lo unico que se sabe es que
  // ESA pagina no trae obras -decir que el catalogo esta vacio ahi seria falso,
  // porque el resto de las paginas no se consulto-.
  if (conFiltros) {
    return (
      <div className="catalogo-vacio">
        <p>Ninguna obra coincide con los filtros.</p>
        <p className="muted">
          Prueba con términos menos específicos o{" "}
          <button type="button" className="enlace" onClick={onLimpiar}>
            limpia los filtros
          </button>
          .
        </p>
      </div>
    );
  }
  return (
    <div className="catalogo-vacio">
      <p>
        {enPrimeraPagina
          ? "El catálogo no tiene obras registradas."
          : "Esta página del catálogo no trae obras."}
      </p>
    </div>
  );
}

/**
 * La tabla del catalogo, con las columnas del mockup.
 *
 * Cada fila enlaza al detalle de SU obra: es lo que el issue #30 pide del
 * catalogo maestro ("cada fila navega al detalle de la obra") y el destino que
 * D-007 fijo para esa vista. Cuando esta tabla se escribio el detalle no
 * existia -era el paso 6 del plan- y la nota decia que ninguna fila navegaba;
 * el enlace se anadio al aterrizar esa pantalla, porque la razon que lo
 * justificaba -un enlace que no lleva a ninguna parte es peor que su ausencia-
 * dejo de ser cierta en cuanto la ruta paso a tener componente. Lo que sigue
 * sin dibujarse es el boton de alta del mockup: `POST /obras` existe, pero el
 * alta no esta en el alcance de #30 (D-010).
 *
 * El enlace lleva ademas la direccion del catalogo -`busqueda`, la query de
 * esta pantalla- para que la ficha sepa a donde devolver al administrador. Se
 * entrega la direccion tal como esta, con los filtros y la pagina que tenga: es
 * la busqueda que esa persona acaba de componer, y devolverla distinta seria
 * devolverla a medias.
 */
function TablaCatalogo({
  obras,
  busqueda,
}: {
  obras: readonly Obra[];
  busqueda: string;
}) {
  return (
    <table className="tabla-catalogo" aria-label="Catálogo de obras">
      <thead>
        <tr>
          <th scope="col">Título</th>
          <th scope="col">Género</th>
          <th scope="col">Año</th>
          <th scope="col">Tipo</th>
          <th scope="col">IDA</th>
          <th scope="col">Declaración</th>
        </tr>
      </thead>
      <tbody>
        {obras.map((obra) => (
          <tr key={obra.id}>
            <td className="catalogo-titulo">
              {/* El enlace va en el titulo y no en la fila entera. Un `<a>` no
                  puede ir entre `<tbody>` y `<tr>` sin romper el modelo de
                  contenido de la tabla -un `<tbody>` contiene filas-, y una
                  capa que cubriera la fila entera se comeria el clic y la
                  seleccion de texto de las demas celdas, donde el IDA es un
                  identificador que alguien puede necesitar copiar. El titulo,
                  ademas, es el nombre accesible del enlace, asi que con un
                  enlace por fila se sabe cual es cual.
                  El id se codifica porque el contrato lo declara opaco y
                  asignado FUERA de este sistema: no hay forma de saber que
                  caracteres trae. `useParams` lo entrega decodificado y
                  `DetalleObra` lo vuelve a codificar al consultar, asi que la
                  obra que se consulta es la que esta fila nombra. */}
              <Link
                to={`/catalogo/${encodeURIComponent(obra.id)}`}
                state={{ [CLAVE_DE_VUELTA_AL_CATALOGO]: busqueda }}
              >
                {obra.titulo}
              </Link>
            </td>
            <td>{obra.genero}</td>
            {/* El anio va sin agrupar miles, por eso no pasa por
                `formatearEntero`: "1.991" no es un anio. */}
            <td>{obra.anio}</td>
            {/* El valor del enum, tal cual: es la clasificacion del reglamento
                (RD 9.1.1) y no hay traduccion de la casa que no invente. */}
            <td>{obra.tipo}</td>
            <td className="catalogo-identificador">
              {/* El contrato declara `ida` opcional y de texto, con la cadena
                  vacia como "no se conoce": el guion dice eso y no otra cosa. */}
              {obra.ida !== undefined && obra.ida !== "" ? obra.ida : "—"}
            </td>
            <td>
              <div className="catalogo-declaracion">
                <EtiquetaDeDeclaracion obra={obra} />
                {/* La suma va SIEMPRE, tambien sin declaracion: es lo que el
                    backend manda en `suma_porcentajes` y ahi vale cero. Quien
                    distingue los dos casos es la etiqueta, no un hueco. */}
                <span className="catalogo-suma">
                  {formatearPorcentaje(obra.suma_porcentajes)}
                </span>
              </div>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
