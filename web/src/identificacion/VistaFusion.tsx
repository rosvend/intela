import { useEffect, useId, useRef, useState, type ReactElement } from "react";
import "../revision.css";
import "./fusion.css";
import { esObra, type Obra } from "../catalogo/tipos";
import { useLista } from "../catalogo/useLista";
import { DEBOUNCE_TECLEO_MS, useValorDiferido } from "../useValorDiferido";
import ComparacionFusion from "./ComparacionFusion";
import { IconoInfo, IconoLupa } from "./iconos";
import MedidorConfianza from "./MedidorConfianza";
import {
  explicarCandidata,
  prefiereQuieto,
  textoDeSugerencia,
  type ObraComparada,
} from "./presentacion";
import {
  MAX_NOTA,
  clasificarError,
  debeVolverALaLista,
  resolverCaso,
  type ErrorDeEnvio,
  type ResolucionDeCaso,
} from "./resolucion";
import type { CasoIdentificacion } from "./tipos";

/** Lo que dura la tarjeta reportada entrando en la del catalogo (fusion.css). */
export const DURACION_UNION_MS = 480;

type Decision = "asignar" | "descartar";

/**
 * Un caso de la cola como una union: lo reportado a la izquierda, la obra del
 * catalogo a la derecha, y la decision debajo. Ninguna candidata llega elegida
 * (ADR 0007): la persona elige, o busca, o dice que no es ninguna.
 *
 * El ciclo de vida de la cola (remocion optimista, devolverlo, avanzar al
 * siguiente) lo decide quien la monta, por las `on*`.
 */
