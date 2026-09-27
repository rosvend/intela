import { describe, expect, it } from "vitest";
import {
  conteoDeTipo,
  conteosPorTipo,
  contarPorTipo,
  etiquetaDeTipo,
  pendientes,
  totalPendientes,
} from "./anomalias";
import { Alerta } from "./tipos";

function alerta(
  parcial: Partial<Alerta> & Pick<Alerta, "id" | "tipo">,
): Alerta {
  return { detalle: "", ...parcial };
}

const muestra: Alerta[] = [
  alerta({ id: "a1", tipo: "oni", detalle: "obra X" }),
  alerta({ id: "a2", tipo: "oni", detalle: "obra Y" }),
  alerta({ id: "a3", tipo: "duplicado_archivo", detalle: "mismo SHA" }),
  alerta({
    id: "a4",
    tipo: "titular_sin_porcentaje",
    detalle: "tit-9",
    resuelta: true,
  }),
  alerta({
    id: "a5",
    tipo: "reserva_declaracion_incompleta",
    detalle: "obra Z",
  }),
];

describe("conteos de alertas", () => {
  it("cuenta solo las abiertas por tipo", () => {
    expect(contarPorTipo(muestra, "oni")).toBe(2);
    expect(contarPorTipo(muestra, "duplicado_archivo")).toBe(1);
    expect(contarPorTipo(muestra, "titular_sin_porcentaje")).toBe(0);
    expect(contarPorTipo(muestra, "reserva_declaracion_incompleta")).toBe(1);
    expect(contarPorTipo(muestra, "duplicado_registro")).toBe(0);
  });

  it("el total pendiente ignora las resueltas", () => {
    expect(pendientes(muestra)).toHaveLength(4);
    expect(totalPendientes(muestra)).toBe(4);
  });

  it("conteosPorTipo cubre los cinco tipos conocidos", () => {
    const conteos = conteosPorTipo(muestra);
    expect(conteos.oni).toBe(2);
    expect(conteos.duplicado_archivo).toBe(1);
    expect(conteos.duplicado_registro).toBe(0);
    expect(conteos.titular_sin_porcentaje).toBe(0);
    expect(conteos.reserva_declaracion_incompleta).toBe(1);
  });

  it("proyecta un Recurso de alertas a un conteo de tarjeta", () => {
    const listo = conteoDeTipo({ tipo: "listo", datos: muestra }, "oni");
    expect(listo).toEqual({ tipo: "listo", datos: { total: 2 } });
    expect(conteoDeTipo({ tipo: "ausente" }, "oni")).toEqual({
      tipo: "ausente",
    });
    expect(conteoDeTipo({ tipo: "error", mensaje: "caida" }, "oni")).toEqual({
      tipo: "error",
      mensaje: "caida",
    });
  });

  it("un tipo desconocido se pinta con su nombre crudo", () => {
    expect(etiquetaDeTipo("oni")).toBe("ONI");
    expect(etiquetaDeTipo("algo_nuevo")).toBe("algo_nuevo");
  });
});
