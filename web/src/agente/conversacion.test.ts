import { describe, expect, it } from "vitest";
import {
  MAX_HISTORIAL,
  aplicarEvento,
  historialPara,
  turnoPendiente,
  type Turno,
} from "./conversacion";

describe("aplicarEvento", () => {
  it("acumula las herramientas en orden y cierra con la respuesta", () => {
    let t = turnoPendiente();
    t = aplicarEvento(t, "tool_call", { herramienta: "buscar_reglamento" });
    t = aplicarEvento(t, "tool_call", { herramienta: "listar_oni" });
    t = aplicarEvento(t, "answer", {
      texto: "Listo (RD 9.1.1).",
      parcial: true,
      restringida: false,
    });
    expect(t).toEqual({
      rol: "asistente",
      texto: "Listo (RD 9.1.1).",
      herramientas: ["buscar_reglamento", "listar_oni"],
      estado: "listo",
      parcial: true,
      restringida: false,
    });
  });

  it("un evento error deja el turno en error con el mensaje del servidor", () => {
    const t = aplicarEvento(turnoPendiente(), "error", {
      mensaje: "no disponible",
    });
    expect(t.estado).toBe("error");
    expect(t.texto).toBe("no disponible");
  });

  it("ignora eventos desconocidos o mal formados", () => {
    const t = turnoPendiente();
    expect(aplicarEvento(t, "otro", {})).toBe(t);
    expect(aplicarEvento(t, "answer", { texto: 3 })).toBe(t);
  });
});

describe("historialPara", () => {
  it("manda solo turnos cerrados con texto, como mucho los ultimos MAX_HISTORIAL", () => {
    const turnos: Turno[] = [];
    for (let i = 0; i < MAX_HISTORIAL + 4; i++) {
      turnos.push({
        rol: "usuario",
        texto: `p${i}`,
        herramientas: [],
        estado: "listo",
      });
    }
    turnos.push({ ...turnoPendiente() });
    turnos.push({
      rol: "asistente",
      texto: "fallo",
      herramientas: [],
      estado: "error",
    });
    const h = historialPara(turnos);
    expect(h).toHaveLength(MAX_HISTORIAL);
    expect(h[0]).toEqual({ rol: "usuario", texto: "p4" });
    expect(h.at(-1)).toEqual({
      rol: "usuario",
      texto: `p${MAX_HISTORIAL + 3}`,
    });
  });
});