export default function VistaFusion({
  caso,
  onEnviarInicio,
  onEnviandoCambia,
  onFalla,
  onExito,
  onRecargarTodo,
}: {
  caso: CasoIdentificacion;
  onEnviarInicio: () => void;
  onEnviandoCambia: (enviando: boolean) => void;
  onFalla: (volvioALaLista: boolean) => void;
  onExito: (info: { obraTitulo: string | null; esDescarte: boolean }) => void;
  onRecargarTodo: () => void;
}): ReactElement {
  const [obra, setObra] = useState<ObraComparada | null>(null);
  const [buscando, setBuscando] = useState(caso.candidatos.length === 0);
  // Solo una busqueda abierta a mano toma el foco: abierta por defecto (caso
  // sin candidatas) le robaria las flechas a la navegacion de la cola.
  const [abiertaAMano, setAbiertaAMano] = useState(false);
  const [nota, setNota] = useState("");
  const [enviando, setEnviando] = useState<Decision | null>(null);
  const [unida, setUnida] = useState(false);
  const [error, setError] = useState<ErrorDeEnvio | null>(null);
  const temporizador = useRef<number>();
  const idNota = useId();
  const idPista = useId();
  const nombreGrupo = useId();

  useEffect(() => () => window.clearTimeout(temporizador.current), []);

  const ocupado = enviando !== null || unida;
  const hayNota = nota.trim() !== "";
  const puedeUnir = !ocupado && hayNota && obra !== null;
  const puedeDescartar = !ocupado && hayNota;
  const candidata = caso.candidatos.find((c) => c.obra_id === obra?.id);
  const sugerencia = textoDeSugerencia(caso.sugerencia);

  let pista: string | null = null;
  if (!ocupado && !hayNota) pista = "Escribe una nota para la bitácora.";
  else if (!ocupado && !obra) pista = "Elige una obra para unir.";

  async function enviar(decision: Decision) {
    if (decision === "asignar" ? !puedeUnir : !puedeDescartar) return;
    setEnviando(decision);
    onEnviandoCambia(true);
    setError(null);
    onEnviarInicio();

    const cuerpo: ResolucionDeCaso =
      decision === "descartar"
        ? { decision, nota: nota.trim() }
        : { decision, obra_id: obra!.id, nota: nota.trim() };
    // El sello es la propuesta que se mostro (contrato de #53).
    if (caso.sugerencia.sello) cuerpo.sello = caso.sugerencia.sello;

    try {
      await resolverCaso(caso.id, cuerpo);
    } catch (err) {
      setEnviando(null);
      onEnviandoCambia(false);
      const clasificado = clasificarError(err);
      setError(clasificado);
      onFalla(debeVolverALaLista(clasificado));
      return;
    }

    setEnviando(null);
    onEnviandoCambia(false);
    const info = {
      esDescarte: decision === "descartar",
      obraTitulo: decision === "descartar" ? null : obra!.titulo,
    };
    if (decision === "descartar" || prefiereQuieto()) {
      onExito(info);
      return;
    }
    setUnida(true);
    temporizador.current = window.setTimeout(
      () => onExito(info),
      DURACION_UNION_MS,
    );
  }

  return (
    <article className="fusion" aria-label={`Caso: ${caso.titulo}`}>
      <section className="fusion-candidatas" aria-label="Obras candidatas">
        {sugerencia && <p className="fusion-sugerencia">{sugerencia}</p>}
        <div className="fusion-pastillas">
          {caso.candidatos.length === 0 && (
            <p className="fusion-sin-candidatas">
              No hay obras parecidas en el catálogo.
            </p>
          )}
          {caso.candidatos.map((c) => (
            <label
              key={c.obra_id}
              className={`fusion-pastilla${obra?.id === c.obra_id ? " fusion-pastilla-elegida" : ""}`}
            >
              <input
                type="radio"
                className="solo-lector"
                name={nombreGrupo}
                checked={obra?.id === c.obra_id}
                disabled={ocupado}
                onChange={() =>
                  setObra({
                    id: c.obra_id,
                    titulo: c.titulo,
                    anio: c.anio,
                    genero: c.genero,
                  })
                }
              />
              <span className="fusion-pastilla-obra">
                <span className="fusion-pastilla-titulo">{c.titulo}</span>
                <span className="fusion-pastilla-meta">{c.anio}</span>
              </span>
              <MedidorConfianza puntaje={c.puntaje} />
            </label>
          ))}
          <button
            type="button"
            className="boton-fantasma fusion-buscar"
            aria-expanded={buscando}
            disabled={ocupado}
            onClick={() => {
              setAbiertaAMano(true);
              setBuscando((b) => !b);
            }}
          >
            <IconoLupa />
            Buscar otra obra
          </button>
        </div>
        {buscando && (
          <BusquedaDeObra
            enfocar={abiertaAMano}
            obraElegidaId={obra?.id ?? null}
            deshabilitada={ocupado}
            onElegir={(o) =>
              setObra({
                id: o.id,
                titulo: o.titulo,
                anio: o.anio,
                genero: o.genero,
              })
            }
          />
        )}
      </section>

      <ComparacionFusion caso={caso} obra={obra} unida={unida} />

      {candidata && !unida && (
        <p className="fusion-porque">{explicarCandidata(candidata)}</p>
      )}

      {error && <ErrorDeEnvioAviso error={error} onRecargar={onRecargarTodo} />}

      <div className="fusion-acciones">
        <div className="fusion-nota">
          <label htmlFor={idNota}>Nota para la bitácora</label>
          <input
            id={idNota}
            type="text"
            autoComplete="off"
            value={nota}
            maxLength={MAX_NOTA}
            required
            disabled={ocupado}
            placeholder="Por qué decides esto"
            aria-describedby={pista ? idPista : undefined}
            onChange={(e) => setNota(e.target.value)}
          />
          <span className="fusion-contador" aria-hidden="true">
            {nota.length}/{MAX_NOTA}
          </span>
        </div>
        <div className="fusion-botones">
          <button
            type="button"
            className="boton-secundario"
            disabled={!puedeDescartar}
            onClick={() => void enviar("descartar")}
          >
            {enviando === "descartar" ? "Guardando…" : "No es ninguna"}
          </button>
          <button
            type="button"
            className="boton-primario"
            disabled={!puedeUnir}
            onClick={() => void enviar("asignar")}
          >
            {enviando === "asignar" ? "Guardando…" : "Unir con esta obra"}
          </button>
        </div>
        {pista && (
          <p id={idPista} className="fusion-pista">
            {pista}
          </p>
        )}
      </div>

      <p className="fusion-pie">
        <IconoInfo />
        <span>
          La decisión queda en la bitácora con tu nota y no se puede deshacer.
        </span>
      </p>
    </article>
  );
}

