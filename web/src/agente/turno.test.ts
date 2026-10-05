import { describe, expect, it } from "vitest";
import {
  aplicarEvento,
  cerrarTurno,
  enCurso,
  turnoVacio,
  type EventoAgente,
} from "./turno";

const respuesta = { texto: "listo", parcial: false, restringida: false };

describe("turno del agente", () => {
  it("acumula las herramientas en el orden en que llegan, repeticiones incluidas", () => {
    const eventos: EventoAgente[] = [
      { evento: "tool_call", datos: { herramienta: "buscar_obra" } },
      { evento: "tool_call", datos: { herramienta: "buscar_reglamento" } },
      { evento: "tool_call", datos: { herramienta: "buscar_obra" } },
    ];
    const t = eventos.reduce(aplicarEvento, turnoVacio);
    expect(t.herramientas).toEqual([
      "buscar_obra",
      "buscar_reglamento",
      "buscar_obra",
    ]);
    expect(enCurso(t)).toBe(true);
  });

  it("la respuesta cierra el turno y lo que llegue despues se ignora", () => {
    const eventos: EventoAgente[] = [
      { evento: "answer", datos: respuesta },
      { evento: "tool_call", datos: { herramienta: "x" } },
      { evento: "error", datos: { mensaje: "tarde" } },
    ];
    const t = eventos.reduce(aplicarEvento, turnoVacio);
    expect(t).toEqual({ herramientas: [], respuesta, fallo: false });
    expect(enCurso(t)).toBe(false);
  });

  it("un error cierra el turno como fallo", () => {
    const t = aplicarEvento(turnoVacio, {
      evento: "error",
      datos: { mensaje: "el asistente no esta disponible" },
    });
    expect(t.fallo).toBe(true);
    expect(enCurso(t)).toBe(false);
  });

  it("un flujo que se corta sin respuesta se cierra como fallo, nunca en blanco", () => {
    const abierto = aplicarEvento(turnoVacio, {
      evento: "tool_call",
      datos: { herramienta: "buscar_obra" },
    });
    const t = cerrarTurno(abierto);
    expect(t.fallo).toBe(true);
    expect(t.herramientas).toEqual(["buscar_obra"]);
  });

  it("cerrar un turno ya respondido no lo cambia", () => {
    const t = aplicarEvento(turnoVacio, { evento: "answer", datos: respuesta });
    expect(cerrarTurno(t)).toBe(t);
  });
});
