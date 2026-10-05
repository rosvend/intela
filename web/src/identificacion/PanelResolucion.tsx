import { Fragment, useId, useState, type ReactElement } from "react";
import "../revision.css";
import { ApiError } from "../api";
import { esObra, type Obra } from "../catalogo/tipos";
import { useLista } from "../catalogo/useLista";
import { DEBOUNCE_TECLEO_MS, useValorDiferido } from "../useValorDiferido";
import {
  MAX_NOTA,
  causaDelConflicto,
  resolverCaso,
  type ResolucionDeCaso,
} from "./resolucion";
import MedidorConfianza from "./MedidorConfianza";
import {
  compararTitulos,
  formatearPeriodo,
  nombreDeFuente,
} from "./presentacion";
import type { CasoIdentificacion } from "./tipos";

/**
 * Con que obra trabaja el panel, segun por donde se abrio:
 *
 * - `candidata`: la tarjeta del caso ya trae una obra propuesta (con su
 *   puntaje); el panel la muestra elegida desde que abre.
 * - `busqueda`: "Buscar otra obra" (D7): no hay obra hasta que se busca y se
 *   elige una del catalogo.
 * - `descarte`: no hay obra en absoluto, el registro no se asigna a nada.
 */
export type ModoResolucion =
  | {
      tipo: "candidata";
      obraId: string;
      obraTitulo: string;
      obraAnio: number;
      obraGenero: string;
      puntaje: number;
    }
  | { tipo: "busqueda" }
  | { tipo: "descarte" };

const TITULO_POR_MODO: Record<ModoResolucion["tipo"], string> = {
  candidata: "Asignar obra",
  busqueda: "Buscar otra obra",
  descarte: "Descartar registro",
};

/** La obra que el panel tiene elegida en un momento dado, sea cual sea el modo. */
type ObraElegida = {
  id: string;
  titulo: string;
  anio?: number;
  genero?: string;
  puntaje?: number;
};

function obraElegidaInicial(modo: ModoResolucion): ObraElegida | null {
  return modo.tipo === "candidata"
    ? {
        id: modo.obraId,
        titulo: modo.obraTitulo,
        anio: modo.obraAnio,
        genero: modo.obraGenero,
        puntaje: modo.puntaje,
      }
    : null;
}

/**
 * Los desenlaces de ENVIAR una resolucion que el panel tiene que distinguir
 * (plano seccion 5, "Panel de resolucion"). No es lo mismo que
 * `CausaDeConflicto` de `resolucion.ts`: esa clasifica el 409, esta ademas
 * decide el texto y si hace falta "Recargar caso".
 */
type ErrorDeEnvio =
  | { tipo: "conexion" }
  | { tipo: "api"; mensaje: string }
  | { tipo: "ya_resuelto" }
  | { tipo: "no_pendiente" }
  | { tipo: "desconocido"; mensaje: string }
  | { tipo: "alias"; mensaje: string };

function clasificarError(error: unknown): ErrorDeEnvio {
  if (error instanceof ApiError) {
    if (error.status === 409) {
      const causa = causaDelConflicto(error);
      if (causa === "ya_resuelto") return { tipo: "ya_resuelto" };
      if (causa === "no_pendiente") return { tipo: "no_pendiente" };
      if (causa === "alias_en_conflicto")
        return { tipo: "alias", mensaje: error.message };
      return { tipo: "desconocido", mensaje: error.message };
    }
    if (error.status < 500) return { tipo: "api", mensaje: error.message };
  }
  // Red, 5xx, o cualquier fallo que `resolverCaso` no tipa: mismo mensaje
  // generico que un 5xx, porque de ninguno de los dos se sabe si el servidor
  // llego a hacer algo (D-016 de `api.ts` es la misma idea: un error sin
  // forma conocida no se disfraza de mas informacion de la que hay).
  return { tipo: "conexion" };
}

/**
 * Si el caso tiene que VOLVER a la lista tras este error (D3): por defecto
 * si, salvo los dos 409 que dicen que el caso ya no esta pendiente -el
 * servidor ya lo saco de la cola, asi que devolverlo a la vista mentiria
 * sobre su estado-.
 */
function debeVolverALaLista(error: ErrorDeEnvio): boolean {
  return error.tipo !== "ya_resuelto" && error.tipo !== "no_pendiente";
}

