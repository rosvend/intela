import type { components } from "../contrato";

type Esquemas = components["schemas"];
export type TurnoAgente = Esquemas["TurnoAgente"];
export type EventoHerramienta = Esquemas["EventoAgenteHerramienta"];
export type EventoRespuesta = Esquemas["EventoAgenteRespuesta"];
export type EventoError = Esquemas["EventoAgenteError"];

/** El servidor rechaza un historial mas largo (api/openapi.yaml, ConsultaAgente). */
export const MAX_HISTORIAL = 20;

/** Un turno de la conversacion. Vive solo en el estado del componente: no se guarda. */
export type Turno = {
  rol: TurnoAgente["rol"];
  texto: string;
  herramientas: string[];
  estado: "pendiente" | "listo" | "error";
  parcial?: boolean;
  restringida?: boolean;
};

export function turnoPendiente(): Turno {
  return { rol: "asistente", texto: "", herramientas: [], estado: "pendiente" };
}

const esObjeto = (d: unknown): d is Record<string, unknown> =>
  typeof d === "object" && d !== null;

/** Aplica un evento SSE al turno del asistente. Lo que no encaja en el contrato se ignora. */
export function aplicarEvento(t: Turno, nombre: string, datos: unknown): Turno {
  if (!esObjeto(datos)) return t;
  switch (nombre) {
    case "tool_call":
      if (typeof datos.herramienta !== "string") return t;
      return { ...t, herramientas: [...t.herramientas, datos.herramienta] };
    case "answer":
      if (typeof datos.texto !== "string") return t;
      return {
        ...t,
        texto: datos.texto,
        estado: "listo",
        parcial: datos.parcial === true,
        restringida: datos.restringida === true,
      };
    case "error":
      if (typeof datos.mensaje !== "string") return t;
      return { ...t, texto: datos.mensaje, estado: "error" };
    default:
      return t;
  }
}

const cerrado = (t: Turno | undefined): t is Turno =>
  t !== undefined && t.estado === "listo" && t.texto.trim() !== "";

/**
 * Lo que se reenvia como contexto: pares pregunta-respuesta cerrados, los
 * ultimos MAX_HISTORIAL. Una pregunta cuya respuesta fallo o sigue en curso no
 * va: el modelo la leeria como contexto sin respuesta.
 */
export function historialPara(turnos: readonly Turno[]): TurnoAgente[] {
  return turnos
    .filter((t, i) => {
      if (!cerrado(t)) return false;
      if (t.rol !== "usuario") return true;
      const respuesta = turnos[i + 1];
      return cerrado(respuesta) && respuesta.rol === "asistente";
    })
    .map((t) => ({ rol: t.rol, texto: t.texto }))
    .slice(-MAX_HISTORIAL);
}
