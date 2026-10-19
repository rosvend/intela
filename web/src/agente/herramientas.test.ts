import { describe, expect, it } from "vitest";
import { etiquetaHerramienta } from "./herramientas";

describe("etiquetaHerramienta", () => {
  it.each([
    ["buscar_reglamento", "Buscando en el Reglamento…"],
    ["explicar_cifra", "Rastreando el origen de la cifra…"],
    ["buscar_obra", "Buscando la obra en el catálogo…"],
    ["estado_declaracion", "Revisando la declaración de obra…"],
    ["listar_oni", "Revisando las obras no identificadas…"],
    ["estado_corrida", "Consultando el estado de la corrida…"],
  ])("traduce %s a lo que el staff entiende", (nombre, etiqueta) => {
    expect(etiquetaHerramienta(nombre)).toBe(etiqueta);
  });

  it("un nombre heredado de Object no se toma por una etiqueta", () => {
    expect(etiquetaHerramienta("toString")).toBe("Consultando: toString…");
    expect(etiquetaHerramienta("constructor")).toBe(
      "Consultando: constructor…",
    );
  });

  it("una herramienta nueva sin etiqueta sigue viendose, con su nombre legible", () => {
    expect(etiquetaHerramienta("contar_titulares")).toBe(
      "Consultando: contar titulares…",
    );
  });

  it("un nombre vacio no deja un chip en blanco", () => {
    expect(etiquetaHerramienta("")).toBe("Consultando una fuente…");
  });
});
