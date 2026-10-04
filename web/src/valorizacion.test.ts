import { describe, expect, it } from "vitest";
import type { ValorizacionDeUso } from "./ingresos";
import { ETIQUETA_FACTOR, lineaDeValorizacion } from "./valorizacion";

describe("lineaDeValorizacion", () => {
  it("TV: un termino de cuatro factores, tal cual los manda el backend", () => {
    const v: ValorizacionDeUso = {
      uso_id: "u-1",
      formula: "RD 9.1.1",
      puntos: "5616",
      terminos: [
        {
          producto: "5616",
          factores: [
            { nombre: "ponderacion", valor: "1.3", origen: "parametro" },
            { nombre: "duracion_min", valor: "48", origen: "uso" },
            { nombre: "rating", valor: "9", origen: "uso" },
            { nombre: "emisiones", valor: "10", origen: "uso" },
          ],
        },
      ],
    };
    expect(lineaDeValorizacion(v)).toBe(
      "RD 9.1.1: ponderacion 1.3 × duracion (min) 48 × rating 9 × emisiones 10 = 5616 puntos",
    );
  });

  it("OTT: cada termino entre parentesis, unidos con +", () => {
    const v: ValorizacionDeUso = {
      uso_id: "u-1",
      formula: "RD 9.7",
      puntos: "12200.65",
      terminos: [
        {
          producto: "0.65",
          factores: [
            { nombre: "pb", valor: "1.3", origen: "uso" },
            { nombre: "wa", valor: "0.5", origen: "parametro" },
          ],
        },
        {
          producto: "12000",
          factores: [
            { nombre: "minutos_vistos", valor: "40000", origen: "uso" },
            { nombre: "wb", valor: "0.3", origen: "parametro" },
          ],
        },
        {
          producto: "200",
          factores: [
            { nombre: "vistas", valor: "1000", origen: "uso" },
            { nombre: "wc", valor: "0.2", origen: "parametro" },
          ],
        },
      ],
    };
    expect(lineaDeValorizacion(v)).toBe(
      "RD 9.7: (puntaje base 1.3 × Wa 0.5) + (minutos vistos 40000 × Wb 0.3) + (vistas 1000 × Wc 0.2) = 12200.65 puntos",
    );
  });

  it("cine: un solo factor", () => {
    const v: ValorizacionDeUso = {
      uso_id: "u-1",
      formula: "RD 9.2",
      puntos: "250",
      terminos: [
        {
          producto: "250",
          factores: [{ nombre: "espectadores", valor: "250", origen: "uso" }],
        },
      ],
    };
    expect(lineaDeValorizacion(v)).toBe(
      "RD 9.2: espectadores 250 = 250 puntos",
    );
  });

  it("hay etiqueta para los 13 factores del contrato", () => {
    expect(Object.keys(ETIQUETA_FACTOR).length).toBe(13);
  });
});
