import { describe, expect, it } from "vitest";
import {
  agruparPorObra,
  estadoDeObra,
  fechaLlana,
  nombreFuente,
  nombreFuentes,
  nombrePeriodo,
  numeroLlano,
  porcentajeLlano,
  primerNombre,
  sumarImportes,
} from "./presentacion";

describe("nombrePeriodo", () => {
  it("un periodo mensual AAAA-MM se lee como mes y año", () => {
    expect(nombrePeriodo("2025-01")).toBe("enero 2025");
    expect(nombrePeriodo("2026-12")).toBe("diciembre 2026");
  });

  it("un periodo semestral AAAA-S se lee como semestre", () => {
    expect(nombrePeriodo("2025-1")).toBe("1.er semestre 2025");
    expect(nombrePeriodo("2025-2")).toBe("2.º semestre 2025");
  });

  it("un año solo se lee como año, y lo desconocido pasa tal cual", () => {
    expect(nombrePeriodo("2025")).toBe("año 2025");
    expect(nombrePeriodo("2025-13")).toBe("2025-13");
    expect(nombrePeriodo("")).toBe("");
  });
});

describe("nombreFuente", () => {
  it("traduce las fuentes conocidas y capitaliza las demas", () => {
    expect(nombreFuente("caracol")).toBe("Caracol Televisión");
    expect(nombreFuente("rcn")).toBe("RCN Televisión");
    expect(nombreFuente("netflix")).toBe("Netflix");
    expect(nombreFuente("canal-uno")).toBe("Canal uno");
  });

  it("una lista separada por comas se une en lenguaje llano", () => {
    expect(nombreFuentes("caracol, rcn")).toBe(
      "Caracol Televisión y RCN Televisión",
    );
    expect(nombreFuentes("caracol")).toBe("Caracol Televisión");
  });
});

describe("primerNombre", () => {
  it("toma el primer nombre, o un saludo neutro si no hay", () => {
    expect(primerNombre("Ana Escritora")).toBe("Ana");
    expect(primerNombre("  ")).toBe("");
  });
});

describe("estadoDeObra", () => {
  it("una declaracion completa esta lista para pagar", () => {
    expect(estadoDeObra("completa").tipo).toBe("lista");
    expect(estadoDeObra("completa").etiqueta).toBe("Lista para pagar");
  });

  it("una declaracion incompleta queda en reserva", () => {
    for (const e of ["incompleta", "declaracion_incompleta", "en_reserva"]) {
      expect(estadoDeObra(e).tipo, e).toBe("reserva");
    }
    expect(estadoDeObra("incompleta").etiqueta).toBe(
      "En reserva: falta completar la declaración",
    );
  });

  it("un estado desconocido no se disfraza de pagable", () => {
    expect(estadoDeObra("otra-cosa").tipo).toBe("otro");
  });
});

describe("sumarImportes", () => {
  it("suma decimales en centavos exactos, sin pasar por float", () => {
    expect(sumarImportes(["0.10", "0.20"])).toBe("0.30");
    expect(sumarImportes(["4800.00", "-1200.00"])).toBe("3600.00");
    expect(sumarImportes([])).toBe("0.00");
  });
});

describe("agruparPorObra", () => {
  it("junta las lineas de la misma obra y conserva el orden de llegada", () => {
    const grupos = agruparPorObra([
      { obra_id: "a", titulo: "A", neto: "100.00" },
      { obra_id: "b", titulo: "B", neto: "50.00" },
      { obra_id: "a", titulo: "A", neto: "25.50" },
    ]);
    expect(grupos).toEqual([
      { obra_id: "a", titulo: "A", neto: "125.50" },
      { obra_id: "b", titulo: "B", neto: "50.00" },
    ]);
  });
});

describe("numeros llanos", () => {
  it("los numeros del backend se leen en es-CO sin perder digitos", () => {
    expect(numeroLlano("1.3")).toBe("1,3");
    expect(numeroLlano("5616")).toBe("5.616");
    expect(numeroLlano("48.000")).toBe("48");
    expect(numeroLlano("abc")).toBe("abc");
  });

  it("un porcentaje se recorta a lo legible", () => {
    expect(porcentajeLlano("60.0000")).toBe("60 %");
    expect(porcentajeLlano("33.3333")).toBe("33,33 %");
  });

  it("una fecha ISO se lee en palabras", () => {
    expect(fechaLlana("2026-02-01T10:00:00Z")).toBe("1 de febrero de 2026");
    expect(fechaLlana("nada")).toBe("nada");
  });
});