/**
 * Panel lateral de resolucion: las tres formas de cerrar un caso de la
 * bandeja (plano de #39, D3 y D7). Vive DENTRO de un `Dialogo` (D6); no
 * gestiona su propio foco, overlay ni Escape -eso es responsabilidad del
 * primitivo-.
 *
 * Llama a `resolverCaso` el mismo, pero el ciclo de vida de la lista
 * (remocion optimista, restauracion, el aviso de exito) lo decide quien lo
 * monta -`BandejaIdentificacion`- a traves de las cuatro `on*`:
 *
 * - `onEnviarInicio`: se llama SINCRONICAMENTE al pulsar el boton principal,
 *   antes de esperar la red, para que la bandeja pueda sacar el caso de la
 *   lista en el mismo instante (D3: "al enviar, el caso sale de la lista").
 * - `onEnviandoCambia`: para que la bandeja pase `bloqueado` al `Dialogo`
 *   (mientras se envia, ni Escape ni el fondo cierran el panel).
 * - `onFalla`: si el error dice que el caso debe volver a la lista.
 * - `onExito`: para que la bandeja cierre el panel y muestre el aviso.
 */
export default function PanelResolucion({
  idTitulo,
  caso,
  modo,
  onCancelar,
  onEnviarInicio,
  onEnviandoCambia,
  onFalla,
  onExito,
  onRecargarTodo,
}: {
  idTitulo: string;
  caso: CasoIdentificacion;
  modo: ModoResolucion;
  onCancelar: () => void;
  onEnviarInicio: () => void;
  onEnviandoCambia: (enviando: boolean) => void;
  onFalla: (volvioALaLista: boolean) => void;
  onExito: (info: { obraTitulo: string | null; esDescarte: boolean }) => void;
  onRecargarTodo: () => void;
}): ReactElement {
  const [obraElegida, setObraElegida] = useState<ObraElegida | null>(
    obraElegidaInicial(modo),
  );
  const [textoBusqueda, setTextoBusqueda] = useState("");
  const [nota, setNota] = useState("");
  const [enviando, setEnviando] = useState(false);
  const [error, setError] = useState<ErrorDeEnvio | null>(null);

  const idNota = useId();
  const idCampoBusqueda = useId();

  const textoDiferido = useValorDiferido(textoBusqueda, DEBOUNCE_TECLEO_MS);
  const textoDeBusqueda = textoDiferido.trim();

  const notaEsValida = nota.trim() !== "";
  const puedeEnviar =
    !enviando &&
    notaEsValida &&
    (modo.tipo === "descarte" || obraElegida !== null);

  let textoBoton: string;
  if (enviando) textoBoton = "Guardando…";
  else if (modo.tipo === "descarte") textoBoton = "Descartar registro";
  else if (obraElegida) textoBoton = `Asignar a ${obraElegida.titulo}`;
  else textoBoton = "Selecciona una obra";

  async function enviar() {
    if (!puedeEnviar) return;
    setEnviando(true);
    onEnviandoCambia(true);
    setError(null);
    onEnviarInicio();

    const notaRecortada = nota.trim();
    const cuerpo: ResolucionDeCaso =
      modo.tipo === "descarte"
        ? { decision: "descartar", nota: notaRecortada }
        : {
            decision: "asignar",
            obra_id: obraElegida!.id,
            nota: notaRecortada,
          };
    // El sello es la propuesta que se mostro. Sin el, el servidor mediria la
    // aceptacion contra el historial de este momento, que puede haber cambiado.
    if (caso.sugerencia.sello) {
      cuerpo.sello = caso.sugerencia.sello;
    }

    try {
      await resolverCaso(caso.id, cuerpo);
      setEnviando(false);
      onEnviandoCambia(false);
      onExito({
        esDescarte: modo.tipo === "descarte",
        obraTitulo: modo.tipo === "descarte" ? null : obraElegida!.titulo,
      });
    } catch (err) {
      setEnviando(false);
      onEnviandoCambia(false);
      const clasificado = clasificarError(err);
      setError(clasificado);
      onFalla(debeVolverALaLista(clasificado));
    }
  }

  return (
    <div className="panel-resolucion">
      <header className="panel-resolucion-cabecera">
        <p className="panel-resolucion-etiqueta">Panel de resolución</p>
        <h2 id={idTitulo} className="panel-resolucion-titulo">
          {TITULO_POR_MODO[modo.tipo]}
        </h2>
        <button
          type="button"
          className="dialogo-cerrar"
          aria-label="Cerrar panel de resolución"
          onClick={onCancelar}
          disabled={enviando}
        >
          <IconoCerrar />
        </button>
      </header>

      <div className="panel-resolucion-cuerpo">
        <Comparacion caso={caso} obra={obraElegida} />

        {modo.tipo === "descarte" ? (
          <section className="panel-aviso-descarte" role="note">
            <p className="panel-aviso-descarte-titulo">
              Este registro no se asignará a ninguna obra
            </p>
            <p className="muted">
              Queda como descartado: no pondera en el reparto ni sale en el
              listado público de ONI. La decisión y tu nota quedan en la
              bitácora.
            </p>
          </section>
        ) : (
          <>
            {modo.tipo === "busqueda" && (
              <section className="panel-busqueda">
                <span className="muted">Buscar en el catálogo</span>
                <label htmlFor={idCampoBusqueda}>Título de la obra</label>
                <input
                  id={idCampoBusqueda}
                  type="text"
                  autoComplete="off"
                  value={textoBusqueda}
                  onChange={(e) => setTextoBusqueda(e.target.value)}
                />
                {textoDeBusqueda !== "" && (
                  <ResultadosDeBusqueda
                    texto={textoDeBusqueda}
                    obraElegidaId={obraElegida?.id ?? null}
                    onElegir={(obra) =>
                      setObraElegida({
                        id: obra.id,
                        titulo: obra.titulo,
                        anio: obra.anio,
                        genero: obra.genero,
                      })
                    }
                  />
                )}
              </section>
            )}
          </>
        )}

        <section className="panel-nota">
          <div className="panel-nota-cabecera">
            <label htmlFor={idNota}>Nota *</label>
            <span className="muted">
              {nota.length}/{MAX_NOTA}
            </span>
          </div>
          <textarea
            id={idNota}
            value={nota}
            maxLength={MAX_NOTA}
            rows={4}
            required
            disabled={enviando}
            onChange={(e) => setNota(e.target.value)}
          />
          <p className="muted">
            La nota es obligatoria para dejar trazabilidad de la decisión.
          </p>
        </section>

        {error && <ErrorDePanel error={error} onRecargar={onRecargarTodo} />}
      </div>

      <footer className="panel-resolucion-pie">
        <button
          type="button"
          className="boton-secundario"
          disabled={enviando}
          onClick={onCancelar}
        >
          Cancelar
        </button>
        <button
          type="button"
          className="boton-primario"
          disabled={!puedeEnviar}
          onClick={() => void enviar()}
        >
          {textoBoton}
        </button>
      </footer>
    </div>
  );
}

