import { describe, expect, it } from "vitest";
import type { ValorizacionDeUso } from "../ingresos";
import {
  cuadraConBruto,
  fraseDeUso,
  fechaDeAprobacion,
  modalidadDeUso,
  partesDelBruto,
} from "./explicacion";

const deducciones = [
  { concepto: "gastos_administrativos", porcentaje: "10.00", monto: "480.00" },
  { concepto: "bienestar_social", porcentaje: "5.00", monto: "240.00" },
  {
    concepto: "reserva_errores_tecnicos",
    porcentaje: "10.00",
    monto: "480.00",
  },
];

const usoTV: ValorizacionDeUso = {
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

describe("partesDelBruto", () => {
  it("el neto va primero, con la marca, y luego cada descuento de ley", () => {
    const partes = partesDelBruto({ neto: "3600.00", deducciones });
    expect(partes.map((p) => p.id)).toEqual([
      "neto",
      "gastos_administrativos",
      "bienestar_social",
      "reserva_errores_tecnicos",
    ]);
    expect(partes[0].etiqueta).toBe("Tú recibes");
    expect(partes[1].etiqueta).toBe("Gastos administrativos");
  });

  it("cada descuento trae una razon llana y la cita del reglamento", () => {
    const [, gastos] = partesDelBruto({ neto: "3600.00", deducciones });
    expect(gastos.razon.length).toBeGreaterThan(10);
    expect(gastos.cita?.texto).toContain("Ley 44 de 1993");
  });

  it("las partes suman exactamente el bruto", () => {
    const partes = partesDelBruto({ neto: "3600.00", deducciones });
    expect(cuadraConBruto(partes, "4800.00")).toBe(true);
    expect(cuadraConBruto(partes, "4800.01")).toBe(false);
  });
});

describe("usos", () => {
  it("la modalidad se nombra en lenguaje llano", () => {
    expect(modalidadDeUso(usoTV)).toBe("Televisión abierta");
  });

  it("los factores se cuentan en una frase, con los puntos al final", () => {
    expect(fraseDeUso(usoTV)).toBe(
      "Pesa 1,3 por su tipo de obra, dura 48 minutos, tuvo un rating de 9 y se emitió 10 veces. En total suma 5.616 puntos.",
    );
  });
});

describe("fechaDeAprobacion", () => {
  it("toma la firma mas reciente, o nada si no hay firmas", () => {
    expect(
      fechaDeAprobacion([
        { cuando: "2026-02-01T10:00:00Z" },
        { cuando: "2026-02-03T09:00:00Z" },
      ]),
    ).toBe("3 de febrero de 2026");
    expect(fechaDeAprobacion([])).toBe("");
  });
});
