import { DragEvent, FormEvent, useId, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api } from "../api";
import ListaCargas from "./ListaCargas";
import PanelResultado, {
  type Entrega,
  type Resultado,
  pudoHaberLlegado,
  resultadoDeError,
} from "./PanelResultado";
import { esRechazo } from "./TablaRechazos";

// Las fuentes con adaptador dado de alta en
// internal/infraestructura/ingesta/fuentes.go. Una fuente nueva se anade alli
// y aqui; si divergen, el 400 del backend lista los pares (fuente, formato)
// validos.
const FUENTES = [
  { valor: "caracol", etiqueta: "Caracol (TV)" },
  { valor: "netflix", etiqueta: "Netflix (OTT)" },
  { valor: "cine", etiqueta: "Cine" },
] as const;

// La forma de un periodo que merece una consulta: AAAA, o AAAA-MM. Es floja a
// proposito -el mes no se mira aqui-: mientras se teclea se pasa por formas a
// medias y consultar con ellas llenaria la pantalla de 400.
//
// Que el mes exista lo decide el servidor, y su 400 es el que se ve. Antes esta
// guarda convivia con una tercera copia del patron del dominio
// (`FORMA_PERIODO_DOMINIO`) que bloqueaba la subida en el navegador, y el
// backend validaba el periodo con uno MAS FLOJO todavia
// (`^[0-9]{4}(-[0-9]{2})?$`, internal/aplicacion/trabajos.go) en el camino que
// escribe la boveda: `2026-13` entraba sin un solo 400 desde `curl`, el
// scheduler o cualquier pantalla futura, y no hay vuelta atras que lo arregle
// -la unicidad de una entrega es (sha256, fuente), la huella no incluye el
// periodo y no hay ruta de borrado-. La regla se apretó donde vive, en el
// dominio (internal/dominio/recaudo/bolsa.go), y su mensaje nombra el mes entre
// 01 y 12; la copia de aqui se borro con ella.
const FORMA_PERIODO = /^\d{4}(-\d{2})?$/;

/** Un texto con algo dentro, que es lo que son los campos de texto `Entrega`. */
function esTextoNoVacio(valor: unknown): valor is string {
  return typeof valor === "string" && valor !== "";
}

/**
 * Si un cuerpo sin tipar tiene la forma minima de una `Entrega`
 * (api/openapi.yaml). Se comprueba todo lo que el panel y la tabla leen:
 *
 * - `id`, `fuente`, `periodo`, `sha256` y `clave_objeto`: cadenas no vacias;
 * - `aceptados`: un numero;
 * - `rechazados`: una lista de rechazos, y cada uno pasa por `esRechazo`, que es
 *   quien comprueba los cuatro campos que `TablaRechazos` lee (`id`, `titulo`,
 *   `ids_fuente` y `motivo`). Una sola definicion: `esRechazo` vive junto al
 *   tipo `Rechazo`, y si la tabla empieza a leer un campo nuevo se toca alli.
 *   Exigir aqui "objeto no nulo" dejaba pasar un `ids_fuente` objeto -el
 *   contrato lo declara `string`- y React lanzaba "Objects are not valid as a
 *   React child" al pintarlo, con el panel entero muerto.
 *
 * `nbytes` no se mira: no lo lee ningun consumidor de la pantalla (el panel no
 * lo pinta, y el listado, que si lo tiene en su tipo, tampoco).
 *
 * Un 2xx con otra forma -el `{"rechazados": []}` de un despliegue a medias, un
 * objeto suelto, el HTML de un proxy- pasaba la guarda anterior, que solo
 * exigia "objeto con `rechazados` lista", y tumbaba la pantalla entera al leer
 * `huellaCorta(entrega.sha256)`; y no hay ErrorBoundary en `web/src` que la
 * recoja.
 *
 * Lo que no pasa el filtro cae en el fallo no clasificable: ahi el panel ya
 * avisa de que la entrega pudo haber llegado, que es lo que corresponde cuando
 * un 2xx no se deja leer.
 */
function esEntrega(cuerpo: unknown): cuerpo is Entrega {
  if (typeof cuerpo !== "object" || cuerpo === null) return false;
  const entrega = cuerpo as {
    id?: unknown;
    fuente?: unknown;
    periodo?: unknown;
    sha256?: unknown;
    clave_objeto?: unknown;
    aceptados?: unknown;
    rechazados?: unknown;
  };
  return (
    esTextoNoVacio(entrega.id) &&
    esTextoNoVacio(entrega.fuente) &&
    esTextoNoVacio(entrega.periodo) &&
    esTextoNoVacio(entrega.sha256) &&
    esTextoNoVacio(entrega.clave_objeto) &&
    typeof entrega.aceptados === "number" &&
    Array.isArray(entrega.rechazados) &&
    entrega.rechazados.every(esRechazo)
  );
}

