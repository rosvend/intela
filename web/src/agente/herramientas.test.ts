import { describe, expect, it } from "vitest";
import { etiquetaHerramienta } from "./herramientas";

describe("etiquetaHerramienta", () => {
  it("traduce las herramientas del catalogo a lo que el staff entiende", () => {
    expect(etiquetaHerramienta("buscar_reglamento")).toBe(
      "Buscando en el Reglamento…",
    );
    expect(etiquetaHerramienta("estado_corrida")).toBe(
      "Consultando el estado de la corrida…",
    );
    expect(etiquetaHerramienta("listar_oni")).toBe(
      "Revisando las obras no identificadas…",
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
