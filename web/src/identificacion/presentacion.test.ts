import { describe, expect, it } from "vitest";
import {
  compararTitulos,
  contarPorEstado,
  explicarCandidata,
  filasDeComparacion,
  formatearPeriodo,
  nivelDePuntaje,
  nombreDeFuente,
  textoDeSugerencia,
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

const casoBase = {
  id: "uso-1",
  titulo: "La Niña T3 E12",
  titulo_original: "La niña — Capítulo 12",
  fuente: "caracol",
  modalidad: "tv",
  reporte_id: "ING-1",
  periodo: "2024-11",
  ids_fuente: "ID_Ficha=48213\nID_Emision=991204",
  evidencia: "",
  estado: "pendiente",
  candidatos: [],
  obra_asignada: null,
  resuelto_por: null,
  resuelto_en: null,
  ultima_actualizacion: "",
  nota: null,
  sugerencia: sugerenciaNinguna(),
} satisfies CasoIdentificacion;

describe("filasDeComparacion", () => {
  it("sin obra elegida pinta solo lo reportado, sin juicio por fila", () => {
    const filas = filasDeComparacion(casoBase, null);
    expect(filas.map((f) => f.campo)).toEqual([
      "titulo",
      "anio",
      "genero",
      "modalidad",
      "periodo",
      "ids",
    ]);
    expect(filas.every((f) => f.estado === null)).toBe(true);
    expect(filas.every((f) => f.catalogo === null)).toBe(true);
    expect(filas.find((f) => f.campo === "titulo")?.reportado).toBe(
      "La Niña T3 E12",
    );
    expect(filas.find((f) => f.campo === "modalidad")?.reportado).toBe("TV");
    expect(filas.find((f) => f.campo === "periodo")?.reportado).toBe(
      "nov 2024",
    );
    expect(filas.find((f) => f.campo === "ids")?.reportado).toBe(
      "ID_Ficha: 48213\nID_Emision: 991204",
    );
  });

  it("el mismo orden de filas con obra: el titulo distinto se marca y lo que un lado no trae falta", () => {
    const filas = filasDeComparacion(casoBase, {
      id: "obra-1",
      titulo: "La Niña",
      anio: 2016,
      genero: "Drama",
    });
    expect(filas.map((f) => [f.campo, f.estado])).toEqual([
      ["titulo", "distinto"],
      ["anio", "falta"],
      ["genero", "falta"],
      ["modalidad", "falta"],
      ["periodo", "falta"],
      ["ids", "falta"],
    ]);
    expect(filas[0]?.catalogo).toBe("La Niña");
    expect(filas[1]).toMatchObject({ reportado: null, catalogo: "2016" });
    expect(filas[2]).toMatchObject({ reportado: null, catalogo: "Drama" });
  });

  it("el titulo coincide si el catalogo es igual al titulo o al original, sin tildes ni mayusculas", () => {
    const igualAlOriginal = filasDeComparacion(
      { ...casoBase, titulo_original: "LA NINA" },
      { id: "obra-1", titulo: "La Niña" },
    );
    expect(igualAlOriginal[0]?.estado).toBe("coincide");
    const igualAlTitulo = filasDeComparacion(
      { ...casoBase, titulo: "la niña", titulo_original: "" },
      { id: "obra-1", titulo: "La  Niña." },
    );
    expect(igualAlTitulo[0]?.estado).toBe("coincide");
  });

  it("una obra de la busqueda sin anio ni genero los deja como faltantes en ambos lados", () => {
    const filas = filasDeComparacion(casoBase, { id: "obra-9", titulo: "X" });
    expect(filas[1]).toMatchObject({
      reportado: null,
      catalogo: null,
      estado: "falta",
    });
  });

  it("sin ids de la fuente, la fila de ids queda vacia", () => {
    const filas = filasDeComparacion({ ...casoBase, ids_fuente: "" }, null);
    expect(filas.find((f) => f.campo === "ids")?.reportado).toBeNull();
  });
});

describe("textoDeSugerencia", () => {
  it("nombra la obra propuesta, propone descartar o no dice nada", () => {
    expect(
      textoDeSugerencia({
        ...sugerenciaNinguna(),
        decision: "asignar",
        obra_id: "obra-1",
        titulo: "La Niña",
      }),
    ).toBe("Sugerencia: asignar a La Niña.");
    expect(
      textoDeSugerencia({ ...sugerenciaNinguna(), decision: "descartar" }),
    ).toBe("Sugerencia: descartar este registro.");
    expect(textoDeSugerencia(sugerenciaNinguna())).toBeNull();
  });
});
