import {
  useEffect,
  useId,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent,
  type MutableRefObject,
  type ReactElement,
} from "react";
import { Link, useParams } from "react-router-dom";
import { api } from "../api";
import Cargando from "../Cargando";
import { formatearInstante } from "../tablero/formato";
import { DEBOUNCE_TECLEO_MS, useApi } from "../useApi";
import { CLAVE_DE_VUELTA_AL_CATALOGO, useVueltaAlCatalogo } from "./Catalogo";
import {
  avisoDelRechazo,
  comoSeNombraLaFila,
  falloDelGuardado,
  filasDePartida,
  nuevaFila,
  porcentajeDeTexto,
  tituloDelRechazo,
  type FilaDeReparto,
  type ResultadoDelGuardado,
} from "./borrador";
import {
  avisoDeVersion,
  estadoDelBorrador,
  formatearPorcentaje,
  puedeGuardarBorrador,
  totalDeclarado,
  type AvisoDeVersion,
  type EstadoBorrador,
  type MotivoDeDuda,
} from "./declaracion";
import { ObraAusente } from "./DetalleObra";
import { EtiquetaDeEstado } from "./EtiquetaDeDeclaracion";
import {
  esTitular,
  esVersionDeclaracion,
  puedeSerParte,
  type Obra,
  type Parte,
  type Titular,
  type VersionDeclaracion,
} from "./tipos";
import { conciliarConElHistorial, useObra } from "./useObra";

/**
 * Cuantos titulares se piden por pagina del padron. El servidor aplica 100 si
 * se omite y no admite mas de 500; se manda explicito para que la pagina que se
 * ve sea la que dice la pantalla, igual que en el catalogo de obras.
 */
const LIMITE_PADRON = 50;

/**
 * La fila que tiene que recibir el foco en cuanto el render la pinte.
 *
 * Es la clave de la fila destino, y `FilasDelReparto` la consume UNA sola vez: la
 * deja en `null` al usarla. Va como ref y no como estado a proposito -el foco no
 * se pinta-, porque como estado costaria un render de mas, el foco llegaria un
 * render tarde y volveria a saltar en cada tecla, que tambien cambia `filas`. Y
 * se usa esto en vez de `autoFocus` porque `autoFocus` dispara al MONTAR: le
 * quitaria el foco a la pantalla en cuanto se abre, cuando lo que hay que
 * resolver es no perderlo despues de quitar o anadir una fila.
 */
type FocoPendiente = MutableRefObject<string | null>;

/**
 * El texto y la explicacion del borrador en cada uno de sus cuatro estados.
 *
 * Son estados del BORRADOR, que no existen para el servidor: los de una
 * declaracion guardada son dos -`completa` e `incompleta`- y los asigna el
 * backend. Por eso no se pinta aqui `EtiquetaDeEstado`, que es la etiqueta del
 * estado que el backend calculo, y por eso ninguna de estas frases dice lo que
 * la version guardada vaya a quedar siendo.
 */
const ESTADO_DEL_BORRADOR: Record<
  EstadoBorrador,
  { texto: string; explicacion: string }
> = {
  vacia: {
    texto: "Sin nada declarado",
    explicacion:
      "No hay ningún porcentaje escrito todavía. El servidor rechaza una declaración sin partes, así que esta pantalla no ofrece guardar mientras el borrador esté así.",
  },
  completa: {
    texto: "Completa",
    explicacion:
      "El borrador suma 100. La comparación da por bueno el ruido de la coma flotante, para que una suma exacta en aritmética decimal no se lea como incompleta. Si el servidor lo acepta, la versión quedará completa.",
  },
  incompleta: {
    texto: "Incompleta",
    explicacion:
      "Un borrador por debajo de 100 no es un error: bajo R-04 (RD 13.1.3) la declaración se guarda igual, no se reparte nada de esa obra y el importe completo queda en reserva, nunca se prorratea. Se puede guardar así.",
  },
  excedida: {
    texto: "Pasa de 100",
    explicacion:
      "El total pasa de 100, y el servidor rechaza con un 400 toda declaración cuya suma supere 100. Por eso esta pantalla no ofrece guardar mientras el total esté por encima: baja el porcentaje de alguna fila.",
  },
};

/**
 * El editor del reparto (issue #30, paso 8; S2, S3 y S4).
 *
 * La pantalla que ABRE una version de la Declaracion de Obra:
 * `PUT /obras/{id}/declaracion` con un array de partes. No edita una version
 * pasada -el contrato no tiene forma de hacerlo-, y por eso todo el rato dice
 * que lo que va a pasar es que se cierre la vigente y se abra una nueva.
 *
 * Aparte del numero de version -que es el defecto que D-009 cierra-, la regla
 * que ordena esta pantalla es de donde sale cada cifra:
 *
 * - el ESTADO y la SUMA de la version guardada los calcula el servidor, y no se
 *   ven aqui: se ven en el catalogo, en el detalle y en el historial. Lo que hay
 *   aqui es el borrador, que no tiene estado de backend porque no existe para el
 *   servidor, y va rotulado como lo que es -calculado en esta pantalla-;
 * - los TITULARES elegibles salen de `GET /titulares`, que devuelve el padron
 *   entero a proposito: quien no puede ser parte se explica en vez de faltar;
 * - el MENSAJE de un rechazo es el del backend, entero y sin reescribir: nombra
 *   la regla que se incumplio, y solo el backend sabe cual de ellas ha sido.
 *
 * El bloqueo del guardado SI es del cliente, y es el unico que hay:
 * `puedeGuardarBorrador` frena dos cuerpos -uno que no declara nada, y uno cuya
 * suma pasa de 100-. Que el cliente frene no es que la regla sea suya: la regla
 * es del servidor, que la aplica aunque el cliente no la mire, y si el total se
 * cuela -una suma que se pasa por menos que la tolerancia, un porcentaje con mas
 * de cuatro decimales- el 400 del backend es lo que se enseña.
 *
 * Este bloqueo NO es la lista de todo lo que el backend rechaza con 400, y el
 * comentario que lo decia asi era falso: `repertorio.NuevaDeclaracion` rechaza
 * ademas un IPI ausente, un porcentaje no positivo, mas de cuatro decimales y un
 * titular repetido. La lista de los dos cuerpos que el cliente SI bloquea vive
 * en un solo sitio, `declaracion.ts` (`puedeGuardarBorrador`,
 * `estadoDelBorrador`); repetirla aqui daria dos fuentes para el mismo hecho, y
 * la que se quede sin actualizar es la que miente.
 */
