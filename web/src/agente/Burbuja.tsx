import { useEffect, useRef, useState, type FormEvent } from "react";
import {
  ChatBubbleLeftRightIcon,
  PaperAirplaneIcon,
  XMarkIcon,
} from "@heroicons/react/24/outline";
import { apiStream, esErrorDeApi } from "../api";
import ErrorBoundary from "../ErrorBoundary";
import {
  aplicarEvento,
  historialPara,
  turnoPendiente,
  type Turno,
} from "./conversacion";

const NO_DISPONIBLE = "El asistente no está disponible en este momento.";

/**
 * Asistente de solo lectura (#66): burbuja flotante abajo a la izquierda que
 * abre un panel superpuesto. La conversacion vive en este estado -el panel se
 * monta y desmonta, la burbuja no-, asi que sobrevive a cerrar y abrir pero no
 * a recargar la pagina: el servidor no la guarda.
 */
export default function Burbuja() {
  const [abierto, setAbierto] = useState(false);
  const [turnos, setTurnos] = useState<Turno[]>([]);
  const [enviando, setEnviando] = useState(false);

  async function preguntar(mensaje: string) {
    const historial = historialPara(turnos);
    const usuario: Turno = {
      rol: "usuario",
      texto: mensaje,
      herramientas: [],
      estado: "listo",
    };
    setTurnos((ts) => [...ts, usuario, turnoPendiente()]);
    setEnviando(true);
    const actualizarUltimo = (f: (t: Turno) => Turno) =>
      setTurnos((ts) => [...ts.slice(0, -1), f(ts[ts.length - 1])]);
    try {
      await apiStream(
        "/api/agente/consulta",
        { mensaje, historial },
        (nombre, datos) =>
          actualizarUltimo((t) => aplicarEvento(t, nombre, datos)),
      );
      actualizarUltimo((t) =>
        t.estado === "pendiente"
          ? { ...t, estado: "error", texto: NO_DISPONIBLE }
          : t,
      );
    } catch (err) {
      const texto = esErrorDeApi(err) ? err.message : NO_DISPONIBLE;
      actualizarUltimo((t) => ({ ...t, estado: "error", texto }));
    } finally {
      setEnviando(false);
    }
  }

  return (
    <div className="agente">
      {abierto && (
        <section
          className="agente-panel"
          role="dialog"
          aria-label="Asistente de Intela"
          onKeyDown={(e) => e.key === "Escape" && setAbierto(false)}
        >
          <header className="agente-cabecera">
            <p className="agente-titulo">Asistente</p>
            <button
              type="button"
              className="agente-cerrar"
              aria-label="Cerrar asistente"
              onClick={() => setAbierto(false)}
            >
              <XMarkIcon aria-hidden="true" />
            </button>
          </header>
          <ErrorBoundary>
            <Conversacion turnos={turnos} />
            <Entrada deshabilitada={enviando} alEnviar={preguntar} />
          </ErrorBoundary>
        </section>
      )}
      {!abierto && (
        <button
          type="button"
          className="agente-burbuja"
          aria-label="Abrir asistente"
          onClick={() => setAbierto(true)}
        >
          <ChatBubbleLeftRightIcon aria-hidden="true" />
        </button>
      )}
    </div>
  );
}

function Conversacion({ turnos }: { turnos: readonly Turno[] }) {
  const fin = useRef<HTMLDivElement>(null);
  useEffect(() => {
    fin.current?.scrollIntoView?.({ block: "end" });
  }, [turnos]);

  return (
    <div className="agente-mensajes" aria-live="polite">
      {turnos.length === 0 && (
        <p className="agente-vacio">
          Pregunta por el reglamento, una corrida o la cola de ONI. Solo
          consulto: no modifico ni apruebo nada.
        </p>
      )}
      {turnos.map((t, i) => (
        <TurnoVista key={i} turno={t} />
      ))}
      <div ref={fin} />
    </div>
  );
}

function TurnoVista({ turno }: { turno: Turno }) {
  if (turno.rol === "usuario") {
    return (
      <p className="agente-mensaje agente-mensaje-usuario">{turno.texto}</p>
    );
  }
  if (turno.estado === "pendiente") {
    return <p className="agente-mensaje agente-pensando">Pensando…</p>;
  }
  if (turno.estado === "error") {
    return (
      <p className="agente-mensaje agente-error" role="alert">
        {turno.texto}
      </p>
    );
  }
  return (
    <p className="agente-mensaje agente-mensaje-asistente">{turno.texto}</p>
  );
}

function Entrada({
  deshabilitada,
  alEnviar,
}: {
  deshabilitada: boolean;
  alEnviar: (mensaje: string) => void;
}) {
  const [texto, setTexto] = useState("");

  function enviar(e?: FormEvent) {
    e?.preventDefault();
    const mensaje = texto.trim();
    if (!mensaje || deshabilitada) return;
    setTexto("");
    alEnviar(mensaje);
  }

  return (
    <form className="agente-entrada" onSubmit={enviar}>
      <textarea
        aria-label="Tu pregunta"
        rows={2}
        maxLength={4000}
        value={texto}
        disabled={deshabilitada}
        placeholder="Escribe tu pregunta…"
        onChange={(e) => setTexto(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.shiftKey) enviar(e);
        }}
      />
      <button
        type="submit"
        aria-label="Enviar"
        disabled={deshabilitada || !texto.trim()}
      >
        <PaperAirplaneIcon aria-hidden="true" />
      </button>
    </form>
  );
}
