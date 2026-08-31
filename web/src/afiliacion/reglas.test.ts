import { describe, expect, it } from "vitest";
import {
  conflictoExclusividad,
  datosVacios,
  errorDelPaso,
  MENSAJE_DOCUMENTOS,
  MENSAJE_EXCLUSIVIDAD,
} from "./reglas";

describe("conflictoExclusividad", () => {
  it("bloquea pertenecer a otra SGC sin renuncia", () => {
    expect(conflictoExclusividad(true, false)).toBe(MENSAJE_EXCLUSIVIDAD);
  });

  it("permite pertenecer a otra SGC con la renuncia adjunta", () => {
    expect(conflictoExclusividad(true, true)).toBeNull();
  });

  it("no bloquea a quien declara no pertenecer", () => {
    expect(conflictoExclusividad(false, false)).toBeNull();
  });
});

describe("errorDelPaso", () => {
  it("exige RUT y certificacion bancaria en documentos", () => {
    const d = { ...datosVacios, nombre: "Ana" };
    expect(errorDelPaso(2, d)).toBe(MENSAJE_DOCUMENTOS);
  });

  it("exige el subtipo", () => {
    expect(errorDelPaso(1, datosVacios)).toMatch(/vínculo/i);
  });
});
