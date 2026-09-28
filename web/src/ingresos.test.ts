import { describe, expect, it } from "vitest";
import {
  filtrarIngresos,
  formatearNeto,
  opcionesFiltro,
  rutaExplicar,
  rutaMisIngresos,
  type Ingreso,
} from "./ingresos";

const anaCasa: Ingreso = {
  ref: "proc-2026-01:obra-completa:tit-ana",
  obra_id: "obra-completa",
  obra: "La Casa de las Dos Palmas",
  fuente: "caracol",
  periodo: "2026-01",
  neto: "3600.00",
};

const anaSegundo: Ingreso = {
  ref: "proc-2026-01:obra-ana-2:tit-ana",
  obra_id: "obra-ana-2",
  obra: "El Segundo Guion",
  fuente: "caracol",
  periodo: "2026-01",
  neto: "750.00",
};

const anaFebrero: Ingreso = {
  ref: "proc-2026-02:obra-completa:tit-ana",
  obra_id: "obra-completa",
  obra: "La Casa de las Dos Palmas",
  fuente: "netflix",
  periodo: "2026-02",
  neto: "900.00",
};

const filas = [anaCasa, anaSegundo, anaFebrero];

describe("filtrarIngresos", () => {
  it("sin filtro deja todas las filas del titular", () => {
    expect(
      filtrarIngresos(filas, { obra: "", fuente: "", periodo: "" }),
    ).toEqual(filas);
  });

  it("filtra por obra", () => {
    const got = filtrarIngresos(filas, {
      obra: "obra-ana-2",
      fuente: "",
      periodo: "",
    });
    expect(got).toEqual([anaSegundo]);
  });

  it("filtra por fuente", () => {
    const got = filtrarIngresos(filas, {
      obra: "",
      fuente: "netflix",
      periodo: "",
    });
    expect(got).toEqual([anaFebrero]);
  });

  it("filtra por periodo", () => {
    const got = filtrarIngresos(filas, {
      obra: "",
      fuente: "",
      periodo: "2026-01",
    });
    expect(got.map((f) => f.ref)).toEqual([anaCasa.ref, anaSegundo.ref]);
  });

  it("combina los tres filtros", () => {
    const got = filtrarIngresos(filas, {
      obra: "obra-completa",
      fuente: "caracol",
      periodo: "2026-01",
    });
    expect(got).toEqual([anaCasa]);
  });

  it("no cuela una fila de otro titular: el listado de entrada ya viene recortado", () => {
    const beto: Ingreso = {
      ...anaCasa,
      ref: "proc-2026-01:obra-completa:tit-beto",
      neto: "2400.00",
    };
    expect(
      filtrarIngresos([anaCasa], { obra: "", fuente: "", periodo: "" }),
    ).not.toContainEqual(beto);
  });
});

describe("opcionesFiltro", () => {
  it("deduplica obras, fuentes y periodos para los selects", () => {
    const o = opcionesFiltro(filas);
    expect(o.obras.map((x) => x.id).sort()).toEqual([
      "obra-ana-2",
      "obra-completa",
    ]);
    expect(o.fuentes).toEqual(["caracol", "netflix"]);
    expect(o.periodos).toEqual(["2026-01", "2026-02"]);
  });
});

describe("rutas", () => {
  it("no pone query si el filtro esta vacio", () => {
    expect(rutaMisIngresos({ obra: "", fuente: "", periodo: "" })).toBe(
      "/api/mis-ingresos",
    );
  });

  it("serializa los tres filtros", () => {
    expect(
      rutaMisIngresos({
        obra: "obra-completa",
        fuente: "caracol",
        periodo: "2026-01",
      }),
    ).toBe(
      "/api/mis-ingresos?obra=obra-completa&fuente=caracol&periodo=2026-01",
    );
  });

  it("no hay parametro titular_id: el alcance lo pone la sesion", () => {
    expect(
      rutaMisIngresos({ obra: "x", fuente: "", periodo: "" }),
    ).not.toContain("titular");
  });

  it("codifica la ref para la ruta de explicar", () => {
    expect(rutaExplicar("proc-1:obra:tit-ana")).toBe(
      "/api/explicar/proc-1%3Aobra%3Atit-ana",
    );
  });
});

describe("formatearNeto", () => {
  it("el headline es el neto, no el bruto", () => {
    expect(formatearNeto("3600.00")).toBe("$ 3600.00");
    expect(formatearNeto("3600.00")).not.toContain("4800");
  });
});
