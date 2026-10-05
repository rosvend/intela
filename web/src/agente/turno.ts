import type { components } from "../contrato";

type Esquemas = components["schemas"];
export type RespuestaAgente = Esquemas["EventoAgenteRespuesta"];

/** Un evento SSE de POST /agente/consulta, ya decodificado. */
export type EventoAgente =
  | { evento: "tool_call"; datos: Esquemas["EventoAgenteHerramienta"] }
  | { evento: "answer"; datos: RespuestaAgente }
  | { evento: "error"; datos: Esquemas["EventoAgenteError"] };

/** Lo que el panel pinta de una respuesta del asistente; da igual si los eventos llegaron de a uno o todos juntos. */
export type TurnoAgente = {
  herramientas: string[];
  respuesta: RespuestaAgente | null;
  fallo: boolean;
};

export const turnoVacio: TurnoAgente = {
  herramientas: [],
  respuesta: null,
  fallo: false,
};

export function enCurso(t: TurnoAgente): boolean {
  return t.respuesta === null && !t.fallo;
}

export function aplicarEvento(t: TurnoAgente, e: EventoAgente): TurnoAgente {
  if (!enCurso(t)) return t;
  switch (e.evento) {
    case "tool_call":
      return { ...t, herramientas: [...t.herramientas, e.datos.herramienta] };
    case "answer":
      return { ...t, respuesta: e.datos };
    case "error":
      return { ...t, fallo: true };
  }
}

/** El flujo termino: si no trajo respuesta ni error, el turno es un fallo y no un hueco. */
export function cerrarTurno(t: TurnoAgente): TurnoAgente {
  return enCurso(t) ? { ...t, fallo: true } : t;
}
