import { describe, expect, it } from "vitest";
import {
  ETAPAS_INTERNACIONAL,
  ETAPAS_NACIONAL,
  ETIQUETA_ETAPA,
  estadoDePaso,
  etapasDe,
} from "./etapas";
import { Etapa } from "./tipos";

describe("etapasDe", () => {
  it("el nacional incluye valorizacion y no Fees in Error", () => {
    expect(etapasDe("nacional")).toEqual(ETAPAS_NACIONAL);
    expect(ETAPAS_NACIONAL).toContain("importe_obra");
    expect(ETAPAS_NACIONAL).toContain("importe_titular");
    expect(ETAPAS_NACIONAL).not.toContain("fees_in_error");
  });

  it("el internacional no valoriza y si tiene Fees in Error", () => {
    expect(etapasDe("internacional")).toEqual(ETAPAS_INTERNACIONAL);
    expect(ETAPAS_INTERNACIONAL).not.toContain("importe_obra");
    expect(ETAPAS_INTERNACIONAL).not.toContain("importe_titular");
    expect(ETAPAS_INTERNACIONAL).toContain("fees_in_error");
  });
});

describe("estadoDePaso", () => {
  it.each(ETAPAS_NACIONAL)(
    "en nacional, %s es el paso actual y los anteriores estan hechos",
    (etapa: Etapa) => {
      const pipeline = ETAPAS_NACIONAL;
      expect(estadoDePaso(pipeline, etapa, etapa)).toBe("actual");
      for (const paso of pipeline) {
        const estado = estadoDePaso(pipeline, etapa, paso);
        const iPaso = pipeline.indexOf(paso);
        const iActual = pipeline.indexOf(etapa);
        if (iPaso < iActual) expect(estado).toBe("hecha");
        if (iPaso > iActual) expect(estado).toBe("pendiente");
      }
    },
  );

  it.each(ETAPAS_INTERNACIONAL)(
    "en internacional, %s es el paso actual",
    (etapa: Etapa) => {
      expect(estadoDePaso(ETAPAS_INTERNACIONAL, etapa, etapa)).toBe("actual");
    },
  );

  it("toda etapa tiene etiqueta para pintar el pipeline", () => {
    for (const etapa of [...ETAPAS_NACIONAL, ...ETAPAS_INTERNACIONAL]) {
      expect(ETIQUETA_ETAPA[etapa].length).toBeGreaterThan(0);
    }
  });
});
