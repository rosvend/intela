import { describe, expect, it } from "vitest";
import { iniciales } from "./iniciales";

describe("iniciales", () => {
  it("toma la primera letra de las dos primeras palabras, en mayuscula", () => {
    expect(iniciales("Beto Revisor")).toBe("BR");
  });

  it("un nombre de una sola palabra deja una sola inicial", () => {
    expect(iniciales("Cher")).toBe("C");
  });

  it("ignora espacios de sobra al principio, en medio y al final", () => {
    expect(iniciales("  Ana   Maria  ")).toBe("AM");
  });

  it("un nombre de tres o mas palabras solo usa las dos primeras", () => {
    expect(iniciales("Maria Jose Perez Gomez")).toBe("MJ");
  });

  it("un texto vacio no revienta: da iniciales vacias", () => {
    expect(iniciales("")).toBe("");
  });

  it("respeta acentos y ñ tal cual, sin transliterar", () => {
    expect(iniciales("Íñigo Núñez")).toBe("ÍN");
  });
});