/**
 * Subida manual de reportes de uso (POST /reportes) y listado de las cargas
 * hechas. Solo administrador: lo filtra el guard del Layout, y el servidor lo
 * exige con `requiereRol`.
 *
 * El navegador no lee ni valida el archivo: las reglas de estructura viven en
 * Go, y su 400 se muestra tal cual.
 */
export default function Ingesta() {
  const [searchParams, setSearchParams] = useSearchParams();
  // El periodo que manda es el de la URL: filtra el listado y es el que viaja
  // en la subida. El campo de texto solo lo escribe ahi cuando lo que dice ya
  // tiene la forma completa.
  const periodoAplicado = searchParams.get("periodo") ?? "";
  const [textoPeriodo, setTextoPeriodo] = useState(periodoAplicado);
  // Si la URL cambia por fuera del campo (el enlace de la nav, atras), el
  // campo la sigue. Es el ajuste de estado durante el render que recomienda
  // React, en vez de un efecto que pintaria primero el valor viejo.
  const [periodoVisto, setPeriodoVisto] = useState(periodoAplicado);
  if (periodoAplicado !== periodoVisto) {
    setPeriodoVisto(periodoAplicado);
    setTextoPeriodo(periodoAplicado);
  }

  const [fuente, setFuente] = useState("");
  const [archivo, setArchivo] = useState<File | null>(null);
  const [arrastrando, setArrastrando] = useState(false);
  const [enviando, setEnviando] = useState(false);
  const [resultado, setResultado] = useState<Resultado | null>(null);
  // Sube para remontar el listado, que asi se vuelve a pedir sin tocar `useApi`.
  // Lo hace en dos casos, y el segundo es el que importa: tras un 201, para ver
  // la carga nueva, y tras cualquier desenlace que deje la duda de si la entrega
  // quedo registrada (`pudoHaberLlegado`), porque el aviso del panel manda al
  // operador a mirar JUSTO ese listado. Sin remontarlo ahi, lo que mira es la
  // foto de antes del intento: si el COMMIT entro, no ve la fila nueva, concluye
  // que no llego y reenvia -409 irreversible-.
  const [version, setVersion] = useState(0);
  // `disabled` llega en el siguiente render; el ref corta tambien un segundo
  // clic que entre antes.
  const enVuelo = useRef(false);

  const idPeriodo = useId();
  const idAyudaPeriodo = useId();
  const idFuente = useId();
  const idArchivo = useId();
  const idAvisoPeriodo = useId();

  // Con el campo a medias el periodo de la URL no es el que se ve: no se sube
  // con uno distinto del que muestra la pantalla.
  //
  // Lo unico que se decide aqui es que el periodo este COMPLETO. Que el mes
  // exista lo decide el servidor, y su 400 se pinta tal cual: es la misma
  // autoridad que juzga el archivo, y duplicar su regla en el cliente era una
  // tercera copia del patron del dominio, la que menos cubria.
  const periodoListo =
    periodoAplicado !== "" && textoPeriodo === periodoAplicado;
  const puedeSubir =
    !enviando && periodoListo && fuente !== "" && archivo !== null;

  /** El texto del boton, que nombra el periodo al que subiria el archivo. */
  function etiquetaSubir(): string {
    if (enviando) return "Subiendo…";
    if (periodoListo) return `Subir a ${periodoAplicado}`;
    return "Subir reporte";
  }

  function cambiarPeriodo(valor: string) {
    setTextoPeriodo(valor);
    if (valor !== "" && !FORMA_PERIODO.test(valor)) return;
    setSearchParams(
      (previos) => {
        const siguientes = new URLSearchParams(previos);
        if (valor) siguientes.set("periodo", valor);
        else siguientes.delete("periodo");
        return siguientes;
      },
      { replace: true },
    );
  }

  function alSoltar(evento: DragEvent<HTMLDivElement>) {
    evento.preventDefault();
    setArrastrando(false);
    const soltado = evento.dataTransfer.files[0];
    if (soltado) setArchivo(soltado);
  }

  async function subir(evento: FormEvent) {
    evento.preventDefault();
    if (enVuelo.current || !puedeSubir || !archivo) return;
    enVuelo.current = true;
    setEnviando(true);
    setResultado(null);

    const formulario = new FormData();
    formulario.append("fuente", fuente);
    formulario.append("periodo", periodoAplicado);
    // Sin `formato`: el backend lo deduce de la extension del archivo.
    formulario.append("archivo", archivo);

    try {
      // Frontera sin validar, como las de sesion.tsx: el 201 trae una
      // `Entrega` segun api/openapi.yaml. El cast no comprueba nada en
      // ejecucion, asi que la forma se revisa antes de entregarsela al panel.
      const cuerpo = await api("/api/reportes", {
        method: "POST",
        body: formulario,
      });
      if (!esEntrega(cuerpo)) {
        throw new Error("el 201 no trajo una entrega legible");
      }
      setResultado({ tipo: "entrega", entrega: cuerpo });
      setVersion((previa) => previa + 1);
    } catch (error) {
      const fallo = resultadoDeError(error);
      setResultado(fallo);
      // La misma duda que hace salir el aviso de PUDO_LLEGAR en el panel, de la
      // misma funcion: el aviso manda a mirar el listado, asi que el listado
      // tiene que volver a pedirse o lo que se mira es la foto de antes.
      if (pudoHaberLlegado(fallo.status)) {
        setVersion((previa) => previa + 1);
      }
    } finally {
      enVuelo.current = false;
      setEnviando(false);
    }
  }

  return (
    <section className="ingesta">
      <header className="ingesta-cabecera">
        <h1>Ingesta de reportes</h1>
        <p className="muted">
          Los reportes de uso no traen importes: solo ponderan el reparto de la
          bolsa recaudada en el periodo. La validación del archivo la hace el
          servidor.
        </p>
      </header>

      <form className="ingesta-formulario" onSubmit={(e) => void subir(e)}>
        <div className="ingesta-campos">
          <div className="ingesta-campo">
            <label htmlFor={idPeriodo}>Periodo de recaudo</label>
            <input
              id={idPeriodo}
              type="text"
              placeholder="AAAA-MM"
              autoComplete="off"
              value={textoPeriodo}
              onChange={(e) => cambiarPeriodo(e.target.value)}
              aria-describedby={idAyudaPeriodo}
            />
            <p id={idAyudaPeriodo} className="ingesta-ayuda">
              AAAA para un periodo anual, AAAA-MM para uno mensual, con el mes
              entre 01 y 12.
            </p>
          </div>

          <div className="ingesta-campo">
            <label htmlFor={idFuente}>Fuente</label>
            <select
              id={idFuente}
              value={fuente}
              onChange={(e) => setFuente(e.target.value)}
            >
              <option value="">Elige la fuente</option>
              {FUENTES.map((f) => (
                <option key={f.valor} value={f.valor}>
                  {f.etiqueta}
                </option>
              ))}
            </select>
          </div>
        </div>

        <div
          className={
            arrastrando
              ? "ingesta-soltar ingesta-soltar-activa"
              : "ingesta-soltar"
          }
          onDragOver={(e) => {
            // Sin esto el navegador no deja soltar aqui: abriria el archivo.
            e.preventDefault();
            setArrastrando(true);
          }}
          onDragLeave={() => setArrastrando(false)}
          onDrop={alSoltar}
        >
          <label htmlFor={idArchivo}>Archivo</label>
          {/* `accept` solo filtra el selector. Lo que se suelta no se revisa
              aqui: una extension sin adaptador la rechaza el backend con 400. */}
          <input
            id={idArchivo}
            type="file"
            accept=".csv,.xlsx,.json"
            onChange={(e) => setArchivo(e.target.files?.[0] ?? null)}
          />
          <p className="ingesta-ayuda">
            Arrastra el archivo hasta aquí o elígelo con el selector: CSV, Excel
            (.xlsx) o JSON.
          </p>
          {archivo && (
            <p className="ingesta-archivo">
              Archivo elegido: <strong>{archivo.name}</strong>
            </p>
          )}
        </div>

        <div className="ingesta-acciones">
          <button
            type="submit"
            className="boton-primario"
            disabled={!puedeSubir}
            aria-describedby={periodoListo ? undefined : idAvisoPeriodo}
          >
            {etiquetaSubir()}
          </button>
          {/* El aviso dice lo unico que decide el cliente: que el periodo este
              completo. Que el mes exista lo contesta el servidor con su 400, y
              el rango del mes se explica en la ayuda del campo. */}
          {!periodoListo && (
            <p id={idAvisoPeriodo} className="ingesta-ayuda">
              Escribe el periodo completo para poder subir el reporte: AAAA, o
              AAAA-MM.
            </p>
          )}
        </div>
      </form>

      {/* Siempre montado: una region viva que aparece junto con su contenido
          no se anuncia. */}
      <div className="ingesta-resultado" aria-live="polite">
        {resultado && <PanelResultado resultado={resultado} />}
      </div>

      <section className="ingesta-cargas">
        <h2>Cargas hechas</h2>
        {/* La clave lleva el periodo ademas de `version`: al cambiar de periodo
            el listado se remonta, y asi sus filas abiertas no se reabren solas
            ni vuelven a pedir su log sin que nadie haga clic. */}
        <ListaCargas
          key={`${periodoAplicado}|${version}`}
          periodo={periodoAplicado}
        />
      </section>
    </section>
  );
}
