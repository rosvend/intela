import { DragEvent, FormEvent, useId, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api } from "../api";
import ListaCargas from "./ListaCargas";
import PanelResultado, {
  type Entrega,
  type Resultado,
  resultadoDeError,
} from "./PanelResultado";

// Las fuentes con adaptador dado de alta en
// internal/infraestructura/ingesta/fuentes.go. Una fuente nueva se anade alli
// y aqui; si divergen, el 400 del backend lista los pares (fuente, formato)
// validos.
const FUENTES = [
  { valor: "caracol", etiqueta: "Caracol (TV)" },
  { valor: "netflix", etiqueta: "Netflix (OTT)" },
  { valor: "cine", etiqueta: "Cine" },
] as const;

// Repite la forma que acepta el backend (AAAA o AAAA-MM) solo para decidir si
// el periodo pasa a la URL y el listado se vuelve a pedir. Es floja a
// proposito: mientras se teclea se pasa por formas a medias, y consultar con
// ellas llenaria la pantalla de 400. La autoridad para consultar sigue siendo
// el backend, que ya contesta su mensaje si el periodo es imposible.
const FORMA_PERIODO = /^\d{4}(-\d{2})?$/;

// El patron de un periodo de recaudo del dominio: un ano, o un ano y un mes
// que existe. Es el mismo de internal/dominio/recaudo/bolsa.go, y es lo que
// decide si se puede SUBIR.
//
// Hace falta porque el backend no lo va a frenar: la capa de aplicacion valida
// el periodo con uno mas flojo (`^[0-9]{4}(-[0-9]{2})?$`, en
// internal/aplicacion/trabajos.go), asi que `2026-00` y `2026-13` entran sin
// un solo 400. El 0 y el 1 son teclas vecinas: `2026-03` se vuelve `2026-13`
// con un solo error. Estrechar el del backend queda como arreglo pendiente,
// fuera de esta pantalla.
//
// Y no hay vuelta atras que lo arregle: la unicidad es (sha256, fuente), la
// huella del reporte no incluye el periodo y no hay ruta de borrado, asi que
// un archivo subido a un mes que no existe queda quemado para siempre; volver
// a subirlo con el periodo corregido da 409 y sus usos ponderan un periodo que
// ningun reparto cierra.
const FORMA_PERIODO_DOMINIO = /^\d{4}(-(0[1-9]|1[0-2]))?$/;

/**
 * Si un cuerpo sin tipar tiene la forma minima de una `Entrega`
 * (api/openapi.yaml): un objeto con `rechazados` como lista, que es lo unico
 * que el panel no puede tolerar que falte. Un 2xx con un cuerpo que no es JSON
 * (el 201 de un proxy o de un despliegue a medias), o con otra forma, tumbaba
 * la pantalla entera al pintar `entrega.rechazados.length`; y no hay
 * ErrorBoundary en `web/src`.
 *
 * Lo que no pasa el filtro cae en el fallo no clasificable: ahi el panel ya
 * avisa de que la entrega pudo haber llegado, que es lo que corresponde cuando
 * un 201 no se deja leer.
 */
function esEntrega(cuerpo: unknown): cuerpo is Entrega {
  return (
    typeof cuerpo === "object" &&
    cuerpo !== null &&
    Array.isArray((cuerpo as { rechazados?: unknown }).rechazados)
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
  // Sube tras cada 201 para remontar el listado, que asi se vuelve a pedir sin
  // tocar `useApi`.
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
  const periodoListo =
    periodoAplicado !== "" && textoPeriodo === periodoAplicado;
  // Que este completo no basta: el periodo tiene que ser uno que exista. La
  // subida no se deshace, y `2026-00` o `2026-13` pasan el validador del
  // backend sin protestar.
  const periodoUtil =
    periodoListo && FORMA_PERIODO_DOMINIO.test(periodoAplicado);
  const puedeSubir =
    !enviando && periodoUtil && fuente !== "" && archivo !== null;

  /** El texto del boton, que nombra el periodo al que subiria el archivo. */
  function etiquetaSubir(): string {
    if (enviando) return "Subiendo…";
    if (periodoUtil) return `Subir a ${periodoAplicado}`;
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
      setResultado(resultadoDeError(error));
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
              AAAA para un periodo anual, AAAA-MM para uno mensual.
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
            aria-describedby={periodoUtil ? undefined : idAvisoPeriodo}
          >
            {etiquetaSubir()}
          </button>
          {/* Un solo aviso para las dos maneras de no servir el periodo: a
              medias o completo pero imposible (un mes que no existe). */}
          {!periodoUtil && (
            <p id={idAvisoPeriodo} className="ingesta-ayuda">
              Escribe un periodo válido para poder subir el reporte: AAAA, o
              AAAA-MM con el mes entre 01 y 12.
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