/** Lado a lado: lo que llego en el reporte contra la obra del catalogo. */
function Comparacion({
  caso,
  obra,
}: {
  caso: CasoIdentificacion;
  obra: ObraElegida | null;
}): ReactElement {
  const contra = obra?.titulo ?? "";
  return (
    <div
      className={`comparacion${obra ? "" : " comparacion-sola"}`}
      role="group"
      aria-label="Reportado vs catálogo"
    >
      <div className="comparacion-lado" role="group" aria-label="Reportado">
        <span className="comparacion-etiqueta">Reportado</span>
        <p className="comparacion-titulo">
          <TituloComparado texto={caso.titulo} contra={obra ? contra : null} />
        </p>
        {caso.titulo_original !== "" && (
          <p className="comparacion-original">
            <TituloComparado
              texto={caso.titulo_original}
              contra={obra ? contra : null}
            />
          </p>
        )}
        <p className="comparacion-meta">
          {nombreDeFuente(caso.fuente)} · {formatearPeriodo(caso.periodo)}
        </p>
      </div>
      {obra && (
        <div
          className="comparacion-lado comparacion-catalogo"
          role="group"
          aria-label="Catálogo"
        >
          <span className="comparacion-etiqueta">Catálogo</span>
          <p className="comparacion-titulo">
            <TituloComparado
              texto={obra.titulo}
              contra={`${caso.titulo} ${caso.titulo_original}`}
            />
          </p>
          {(obra.anio !== undefined || obra.genero !== undefined) && (
            <p className="comparacion-meta">
              {[obra.anio, obra.genero]
                .filter((v) => v !== undefined)
                .join(" · ")}
            </p>
          )}
          {obra.puntaje !== undefined && (
            <MedidorConfianza puntaje={obra.puntaje} />
          )}
        </div>
      )}
    </div>
  );
}

