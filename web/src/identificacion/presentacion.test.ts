import { describe, expect, it } from "vitest";
import {
  compararTitulos,
  contarPorEstado,
  explicarCandidata,
  formatearPeriodo,
  nivelDePuntaje,
  nombreDeFuente,
} from "./presentacion";
import { sugerenciaNinguna, type CasoIdentificacion } from "./tipos";

describe("nivelDePuntaje", () => {
  it("usa las bandas de matching.umbral (0,60) y matching.umbral_banda (0,45)", () => {
    expect(nivelDePuntaje(0.71)).toBe("alta");
    expect(nivelDePuntaje(0.6)).toBe("alta");
    expect(nivelDePuntaje(0.59)).toBe("media");
    expect(nivelDePuntaje(0.45)).toBe("media");
    expect(nivelDePuntaje(0.44)).toBe("baja");
    expect(nivelDePuntaje(0)).toBe("baja");
  });
});

describe("nombreDeFuente", () => {
  it("traduce los slugs conocidos a un nombre legible", () => {
    expect(nombreDeFuente("caracol")).toBe("Caracol TV");
    expect(nombreDeFuente("netflix")).toBe("Netflix");
    expect(nombreDeFuente("rcn")).toBe("RCN");
  });

  it("capitaliza un slug desconocido y deja tal cual un nombre ya legible", () => {
    expect(nombreDeFuente("canal_uno")).toBe("Canal Uno");
    expect(nombreDeFuente("Caracol Televisión")).toBe("Caracol Televisión");
  });
});

describe("formatearPeriodo", () => {
  it("convierte AAAA-MM en mes corto y año", () => {
    expect(formatearPeriodo("2024-11")).toBe("nov 2024");
    expect(formatearPeriodo("2025-01")).toBe("ene 2025");
  });

  it("deja tal cual lo que no tiene forma de mes", () => {
    expect(formatearPeriodo("2025")).toBe("2025");
    expect(formatearPeriodo("2025-13")).toBe("2025-13");
  });
});

describe("compararTitulos", () => {
  it("marca las palabras que no estan en el otro titulo, sin importar tildes ni mayusculas", () => {
    expect(compararTitulos("La Niña T3 E12", "la nina")).toEqual([
      { texto: "La Niña", distinto: false },
      { texto: " T3 E12", distinto: true },
    ]);
  });

  it("no marca la puntuacion suelta", () => {
    expect(compararTitulos("La niña — Capítulo 12", "La Niña")).toEqual([
      { texto: "La niña —", distinto: false },
      { texto: " Capítulo 12", distinto: true },
    ]);
  });

  it("un titulo identico es un solo segmento sin diferencias", () => {
    expect(compararTitulos("La Niña", "La Niña")).toEqual([
      { texto: "La Niña", distinto: false },
    ]);
  });
});

describe("explicarCandidata", () => {
  it("habla en lenguaje llano y nombra los dos titulos comparados", () => {
    const texto = explicarCandidata({
      obra_id: "obra-1",
      titulo: "La Niña",
      anio: 2016,
      genero: "Drama",
      puntaje: 0.71,
      titulo_consultado: "La Niña T3 E12",
    });
    expect(texto).toContain("se parece mucho");
    expect(texto).toContain("“La Niña T3 E12”");
    expect(texto).toContain("“La Niña”");
    expect(texto).not.toMatch(/difuso|cascada|jaro|levenshtein/i);
  });
});

describe("contarPorEstado", () => {
  it("cuenta los tres estados, con cero para los ausentes", () => {
    const base = {
      id: "x",
      titulo: "",
      titulo_original: "",
      fuente: "",
      modalidad: "tv",
      reporte_id: "",
      periodo: "",
      ids_fuente: "",
      evidencia: "",
      candidatos: [],
      obra_asignada: null,
      resuelto_por: null,
      resuelto_en: null,
      ultima_actualizacion: "",
      nota: null,
      sugerencia: sugerenciaNinguna(),
    } satisfies Omit<CasoIdentificacion, "estado">;
    const casos: CasoIdentificacion[] = [
      { ...base, estado: "pendiente" },
      { ...base, estado: "pendiente" },
      { ...base, estado: "descartado" },
    ];
    expect(contarPorEstado(casos)).toEqual({
      pendiente: 2,
      asignado: 0,
      descartado: 1,
    });
  });
});
