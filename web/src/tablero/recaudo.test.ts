import { describe, expect, it } from "vitest";
import {
  Bolsa,
  categoriaDeFuente,
  etiquetaDeFuente,
  periodoMasReciente,
  recaudoPorFuente,
} from "./recaudo";

function bolsa(parcial: Partial<Bolsa>): Bolsa {
  return {
    id: "b",
    usuario_id: "caracol",
    periodo: "2025",
    circuito: "nacional",
    bruto: "0",
    ...parcial,
  };
}

describe("etiquetaDeFuente", () => {
  it("nombra las fuentes conocidas como las dice la casa", () => {
    expect(etiquetaDeFuente("caracol")).toBe("Caracol");
    expect(etiquetaDeFuente("rcn")).toBe("RCN");
  });

  it("una fuente desconocida se capitaliza, sin inventar nombre", () => {
    expect(etiquetaDeFuente("canal-uno")).toBe("Canal uno");
  });
});

describe("categoriaDeFuente", () => {
  it("dice el tipo de usuario; el circuito internacional manda", () => {
    expect(categoriaDeFuente("netflix", "nacional")).toBe("Plataforma");
    expect(categoriaDeFuente("desconocida", "internacional")).toBe(
      "Internacional",
    );
    expect(categoriaDeFuente("desconocida", "nacional")).toBe("");
  });
});

describe("periodoMasReciente", () => {
  it("elige el mayor periodo AAAA o AAAA-MM", () => {
    expect(
      periodoMasReciente([
        bolsa({ periodo: "2024-12" }),
        bolsa({ periodo: "2025-02" }),
        bolsa({ periodo: "2025-01" }),
      ]),
    ).toBe("2025-02");
    expect(periodoMasReciente([])).toBeUndefined();
  });
});

describe("recaudoPorFuente", () => {
  it("agrupa por usuario dentro del periodo, ordena de mayor a menor y suma exacto", () => {
    const r = recaudoPorFuente(
      [
        bolsa({ id: "1", usuario_id: "rcn", bruto: "100.10" }),
        bolsa({ id: "2", usuario_id: "caracol", bruto: "300" }),
        bolsa({
          id: "3",
          usuario_id: "rcn",
          circuito: "internacional",
          bruto: "250.20",
        }),
        bolsa({ id: "4", usuario_id: "caracol", periodo: "2024", bruto: "9" }),
      ],
      "2025",
    );
    expect(r?.periodo).toBe("2025");
    expect(r?.total).toBe("650.30");
    expect(r?.fuentes.map((f) => [f.id, f.valor])).toEqual([
      ["rcn", "350.30"],
      ["caracol", "300.00"],
    ]);
    expect(r?.fuentes[0].etiqueta).toBe("RCN");
  });

  it("sin periodo usa el mas reciente", () => {
    const r = recaudoPorFuente([
      bolsa({ periodo: "2024", bruto: "1" }),
      bolsa({ periodo: "2025", bruto: "2" }),
    ]);
    expect(r?.periodo).toBe("2025");
    expect(r?.total).toBe("2.00");
  });

  it("mas de seis fuentes se pliegan en Otras, nunca un septimo color", () => {
    const bolsas = Array.from({ length: 8 }, (_, i) =>
      bolsa({ id: `b${i}`, usuario_id: `u${i}`, bruto: String(100 - i) }),
    );
    const r = recaudoPorFuente(bolsas, "2025");
    expect(r?.fuentes).toHaveLength(6);
    expect(r?.fuentes[5]).toMatchObject({
      id: "otras",
      etiqueta: "Otras fuentes",
      valor: String(95 + 94 + 93) + ".00",
    });
  });

  it("un periodo anual incluye sus meses", () => {
    const r = recaudoPorFuente(
      [
        bolsa({ periodo: "2025-03", bruto: "5" }),
        bolsa({ periodo: "2025", bruto: "1" }),
      ],
      "2025",
    );
    expect(r?.total).toBe("6.00");
  });

  it("sin bolsas no hay recaudo que mostrar", () => {
    expect(recaudoPorFuente([])).toBeNull();
  });
});