/** Un titulo con las palabras que no estan en `contra` resaltadas. */
function TituloComparado({
  texto,
  contra,
}: {
  texto: string;
  contra: string | null;
}): ReactElement {
  if (contra === null) return <>{texto}</>;
  const segmentos = compararTitulos(texto, contra);
  if (segmentos.every((s) => !s.distinto)) return <>{texto}</>;
  return (
    <>
      {segmentos.map((s, i) => {
        if (!s.distinto) return <Fragment key={i}>{s.texto}</Fragment>;
        const espacio = /^\s*/.exec(s.texto)?.[0] ?? "";
        return (
          <Fragment key={i}>
            {espacio}
            <mark className="diferencia">{s.texto.slice(espacio.length)}</mark>
          </Fragment>
        );
      })}
    </>
  );
}

/**
 * La lista de resultados de "Buscar otra obra". Se monta SOLO cuando hay
 * texto (D7): con el campo vacio, `PanelResolucion` ni la monta, asi que no
 * hay peticion que evitar aqui adentro.
 */
function ResultadosDeBusqueda({
  texto,
  obraElegidaId,
  onElegir,
}: {
  texto: string;
  obraElegidaId: string | null;
  onElegir: (obra: Obra) => void;
}): ReactElement {
  const lista = useLista<Obra>(
    `/api/obras?titulo=${encodeURIComponent(texto)}&limite=5`,
    esObra,
  );

  if (lista.estado === "cargando") {
    return (
      <p className="muted" role="status">
        Buscando…
      </p>
    );
  }
  if (lista.estado === "error") {
    return (
      <p className="panel-error" role="alert">
        No pudimos buscar en el catálogo: {lista.mensaje}
      </p>
    );
  }
  if (lista.estado === "ilegible") {
    return (
      <p className="panel-error" role="alert">
        La búsqueda no llegó como una lista de obras legible.
      </p>
    );
  }
  if (lista.elementos.length === 0) {
    return <p className="muted">No encontramos obras con ese título.</p>;
  }
  return (
    <ul className="panel-resultados">
      {lista.elementos.map((obra) => (
        <li key={obra.id}>
          <button
            type="button"
            className="panel-resultado"
            aria-pressed={obraElegidaId === obra.id}
            onClick={() => onElegir(obra)}
          >
            <span>{obra.titulo}</span>
            <span className="muted">
              {obra.anio} · {obra.genero}
            </span>
          </button>
        </li>
      ))}
    </ul>
  );
}

function ErrorDePanel({
  error,
  onRecargar,
}: {
  error: ErrorDeEnvio;
  onRecargar: () => void;
}): ReactElement {
  switch (error.tipo) {
    case "conexion":
      return (
        <p className="panel-error" role="alert">
          No pudimos guardar la resolución. Revisa tu conexión e inténtalo de
          nuevo; tu nota sigue aquí.
        </p>
      );
    case "api":
      return (
        <p className="panel-error" role="alert">
          No pudimos guardar la resolución: {error.mensaje}. Tu nota sigue aquí.
        </p>
      );
    case "ya_resuelto":
      return (
        <div className="panel-error" role="alert">
          <p>
            <strong>Otra persona resolvió este caso antes</strong>
          </p>
          <p>Recarga el caso para consultar la decisión más reciente.</p>
          <button
            type="button"
            className="boton-secundario"
            onClick={onRecargar}
          >
            Recargar caso
          </button>
        </div>
      );
    case "no_pendiente":
      return (
        <div className="panel-error" role="alert">
          <p>
            <strong>Este caso ya no está pendiente</strong>
          </p>
          <p>La cascada de identificación lo resolvió mientras lo revisabas.</p>
          <button
            type="button"
            className="boton-secundario"
            onClick={onRecargar}
          >
            Recargar caso
          </button>
        </div>
      );
    case "desconocido":
      return (
        <div className="panel-error" role="alert">
          <p>
            <strong>El caso cambió mientras lo revisabas</strong>
          </p>
          <p>{error.mensaje}</p>
          <button
            type="button"
            className="boton-secundario"
            onClick={onRecargar}
          >
            Recargar caso
          </button>
        </div>
      );
    case "alias":
      return (
        <div className="panel-error" role="alert">
          <p>
            <strong>Ese identificador ya apunta a otra obra</strong>
          </p>
          <p>{error.mensaje}</p>
          <p>
            Asigna el caso a esa obra o descártalo: corregir una decisión
            anterior no está disponible.
          </p>
        </div>
      );
  }
}

function IconoCerrar() {
  return (
    <svg
      viewBox="0 0 24 24"
      width="18"
      height="18"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      <path d="M6 6l12 12M18 6L6 18" />
    </svg>
  );
}