/**
 * "Buscar otra obra" (D7): el campo y sus resultados. Los resultados se
 * montan solo con texto, asi que con el campo vacio no hay peticion.
 */
function BusquedaDeObra({
  enfocar,
  obraElegidaId,
  deshabilitada,
  onElegir,
}: {
  enfocar: boolean;
  obraElegidaId: string | null;
  deshabilitada: boolean;
  onElegir: (obra: Obra) => void;
}): ReactElement {
  const [texto, setTexto] = useState("");
  const diferido = useValorDiferido(texto, DEBOUNCE_TECLEO_MS).trim();
  const id = useId();
  return (
    <div className="fusion-busqueda">
      <label htmlFor={id} className="solo-lector">
        Título de la obra
      </label>
      <input
        id={id}
        type="search"
        autoComplete="off"
        autoFocus={enfocar}
        placeholder="Título de la obra en el catálogo"
        value={texto}
        disabled={deshabilitada}
        onChange={(e) => setTexto(e.target.value)}
      />
      {diferido !== "" && (
        <ResultadosDeBusqueda
          texto={diferido}
          obraElegidaId={obraElegidaId}
          onElegir={onElegir}
        />
      )}
    </div>
  );
}

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
      <p className="fusion-busqueda-nota" role="status">
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
    return (
      <p className="fusion-busqueda-nota">
        No encontramos obras con ese título.
      </p>
    );
  }
  return (
    <ul className="fusion-resultados">
      {lista.elementos.map((obra) => (
        <li key={obra.id}>
          <button
            type="button"
            className="fusion-resultado"
            aria-pressed={obraElegidaId === obra.id}
            onClick={() => onElegir(obra)}
          >
            <span>{obra.titulo}</span>
            <span className="fusion-pastilla-meta">
              {obra.anio} · {obra.genero}
            </span>
          </button>
        </li>
      ))}
    </ul>
  );
}

function ErrorDeEnvioAviso({
  error,
  onRecargar,
}: {
  error: ErrorDeEnvio;
  onRecargar: () => void;
}): ReactElement {
  const recargar = (
    <button type="button" className="boton-secundario" onClick={onRecargar}>
      Recargar caso
    </button>
  );
  switch (error.tipo) {
    case "conexion":
      return (
        <p className="panel-error" role="alert">
          No pudimos guardar la decisión. Revisa tu conexión e inténtalo de
          nuevo; tu nota sigue aquí.
        </p>
      );
    case "api":
      return (
        <p className="panel-error" role="alert">
          No pudimos guardar la decisión: {error.mensaje}. Tu nota sigue aquí.
        </p>
      );
    case "ya_resuelto":
      return (
        <div className="panel-error" role="alert">
          <p>
            <strong>Otra persona resolvió este caso antes</strong>
          </p>
          <p>Recarga el caso para consultar la decisión más reciente.</p>
          {recargar}
        </div>
      );
    case "no_pendiente":
      return (
        <div className="panel-error" role="alert">
          <p>
            <strong>Este caso ya no está pendiente</strong>
          </p>
          <p>La cascada de identificación lo resolvió mientras lo revisabas.</p>
          {recargar}
        </div>
      );
    case "desconocido":
      return (
        <div className="panel-error" role="alert">
          <p>
            <strong>El caso cambió mientras lo revisabas</strong>
          </p>
          <p>{error.mensaje}</p>
          {recargar}
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
            Une el caso a esa obra o descártalo: corregir una decisión anterior
            no está disponible.
          </p>
        </div>
      );
  }
}
