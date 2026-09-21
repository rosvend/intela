import { describe, expect, it } from "vitest";
import { CATEGORIAS, categoriaPorId } from "./categoriasDeBusqueda";

// Los parametros de consulta que declara `GET /obras` en api/openapi.yaml.
const PARAMETROS_DE_OBRAS = ["titulo", "genero", "anio", "ipi"];

describe("categorias de busqueda", () => {
  it("hay cuatro, en el orden del menu", () => {
    expect(CATEGORIAS.map((c) => c.id)).toEqual([
      "titulo",
      "genero",
      "ipi",
      "anio",
    ]);
    expect(CATEGORIAS.map((c) => c.etiqueta)).toEqual([
      "Título",
      "Género",
      "IPI de coautor",
      "Año",
    ]);
  });

  it("cada id es un parametro de GET /obras", () => {
    for (const categoria of CATEGORIAS) {
      expect(PARAMETROS_DE_OBRAS).toContain(categoria.id);
    }
  });

  it("solo Titulo filtra en vivo y solo Año es numerico", () => {
    expect(
      CATEGORIAS.filter((c) => c.modo === "vivo").map((c) => c.id),
    ).toEqual(["titulo"]);
    expect(
      CATEGORIAS.filter((c) => c.inputMode === "numeric").map((c) => c.id),
    ).toEqual(["anio"]);
  });

  it("solo Año valida", () => {
    expect(CATEGORIAS.filter((c) => c.validar).map((c) => c.id)).toEqual([
      "anio",
    ]);
  });

  describe("validacion del año", () => {
    const validar = categoriaPorId("anio").validar!;

    it("acepta un entero positivo", () => {
      expect(validar("1991")).toBeNull();
    });

    it.each(["0", "-1", "19a1", "1991.5"])("rechaza %j", (texto) => {
      expect(validar(texto)).toBe("Escribe un año entero positivo.");
    });
  });
});
