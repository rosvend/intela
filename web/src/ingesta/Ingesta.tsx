import { DragEvent, FormEvent, useId, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api } from "../api";
import ListaCargas from "./ListaCargas";
import PanelResultado, {
  type Entrega,
  type Resultado,
  resultadoDeError,
} from "./PanelResultado";

// D-005: las fuentes con adaptador dado de alta en
// internal/infraestructura/ingesta/fuentes.go. Una fuente nueva se anade alli
// y aqui; si divergen, el 400 del backend lista los pares (fuente, formato)
// validos.
const FUENTES = [
  { valor: "caracol", etiqueta: "Caracol (TV)" },
  { valor: "netflix", etiqueta: "Netflix (OTT)" },
  { valor: "cine", etiqueta: "Cine" },
] as const;

// D-004: repite la forma que acepta el backend (AAAA o AAAA-MM) solo para no
// consultar ni subir con un periodo a medias. La autoridad sigue siendo el
// backend: si algo diverge, su 400 se muestra.
const FORMA_PERIODO = /^\d{4}(-\d{2})?$/;

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
  // D-004: el periodo que manda es el de la URL. Filtra el listado y va en la
  // subida; el campo de texto solo lo escribe ahi cuando esta completo.
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
  // D-007: sube tras cada 201 para remontar el listado, que asi se vuelve a
  // pedir sin tocar `useApi`.
  const [version, setVersion] = useState(0);
  // `disabled` llega en el siguiente render; el ref corta tambien un segundo
  // clic que entre antes.
  const enVuelo = useRef(false);

  const idPeriodo = useId();
  const idAyudaPeriodo = useId();
  const idFuente = useId();
  const idArchivo = useId();
  const idFaltaPeriodo = useId();

  // Con el campo a medias el periodo de la URL no es el que se ve: no se sube
  // con uno distinto del que muestra la pantalla.
  const periodoListo =
    periodoAplicado !== "" && textoPeriodo === periodoAplicado;
  const puedeSubir =
    !enviando && periodoListo && fuente !== "" && archivo !== null;

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
      // `Entrega` segun api/openapi.yaml.
      const entrega = (await api("/api/reportes", {
        method: "POST",
        body: formulario,
      })) as Entrega;
      setResultado({ tipo: "entrega", entrega });
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
            aria-describedby={periodoListo ? undefined : idFaltaPeriodo}
          >
            {enviando ? "Subiendo…" : "Subir reporte"}
          </button>
          {!periodoListo && (
            <p id={idFaltaPeriodo} className="ingesta-ayuda">
              Escribe un periodo completo (AAAA o AAAA-MM) para poder subir el
              reporte.
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
        <ListaCargas key={version} periodo={periodoAplicado} />
      </section>
    </section>
  );
}