export default function EditorReparto() {
  const { id = "" } = useParams();
  // La vuelta al catalogo, resuelta por la misma funcion que usan el detalle y
  // el historial: las tres salidas de la pantalla -la ficha, la obra ausente y
  // el aviso de que la obra se fue al guardar- salen de un solo sitio.
  const { busqueda, destino } = useVueltaAlCatalogo();
  // La obra y sus desenlaces, de `useObra` (item 13): la peticion, el 404 como
  // caso propio -el servidor diciendo que con ese identificador no hay ninguna
  // obra, y por tanto que no hay reparto que declarar- y la guarda de forma
  // viven alli, donde tambien las leen el detalle y el historial.
  const estado = useObra(id);

  if (estado.estado === "cargando")
    return <Cargando texto="Cargando la obra…" />;

  if (estado.estado === "ausente") {
    return (
      <ObraAusente
        id={estado.id}
        volver={destino}
        className="editor-reparto"
        explicacion=", así que no hay ninguna declaración que abrir. El reparto de una obra se declara sobre la obra: sin ella no hay nada que editar."
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
    // comprobacion: un 2xx con otra forma llegaria hasta aqui y `obra.titulo` o
    // `obra.id` -lo que esta pantalla lee- la tumbarian. La guarda es la del
    // tipo, la misma que usan el listado, el detalle y el historial.
    return (
      <p className="catalogo-error" role="alert">
        La obra no llegó con la forma que el contrato promete para una obra.
      </p>
    );
  }

  return (
    <EditorDeLaObra obra={estado.obra} busqueda={busqueda} destino={destino} />
  );
}

/**
 * La obra ya legible: primero su declaracion vigente, y con ella el borrador.
 *
 * El historial se lee UNA vez y de aqui sale todo lo que depende de el: con que
 * reparto empieza el borrador y que version se cerrara. Se lee antes de montar
 * el formulario a proposito: asi el borrador se construye con el reparto
 * vigente ya en la mano en vez de sembrarse despues -que sobrescribiria lo que
 * se hubiera tecleado mientras llegaba la respuesta-.
 *
 * Que el historial falle NO impide editar: el borrador empieza sin filas, el
 * aviso dice que no se sabe que version se cerrara, y el padron sigue estando
 * para armar un reparto. Lo que no se hace es inventar el reparto que no se pudo
 * leer.
 */
function EditorDeLaObra({
  obra,
  busqueda,
  destino,
}: {
  obra: Obra;
  busqueda: string;
  destino: string;
}) {
  const {
    datos: historial,
    cargando,
    error,
  } = useApi<VersionDeclaracion[]>(
    `/api/obras/${encodeURIComponent(obra.id)}/declaracion/historial`,
  );

  if (cargando) return <Cargando texto="Cargando la declaración vigente…" />;

  // El historial de una obra que existe llega como lista vacia -nunca 404-, asi
  // que un error aqui es un fallo de lectura y no "no hay declaracion": lo que
  // se lee con exito es lo que decide entre los dos casos. Una lista mal formada
  // cuenta como no leida, por la misma razon por la que el historial del detalle
  // no pinta versiones a medias.
  const legible =
    !error && Array.isArray(historial) && historial.every(esVersionDeclaracion);

  return (
    <FormularioDeReparto
      obra={obra}
      // `null` quiere decir "no se pudo leer", que es lo unico que el aviso
      // necesita distinguir de "no hay ninguna declaracion".
      historial={legible ? historial : null}
      mensajeDelHistorial={
        error
          ? error.message
          : "la respuesta no llegó como una lista de versiones"
      }
      busqueda={busqueda}
      destino={destino}
    />
  );
}

/**
 * Enter en un campo de texto NO guarda.
 *
 * Un `<form>` con un boton de envio convierte Enter dentro de un `<input>` en un
 * envio implicito, y este formulario guarda: pulsar Enter a media edicion cierra
 * la version abierta y abre otra con lo que hubiera escrito en ese instante. Con
 * el porcentaje a medio teclear -el "7" de un "70"- la obra pasa a sumar 7 y bajo
 * R-04 deja de repartir nada. Medido en navegador real, con precondicion y control
 * negativo: teclear un digito no manda nada, y Enter mandaba un `PUT`.
 *
 * Guardar sigue siendo el boton, y por eso el Enter se CANCELA en vez de
 * confirmarse: convertirlo en una confirmacion anadiria un paso -y un dialogo- al
 * camino legitimo, que hoy es un clic. La condicion es `INPUT` de tipo `text` y no
 * "cualquier Enter": pulsar Enter con el foco en el boton tiene que seguir
 * guardando, que es el comportamiento que espera quien tabula hasta el y pulsa.
 */
function alPulsarTecla(evento: KeyboardEvent<HTMLFormElement>) {
  const destino = evento.target as HTMLElement;
  if (
    evento.key === "Enter" &&
    destino.tagName === "INPUT" &&
    (destino as HTMLInputElement).type === "text"
  ) {
    evento.preventDefault();
  }
}

/**
 * El borrador, el padron y el guardado.
 *
 * Se monta cuando el historial ya se resolvio -bien o mal-, y no se vuelve a
 * montar: el borrador es del que esta editando y perderlo por una respuesta que
 * llega tarde seria justo lo que el mockup pide evitar. Por eso la respuesta del
 * guardado no se lee del historial sino del `PUT`, que es lo que deja el aviso
 * de la version al dia sin volver a pedir nada.
 */
function FormularioDeReparto({
  obra,
  historial,
  mensajeDelHistorial,
  busqueda,
  destino,
}: {
  obra: Obra;
  historial: VersionDeclaracion[] | null;
  mensajeDelHistorial: string;
  busqueda: string;
  destino: string;
}) {
  const [filas, setFilas] = useState<FilaDeReparto[]>(() =>
    historial === null ? [] : filasDePartida(historial),
  );
  const [resultado, setResultado] = useState<ResultadoDelGuardado | null>(null);
  // La version que el servidor contesto que abrio. Es lo que mantiene el aviso
  // al dia despues de guardar, sin releer el historial ni adivinar el numero.
  const [versionGuardada, setVersionGuardada] = useState<number | null>(null);
  // La duda que un guardado dejo pendiente, en su PROPIO estado y no derivada
  // del `resultado` del render: `resultado` es transitorio -`guardar()` lo pone a
  // `null` al empezar- y deducirla de ahi hacia que el guardado siguiente
  // devolviera el numero viejo al aviso, incluido el hueco en vuelo. Un 4xx no la
  // toca; la limpia un 200 legible. Ver `MotivoDeDuda`.
  const [dudaPendiente, setDudaPendiente] = useState<MotivoDeDuda | null>(null);
  const [guardando, setGuardando] = useState(false);
  // `disabled` llega en el siguiente render; el ref corta tambien un segundo
  // clic que entre antes. Un segundo `PUT` con el mismo reparto abriria una
  // version de mas.
  const enVuelo = useRef(false);
  // La fila que tiene que recibir el foco en cuanto el borrador cambie. La
  // escriben las dos acciones que cambian la rejilla y la consume
  // `FilasDelReparto`. Ver `FocoPendiente`.
  const focoPendiente = useRef<string | null>(null);

  const total = totalDeclarado(
    filas.map((fila) => ({ porcentaje: porcentajeDeTexto(fila.porcentaje) })),
  );
  const estado = estadoDelBorrador(total);
  // Filas sin un numero legible en el porcentaje. No es una regla de negocio
  // -la del total la aplica `puedeGuardarBorrador`-, es la forma del cuerpo:
  // cada parte lleva un numero, y esta pantalla no manda `null` donde el
  // contrato pide una cifra ni convierte en 0 lo que nadie escribio.
  const filasSinNumero = filas.filter(
    (fila) => !Number.isFinite(porcentajeDeTexto(fila.porcentaje)),
  ).length;
  // La obra dice cual es su version vigente y el historial cual tiene abierta:
  // son dos lecturas del mismo hecho, y si no coinciden -una declaracion se
  // guardo entre las dos peticiones, o el historial no trae ninguna abierta-
  // guardar cerraria una version que `GET /obras/{id}` no esta sosteniendo. Es
  // la MISMA comprobacion que hace el detalle, y desde el paso 16 es el MISMO
  // codigo: `conciliarConElHistorial`, en `useObra.ts` (PM-6 del plan). Antes de
  // este paso cada pantalla tenia su copia -aqui un `filter` con `versionAbierta`
  // y en el detalle otro-, que es el defecto que el item 13 existe para cerrar.
  //
  // Se compara solo cuando el historial se pudo leer. `null` es "no se pudo
  // leer", y ese caso ya tiene su aviso -y guardar sin haber leido el historial
  // es justo lo que el aviso advierte-: prohibirlo aqui seria cambiar una
  // advertencia por una puerta cerrada sin haberselo pedido a nadie. Por eso el
  // historial no leido no entra en la conciliacion: entra en el `null` del
  // diagnostico de abajo.
  const conciliacion =
    historial === null ? null : conciliarConElHistorial(obra, historial);
  // El diagnostico del descuadre, y `null` cuando no lo hay -ni cuando las dos
  // lecturas cuadran ni cuando el historial no se leyo-. Es la unica forma de
  // esa frase en la pantalla: el texto dice el motivo verdadero en vez de
  // reescribirlo, que es como se acaba discrepando del diagnostico.
  const descuadre =
    conciliacion !== null && conciliacion.estado === "descuadrado"
      ? conciliacion.mensaje
      : null;

  const puedeGuardar =
    puedeGuardarBorrador(estado) && filasSinNumero === 0 && descuadre === null;
  // Las tres fuentes: el historial, la version que el `PUT` devolvio y la duda
  // que un guardado dejo pendiente. La duda manda, y por eso no basta con las dos
  // primeras -ver `avisoDeVersion`-.
  const aviso = avisoDeVersion(
    historial,
    versionGuardada,
    dudaPendiente,
    // Un 4xx no limpia la duda, asi que cuando el panel de abajo cuenta un
    // rechazo y hay una duda pendiente, los dos textos son de guardados
    // distintos: el rechazo es posterior al guardado que dejo la duda.
    resultado?.tipo === "rechazada",
  );

  function agregarTitular(titular: Titular) {
    // La fila nace SIN porcentaje: la cifra la escribe quien declara, y un
    // valor de relleno acabaria guardado como si se hubiera declarado. Lo que si
    // guarda es el NOMBRE, que el `Titular` del padron ya traia y hasta ahora se
    // tiraba: es el unico momento en que esta pantalla lo tiene, y sin guardarlo
    // aqui la columna solo puede decir el `titular_id`.
    const fila = nuevaFila(titular.id, titular.ipi, "", titular.nombre);
    // El foco va a la fila nueva. El boton que se pulso esta en el padron -al
    // final de la pantalla- y ademas desaparece en cuanto el titular entra en el
    // reparto, asi que sin esto el foco se queda en la nada: el `Tab` siguiente
    // vuelve al principio de la pagina, que es justo de donde se viene.
    focoPendiente.current = fila.clave;
    setFilas((previas) => [...previas, fila]);
  }

  function quitarFila(clave: string) {
    const indice = filas.findIndex((fila) => fila.clave === clave);
    const restantes = filas.filter((fila) => fila.clave !== clave);
    // Quien ocupa el hueco: la fila que se corre a ese indice y, si la quitada
    // era la ultima, la que pasa a ser la ultima. Se calcula sobre el array que
    // se va a guardar -y no sobre `filas`- para que la clave que se pide sea
    // siempre la de una fila que existe; con la rejilla vacia no queda ningun
    // control al que llevar el foco y no se promete ninguno.
    const relevo = restantes[indice] ?? restantes[restantes.length - 1];
    focoPendiente.current = relevo ? relevo.clave : null;
    setFilas(restantes);
  }

  function cambiarFila(clave: string, cambio: Partial<FilaDeReparto>) {
    setFilas((previas) =>
      previas.map((fila) =>
        fila.clave === clave ? { ...fila, ...cambio } : fila,
      ),
    );
  }

  async function guardar(evento: FormEvent) {
    evento.preventDefault();
    if (enVuelo.current || !puedeGuardar) return;
    enVuelo.current = true;
    setGuardando(true);
    setResultado(null);

    // El cuerpo es el array de partes que declara el contrato: el mismo titulo,
    // el mismo IPI y el porcentaje leido del campo, SIN redondear. Redondear
    // aqui declararia una cifra distinta de la escrita, y mas de cuatro
    // decimales lo rechaza el servidor con su mensaje.
    const partes: Parte[] = filas.map((fila) => ({
      titular_id: fila.titularId,
      ipi: fila.ipi,
      porcentaje: porcentajeDeTexto(fila.porcentaje),
    }));

    try {
      const cuerpo = await api(
        `/api/obras/${encodeURIComponent(obra.id)}/declaracion`,
        { method: "PUT", body: JSON.stringify(partes) },
      );
      // Frontera sin validar, como las de la ingesta: el 200 trae una
      // `VersionDeclaracion` segun api/openapi.yaml, y `T` es una promesa, no
      // una comprobacion. Se revisa antes de afirmar su numero.
      if (!esVersionDeclaracion(cuerpo)) {
        setResultado({ tipo: "guardadaSinLeer" });
        // El 200 prueba que una version se abrio; lo que no se puede leer es
        // cual. La duda queda PENDIENTE hasta que otro guardado conteste 200 con
        // un cuerpo legible.
        setDudaPendiente("guardadoSinLeer");
        return;
      }
      setResultado({ tipo: "guardada", version: cuerpo });
      setVersionGuardada(cuerpo.version);
      // El unico desenlace que limpia la duda: es el unico que dice cual se
      // abrio. La version vieja de `versionGuardada` no se borra -sigue siendo
      // el ultimo dato bueno-, pero deja de mandar mientras haya duda.
      setDudaPendiente(null);
    } catch (error) {
      const fallo = falloDelGuardado(error);
      setResultado(fallo);
      // Un 4xx (`rechazada`) NO entra en ninguna rama y por eso no limpia nada:
      // prueba que ESA peticion no abrio ninguna version, y no dice nada del
      // guardado anterior que quedo en duda.
      if (fallo.tipo === "guardadaSinLeer") setDudaPendiente("guardadoSinLeer");
      if (fallo.tipo === "incierta") setDudaPendiente("guardadoIncierto");
    } finally {
      enVuelo.current = false;
      setGuardando(false);
    }
  }

  return (
    <section className="editor-reparto">
      <p className="detalle-volver">
        <Link
          to={`/catalogo/${encodeURIComponent(obra.id)}`}
          state={{ [CLAVE_DE_VUELTA_AL_CATALOGO]: busqueda }}
        >
          ← Volver a la obra
        </Link>
      </p>

      <header className="detalle-cabecera">
        <h1>Declaración de la obra</h1>
        <p className="muted detalle-nota">
          {obra.titulo} · {obra.id}
        </p>
        {/* De que va esta pantalla, sin prometer nada que no se cumpla: los
            porcentajes de reparto solo salen de la Declaracion de Obra (R-03), y
            guardar aqui no corrige la version vigente, abre una nueva. */}
        <p className="muted detalle-nota">
          Los porcentajes de reparto solo salen de la Declaración de Obra
          (R-03). Guardar aquí no corrige la versión vigente: la cierra y abre
          una nueva con el reparto que quede escrito.
        </p>
      </header>

      <AvisoDeVersionVisible aviso={aviso} mensaje={mensajeDelHistorial} />

      {resultado && (
        <PanelDelGuardado
          resultado={resultado}
          obraId={obra.id}
          busqueda={busqueda}
          volver={destino}
        />
      )}

      <form
        className="editor-formulario"
        onSubmit={(e) => void guardar(e)}
        onKeyDown={alPulsarTecla}
      >
        <section className="editor-borrador">
          <h2>Borrador del reparto</h2>
          <p className="muted detalle-nota">
            El estado de la declaración guardada lo calcula el servidor; lo de
            aquí es el borrador, que todavía no existe para el servidor.
          </p>
          {/* `role="status"` en las dos cifras y en los avisos, siguiendo el
              patron de `PanelDelGuardado`: son nodos VIVOS, y lo que cambia en
              ellos -el total al teclear, el estado del borrador, el motivo por
              el que no se ofrece guardar, el aviso de la version- es justo lo
              que hay que anunciar sin mover el foco. No se anade `aria-live`
              por encima ni se mete un `role="alert"` dentro: las dos cosas
              anunciarian el mismo cambio dos veces. */}
          <dl className="detalle-ficha">
            <div>
              <dt>Total del borrador (calculado en esta pantalla)</dt>
              <dd className="detalle-cifra" role="status">
                {formatearPorcentaje(total)}
              </dd>
            </div>
            <div>
              <dt>Estado del borrador</dt>
              <dd className="detalle-cifra" role="status">
                {ESTADO_DEL_BORRADOR[estado].texto}
              </dd>
            </div>
          </dl>
          <p className="muted detalle-nota">
            {ESTADO_DEL_BORRADOR[estado].explicacion}
          </p>
          <p className="muted detalle-nota">
            El total se muestra con 4 decimales, la precisión de la columna, y
            el estado del borrador se decide sobre la suma exacta de lo escrito,
            sin redondearla. El total y el estado de la versión guardada los
            calcula el servidor: son los que se ven en el catálogo y en el
            historial.
          </p>
        </section>

        <FilasDelReparto
          filas={filas}
          focoPendiente={focoPendiente}
          onCambiar={cambiarFila}
          onQuitar={quitarFila}
        />

        <div className="editor-acciones">
          {/* La unica via de guardado, y la que espera quien opera. El Enter de
              un campo de texto lo cancela `alPulsarTecla`; el de este boton no,
              porque ahi el envio implicito es lo que se quiere. */}
          <button
            type="submit"
            className="boton-primario"
            disabled={!puedeGuardar || guardando}
          >
            {guardando ? "Guardando…" : "Guardar la declaración"}
          </button>
          {filasSinNumero > 0 && (
            <p className="editor-aviso" role="status">
              {filasSinNumero === 1
                ? "Hay 1 fila sin un porcentaje que se lea como número."
                : `Hay ${filasSinNumero} filas sin un porcentaje que se lea como número.`}{" "}
              El cuerpo que espera el servidor lleva un número en cada parte,
              así que la pantalla no envía nada mientras falte.
            </p>
          )}
          {filasSinNumero === 0 && descuadre === null && !puedeGuardar && (
            <p className="editor-aviso" role="status">
              {estado === "vacia"
                ? "Añade al menos una parte desde el padrón para poder guardar."
                : "El total pasa de 100, y el servidor rechaza esa suma con un 400."}
            </p>
          )}
          {/* El tercer motivo por el que no se ofrece guardar, y el unico que
              NO es del borrador: el historial no cuadra con la version vigente
              que declara la obra. Va aparte de los dos de arriba para que el
              texto diga siempre el motivo verdadero -sin esta guarda, un
              historial que no cuadra saldria como "el total pasa de 100"-.
              La primera frase es el diagnostico de `useObra` -el unico sitio
              donde se decide que las dos lecturas no cuadran-, y la segunda es
              la consecuencia de ESTA pantalla: la que lee se niega a pintar, la
              que escribe se niega a escribir. */}
          {descuadre !== null && (
            <p className="editor-aviso" role="status">
              {descuadre} Mientras las dos lecturas no cuadren, esta pantalla no
              ofrece guardar: guardaría cerrando una versión que el sistema no
              está sosteniendo. Recarga la pantalla y vuelve a mirar el
              historial.
            </p>
          )}
        </div>
      </form>

      <PadronDelEditor
        enElReparto={filas.map((fila) => fila.titularId)}
        onAgregar={agregarTitular}
      />
    </section>
  );
}

/**
 * El aviso de la version, en sus tres formas. Ver `AvisoDeVersion`.
 *
 * Va arriba, antes del formulario, porque es lo que hay que entender ANTES de
 * tocar nada: quien crea que esta corrigiendo el reparto vigente va a descubrir
 * lo contrario despues de guardar.
 */
function AvisoDeVersionVisible({
  aviso,
  mensaje,
}: {
  aviso: AvisoDeVersion;
  mensaje: string;
}) {
  if (aviso.tipo === "cierraYabre") {
    return (
      <p className="editor-aviso-version" role="status">
        Al guardar, el servidor cerrará la versión {aviso.seCierra} y abrirá la
        versión {aviso.seAbre} con este reparto. La versión {aviso.seCierra} no
        se borra ni se modifica: queda en el historial con los porcentajes que
        regían hasta ahora.
      </p>
    );
  }

  if (aviso.tipo === "abreLaPrimera") {
    return (
      <p className="editor-aviso-version" role="status">
        Esta obra no tiene ninguna declaración todavía, así que al guardar se
        abrirá la primera versión. No hay ninguna versión anterior que cerrar.
      </p>
    );
  }

  // Sin numeros. Lo unico que se afirma es la consecuencia, que se sostiene en
  // todos los casos: no hay forma de editar una version pasada, solo de abrir
  // una nueva, asi que la anterior -si la hay- no se borra ni se modifica.
  const consecuencia =
    "Lo que sí se sabe: la versión anterior, si la hay, no se borra ni se modifica, y queda en el historial con los porcentajes que regían hasta ahora.";
  // Si el borrador no se pudo sembrar, se sigue diciendo: es un hecho del
  // historial, no del guardado, y ninguno de los motivos de abajo lo cambia.
  const sinBorrador = aviso.sinBorrador
    ? " El borrador tampoco se ha podido cargar con el reparto vigente."
    : "";
  // Cuando en la misma pantalla hay ademas un rechazo, los dos textos hablan de
  // guardados DISTINTOS -el aviso, del que dejo la duda; el panel de abajo, de
  // otro posterior- y hay que decirlo: sin esta frase el aviso se lee como si
  // describiera el guardado que acaba de rechazarse, y afirmaria entonces una
  // version abierta sobre un guardado que no abrio ninguna. Lo que se afirma del
  // rechazo es lo unico que un 4xx prueba, y esta escrito con el mismo criterio
  // que `avisoDelRechazo`.
  const rechazoPosterior = aviso.hayRechazoPosterior
    ? " El rechazo que cuenta el panel de abajo es de otro guardado, posterior: prueba que ESA petición no abrió ninguna versión, no lo que hizo la que dejó esta duda, así que no la resuelve."
    : "";

  if (aviso.porque === "historialNoLeido") {
    return (
      <p className="editor-aviso-version" role="status">
        No se pudo leer el historial de esta declaración ({mensaje}), así que
        esta pantalla no puede decir qué versión se cerrará ni con qué número se
        abre la nueva. {consecuencia}
        {sinBorrador}
      </p>
    );
  }

  // El 200 con un cuerpo que no se pudo leer: el servidor SI abrio una version
  // -un 200 es lo que el contrato promete cuando la abre-, pero no dijo cual. Las
  // dos formas de que el cuerpo no sirva -que no se pueda ni leer y que se lea
  // sin la forma del contrato- llegan al mismo desenlace, `guardadaSinLeer`. El
  // aviso no puede caer al historial en memoria: alli la version que se cerraria
  // es justo la que el 200 acaba de cerrar.
  if (aviso.porque === "guardadoSinLeer") {
    return (
      <p className="editor-aviso-version" role="status">
        El servidor contestó sin error, así que una versión se abrió, pero la
        respuesta no llegó con la forma del contrato y esta pantalla no puede
        decir cuál es.{rechazoPosterior} Compruébalo en el historial antes de
        volver a guardar: guardar otra vez abre una versión más. {consecuencia}
        {sinBorrador}
      </p>
    );
  }

  // Un 5xx o una respuesta perdida: no se sabe si una version quedo abierta, y
  // sin saberlo no se puede decir cual se cerraria. El panel de abajo da el
  // paso siguiente; aqui solo se deja de afirmar el numero.
  if (aviso.porque === "guardadoIncierto") {
    return (
      <p className="editor-aviso-version" role="status">
        No se sabe si el guardado abrió una versión: un fallo al confirmar es
        indistinguible de una respuesta que se perdió, así que esta pantalla no
        puede decir qué versión se cerrará ni con qué número se abre la nueva.
        {rechazoPosterior} {consecuencia}
        {sinBorrador}
      </p>
    );
  }

  return (
    <p className="editor-aviso-version">
      Con lo que el servidor devolvió en el historial, esta pantalla no puede
      decir qué versión se cerrará ni con qué número se abre la nueva.{" "}
      {consecuencia}
      {sinBorrador}
    </p>
  );
}

/**
 * Las filas del borrador: el titular que ya se eligio, su IPI y su porcentaje.
 *
 * No hay boton de "anadir fila" en blanco, y no es un olvido: una parte sin
 * `titular_id` no es una parte -el backend la rechaza-, asi que la fila nace de
 * elegir un titular en el padron, que es el unico sitio donde se sabe cual es su
 * identificador y su IPI.
 *
 * El porcentaje es un campo de texto y no `type="number"`, por la misma razon
 * que el anio del catalogo: un `number` descarta en silencio lo que no parsea
 * -un "33,5" se leeria vacio- y la pantalla no podria explicar por que su fila
 * no cuenta. Aqui lo que se teclea se lee tal cual, y la coma se admite.
 */
function FilasDelReparto({
  filas,
  focoPendiente,
  onCambiar,
  onQuitar,
}: {
  filas: readonly FilaDeReparto[];
  focoPendiente: FocoPendiente;
  onCambiar: (clave: string, cambio: Partial<FilaDeReparto>) => void;
  onQuitar: (clave: string) => void;
}) {
  // Los nodos de las filas, por su clave. Hacen falta los refs y no un
  // `querySelector` por texto o por `data-*`: la clave es lo unico que identifica
  // la fila de forma estable -el `titularId` no sirve, la misma persona puede
  // estar en dos filas mientras se edita-.
  const refsDeFilas = useRef(new Map<string, HTMLTableRowElement>());

  // El salto de foco, UNA sola vez por cambio. `filas` cambia tambien al
  // teclear, asi que este efecto corre muchas veces; lo que lo limita a un solo
  // salto es vaciar `focoPendiente` en la primera pasada. El destino es el primer
  // control de la fila -su primer campo-, que es lo que "la fila que ocupa el
  // hueco" significa para quien llega tabulando. Si no hay fila con esa clave
  // -la rejilla se quedo vacia- no se enfoca nada: no hay destino que prometer.
  useEffect(() => {
    const clave = focoPendiente.current;
    if (clave === null) return;
    focoPendiente.current = null;
    refsDeFilas.current
      .get(clave)
      ?.querySelector<HTMLElement>("input, button")
      ?.focus();
  }, [filas, focoPendiente]);

  if (filas.length === 0) {
    return (
      <div className="catalogo-vacio">
        <p>El borrador no tiene ninguna parte.</p>
        <p className="muted">
          Añade titulares desde el padrón de abajo: el reparto se declara sobre
          quien figura en él.
        </p>
      </div>
    );
  }

  return (
    <div className="catalogo-caja">
      <table
        className="tabla-partes editor-filas"
        aria-label="Borrador del reparto"
      >
        <thead>
          <tr>
            <th scope="col">Titular</th>
            <th scope="col">IPI</th>
            <th scope="col" className="tabla-partes-numero">
              Porcentaje
            </th>
            <th scope="col" aria-label="Acciones" />
          </tr>
        </thead>
        <tbody>
          {filas.map((fila) => (
            <tr
              key={fila.clave}
              ref={(nodo) => {
                // Al desmontarse la fila React llama con `null`, y esa clave hay
                // que borrarla: si no, el mapa se queda con el nodo de una fila
                // que ya no esta y el foco podria irse a un elemento muerto.
                if (nodo === null) refsDeFilas.current.delete(fila.clave);
                else refsDeFilas.current.set(fila.clave, nodo);
              }}
            >
              <td>
                {/* El nombre, cuando la fila lo sabe, con el `titular_id` al
                    lado. El identificador va SIEMPRE -es la identidad que viaja
                    en la declaracion y lo que deja conciliar la fila con la API,
                    y ademas distingue homonimos-, y el nombre solo lo tiene la
                    fila que se eligio del padron: la sembrada desde el historial
                    no puede tenerlo, porque la `Parte` no lo lleva (D-006). Ese
                    caso NO pinta hueco ni guion -no falta un dato, falta un
                    nombre que esa version nunca guardo- y el espacio que separa
                    los dos va con el nombre, para que no quede suelto. */}
                {fila.nombre !== "" && <span>{fila.nombre} </span>}
                <span className="detalle-identificador">{fila.titularId}</span>
              </td>
              <td>
                {/* El IPI se rellena con el que el padron tiene hoy para ese
                    titular, y se puede corregir: la parte que se guarda lleva
                    el IPI declarado. Quien lo concilia es el backend, en
                    exigirPuedenRecibirReparto (internal/aplicacion/declaraciones.go),
                    que ya trae la fila del padron para R-01 y compara su IPI
                    con este: si no cuadra no se guarda nada y la peticion sale
                    con 400. Esta pantalla no lo comprueba porque no tiene el
                    padron; lo que no puede hacer es dar por bueno lo que
                    escriba. */}
                <input
                  className="editor-campo"
                  type="text"
                  autoComplete="off"
                  aria-label={`IPI de ${fila.titularId}`}
                  value={fila.ipi}
                  onChange={(e) =>
                    onCambiar(fila.clave, { ipi: e.target.value })
                  }
                />
              </td>
              <td className="tabla-partes-numero">
                <input
                  className="editor-campo editor-campo-numero"
                  type="text"
                  inputMode="decimal"
                  autoComplete="off"
                  aria-label={`Porcentaje de ${fila.titularId}`}
                  value={fila.porcentaje}
                  onChange={(e) =>
                    onCambiar(fila.clave, { porcentaje: e.target.value })
                  }
                />
              </td>
              <td>
                {/* El texto visible no cambia de significado: dice la accion y el
                    titular que ya estaban escritos. El nombre accesible es lo que
                    anuncia un lector de pantalla, y el paso 11 lo dejo con el
                    `titular_id` porque la fila no tenia el nombre; ahora se nombra
                    al titular por su nombre cuando la fila lo sabe -la que se
                    agrego del padron- y se cae al identificador cuando no -la
                    sembrada desde el historial (D-006)-, con el mismo criterio con
                    el que la primera columna se lee. Sin nombre, "Quitar a tit-3
                    del reparto" nombra sin decir de quien. */}
                <button
                  type="button"
                  className="enlace"
                  aria-label={`Quitar a ${comoSeNombraLaFila(fila)} del reparto`}
                  onClick={() => onQuitar(fila.clave)}
                >
                  Quitar de {fila.titularId}
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/**
 * El padron de titulares: de donde se eligen las partes.
 *
 * Se pide SIN filtro de `persona_natural`, y eso es una decision, no un
 * descuido: el contrato devuelve el padron entero justamente para que quien
 * edita pueda explicar por que el titular que busca no esta entre los elegibles.
 * Pidiendo solo personas naturales, la productora desapareceria del listado y la
 * regla se leeria como un dato que falta en vez de como `R-01`.
 *
 * La pagina y la busqueda son de esta pantalla y no de la URL: son la
 * herramienta con la que se busca un titular, no un estado del reparto, y un
 * enlace compartido que abriera el editor con el padron ya filtrado diria del
 * reparto algo que no es.
 */
function PadronDelEditor({
  enElReparto,
  onAgregar,
}: {
  enElReparto: readonly string[];
  onAgregar: (titular: Titular) => void;
}) {
  const [busqueda, setBusqueda] = useState("");
  const [desplazamiento, setDesplazamiento] = useState(0);
  const idNombre = useId();
  const idAyuda = useId();

  const params = new URLSearchParams();
  if (busqueda !== "") params.set("nombre", busqueda);
  params.set("limite", String(LIMITE_PADRON));
  if (desplazamiento > 0) {
    params.set("desplazamiento", String(desplazamiento));
  }
  // La busqueda se pide mientras se teclea, asi que la consulta espera a que el
  // tecleo pare (`DEBOUNCE_TECLEO_MS`, declarado una sola vez en `useApi.ts`):
  // sin esa espera cada tecla era una peticion al padron entero. El `path`
  // cambia con la busqueda y con la pagina, y el hook aborta la que ya no sirve.
  const {
    datos: titulares,
    cargando,
    error,
  } = useApi<Titular[]>(
    `/api/titulares?${params.toString()}`,
    DEBOUNCE_TECLEO_MS,
  );

  /** Cambiar la busqueda vuelve a la primera pagina: la 3 de otra busqueda no existe. */
  function buscar(valor: string) {
    setBusqueda(valor);
    setDesplazamiento(0);
  }

  let contenido: ReactElement;
  if (cargando) {
    contenido = <Cargando texto="Cargando el padrón…" />;
  } else if (error) {
    // Sin padron no se pueden elegir partes, y la pantalla lo dice: sin este
    // aviso el borrador solo se podria llenar a ciegas. Lo que no se hace es
    // dejar de ofrecer el guardado de lo ya escrito -el padron no es la
    // autoridad de nada, solo la lista de donde elegir-.
    contenido = (
      <p className="catalogo-error" role="alert">
        No se pudo consultar el padrón de titulares: {error.message} Sin el
        padrón no se pueden elegir partes, pero lo que ya esté en el borrador
        sigue siendo lo que se guarda.
      </p>
    );
  } else if (!Array.isArray(titulares) || !titulares.every(esTitular)) {
    contenido = (
      <p className="catalogo-error" role="alert">
        El padrón no llegó como una lista de titulares legibles.
      </p>
    );
  } else if (titulares.length === 0) {
    contenido = (
      <div className="catalogo-vacio">
        <p>
          {busqueda === ""
            ? "El padrón no trae ningún titular en esta página."
            : "Ningún titular del padrón coincide con esa búsqueda."}
        </p>
      </div>
    );
  } else {
    contenido = (
      <>
        <div className="catalogo-caja">
          <table className="tabla-partes" aria-label="Padrón de titulares">
            <thead>
              <tr>
                <th scope="col">Nombre</th>
                <th scope="col">Titular</th>
                <th scope="col">IPI</th>
                <th scope="col">Clase</th>
                <th scope="col">Puede ser parte</th>
              </tr>
            </thead>
            <tbody>
              {titulares.map((titular) => (
                <tr key={titular.id}>
                  {/* "Nombre" a secas y no el rotulo del padron actual: esta
                      tabla ES el padron leido ahora, no el nombre de un titular
                      junto a una version historica. El dia que un nombre se
                      pinte al lado de una version, ese si lleva el rotulo
                      (D-006). */}
                  <td>{titular.nombre}</td>
                  <td className="detalle-identificador">{titular.id}</td>
                  {/* Vacio es "no se conoce": el padron solo exige IPI a las
                      personas naturales, y esta celda no puede decir mas. */}
                  <td className="detalle-identificador">
                    {titular.ipi !== "" ? titular.ipi : "—"}
                  </td>
                  <td>{titular.clase}</td>
                  <td>
                    {puedeSerParte(titular) ? (
                      enElReparto.includes(titular.id) ? (
                        <span className="muted">Ya está en el reparto</span>
                      ) : (
                        <button
                          type="button"
                          className="catalogo-limpiar"
                          onClick={() => onAgregar(titular)}
                        >
                          Añadir al reparto
                        </button>
                      )
                    ) : (
                      // La explicacion, no un hueco: el titular existe en el
                      // padron y no se ofrece, y sin esta frase pareceria que
                      // falta un dato. La regla la impone el backend al guardar
                      // -400 con su propio mensaje-, y este es el mismo hecho
                      // dicho antes.
                      <span className="editor-no-elegible">
                        No: solo un escritor persona natural puede ser titular
                        de una declaración (R-01, RD 4.5)
                      </span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="catalogo-pie">
            <span>
              {desplazamiento === 0
                ? `Titulares 1 a ${titulares.length}`
                : `Titulares ${desplazamiento + 1} a ${desplazamiento + titulares.length}`}
            </span>
            <div className="catalogo-pie-botones">
              <button
                type="button"
                className="catalogo-pagina"
                disabled={desplazamiento === 0}
                onClick={() =>
                  setDesplazamiento(Math.max(0, desplazamiento - LIMITE_PADRON))
                }
              >
                Anterior
              </button>
              <button
                type="button"
                className="catalogo-pagina"
                disabled={titulares.length < LIMITE_PADRON}
                onClick={() =>
                  setDesplazamiento(desplazamiento + LIMITE_PADRON)
                }
              >
                Siguiente
              </button>
            </div>
          </div>
        </div>
        <p className="muted detalle-nota">
          El padrón se sirve por páginas y no admite filtrar por identificador,
          así que una página que no traiga al titular no prueba que no exista.
          El nombre y la clase son los que el padrón tiene hoy.
        </p>
      </>
    );
  }

  return (
    <section className="editor-padron">
      <h2>Padrón de titulares</h2>
      <p className="muted detalle-nota">
        Se listan las dos clases de titular a propósito: quien está en el padrón
        y no puede figurar como parte se explica aquí en vez de faltar. Estar en
        el padrón no da derecho a cobrar por sí solo: el porcentaje sale de esta
        declaración (R-02, R-03).
      </p>
      <div className="catalogo-campo">
        <label htmlFor={idNombre}>Buscar por nombre</label>
        <input
          id={idNombre}
          type="text"
          autoComplete="off"
          placeholder="Buscar en el padrón…"
          value={busqueda}
          onChange={(e) => buscar(e.target.value)}
          aria-describedby={idAyuda}
        />
        <p id={idAyuda} className="catalogo-ayuda">
          Coincidencia parcial, sin distinguir mayúsculas.
        </p>
      </div>
      {contenido}
    </section>
  );
}

/**
 * Lo que contesto el guardado.
 *
 * El caso que importa es `incierta`: **no dice que no se guardara nada**. Un 5xx
 * es indistinguible de un COMMIT que si entro -ver `puedeHaberGuardado`-, asi
 * que lo unico honesto es decir que no se sabe y mandar al historial, que es
 * donde se comprueba. El mismo criterio que #29 aplico al 500 de la ingesta.
 */
function PanelDelGuardado({
  resultado,
  obraId,
  busqueda,
  volver,
}: {
  resultado: ResultadoDelGuardado;
  obraId: string;
  busqueda: string;
  volver: string;
}) {
  // La misma busqueda del catalogo que lleva la ficha viaja al historial: el
  // camino de vuelta no pierde los filtros por pasar por dos pantallas.
  const enlaceAlHistorial = (
    <p className="detalle-nota">
      <Link
        to={`/catalogo/${encodeURIComponent(obraId)}/historial`}
        state={{ [CLAVE_DE_VUELTA_AL_CATALOGO]: busqueda }}
      >
        Ver el historial de la declaración
      </Link>
    </p>
  );

  if (resultado.tipo === "guardada") {
    const { version } = resultado.version;
    return (
      <section className="panel-resultado panel-ok" role="status">
        <h2>El servidor abrió la versión {version}</h2>
        {/* El numero, el estado y la fecha son los de la respuesta: no hay
            ninguna cifra calculada aqui. */}
        <dl className="panel-datos">
          <div>
            <dt>Versión</dt>
            <dd>{version}</dd>
          </div>
          <div>
            <dt>Estado</dt>
            <dd>
              <EtiquetaDeEstado estado={resultado.version.estado} />
            </dd>
          </div>
          <div>
            <dt>Vigente desde</dt>
            <dd>
              <time
                dateTime={resultado.version.vigente_desde}
                title={resultado.version.vigente_desde}
              >
                {formatearInstante(resultado.version.vigente_desde)}
              </time>
            </dd>
          </div>
        </dl>
        {/* El parrafo va ramificado por el numero que el servidor acaba de
            contestar, y no es un detalle de estilo: con la version 1 -el caso
            central del issue, una obra sin ninguna declaracion- NO hay ninguna
            version anterior, asi que el texto que se pintaba sin condicion
            afirmaba un hecho del registro que no existe. Es la misma disciplina
            del aviso de arriba, que se cura con "si la hay". Las dos ramas
            dicen cosas que la otra no desmiente. */}
        {version === 1 ? (
          <p className="panel-aviso">
            Esta es la primera declaración de la obra: no había ninguna versión
            anterior que conservar, y esta queda en el historial desde ahora con
            los porcentajes que se acaban de declarar. Un reparto de un periodo
            pasado se reproduce con el reparto que regía entonces, y antes de
            esta versión no había ninguno declarado.
          </p>
        ) : (
          <p className="panel-aviso">
            La versión anterior no se borra ni se modifica: sigue en el
            historial con los porcentajes que regían hasta ahora. Un reparto de
            un periodo pasado se reproduce con el reparto que estaba vigente
            entonces.
          </p>
        )}
        {enlaceAlHistorial}
      </section>
    );
  }

  if (resultado.tipo === "guardadaSinLeer") {
    return (
      <section className="panel-resultado panel-parcial" role="alert">
        {/* "Contesto sin error, sin la version nueva" y no "se guardo": un 200
            es lo que el contrato promete cuando la version se abre, pero de un
            cuerpo que no se puede leer lo unico que se sabe es el status. */}
        <h2>El servidor contestó sin error, sin la versión nueva</h2>
        <p className="panel-mensaje">
          El contrato promete la versión nueva en el cuerpo de un 200, y este no
          llegó con esa forma: esta pantalla no puede decir qué versión se abrió
          ni con qué estado quedó. El historial es donde se comprueba.
        </p>
        {enlaceAlHistorial}
      </section>
    );
  }

  if (resultado.tipo === "rechazada") {
    return (
      <section className="panel-resultado panel-fallo" role="alert">
        <h2>{tituloDelRechazo(resultado.status)}</h2>
        {/* El mensaje del backend, entero y tal cual: nombra la regla que se
            incumplio y cual de ellas ha sido solo lo sabe el. Reescribirlo
            acoplaria la pantalla a la prosa de Go y se romperia en silencio. */}
        <p className="panel-mensaje">{resultado.mensaje}</p>
        {/* El aviso se pinta en todos los 4xx que llegan hasta esta pantalla
            -el 400, el 403 y el 404; el 401 lo corta `api()` antes, ver
            `avisoDelRechazo`-, y no solo en el 400: en los tres la version nueva
            no se abrio, y callarlo dejaba a quien edita sin saber si puede
            reintentar con tranquilidad. */}
        <p>{avisoDelRechazo(resultado.status)}</p>
        {resultado.status === 404 && (
          <p className="detalle-nota">
            <Link to={volver}>Volver al catálogo</Link>
          </p>
        )}
      </section>
    );
  }

  return (
    <section className="panel-resultado panel-fallo" role="alert">
      <h2>No se sabe si la declaración se guardó</h2>
      {/* El mensaje del servidor va igual: lo que no se afirma es que no haya
          quedado nada escrito. */}
      <p className="panel-mensaje">{resultado.mensaje}</p>
      {/* El aviso que este paso existe para no olvidar. `Store.Guardar` corre
          dentro de `EnTransaccion`, asi que el fallo del COMMIT es
          indistinguible de uno limpio y la version PUEDE haberse abierto. */}
      <p>
        No se puede distinguir un fallo al confirmar de una respuesta que se
        perdió, así que la versión pudo haberse abierto igual. Compruébalo en el
        historial antes de volver a guardar: si se abrió, guardar otra vez abre
        una versión de más.
      </p>
      {enlaceAlHistorial}
    </section>
  );
}
