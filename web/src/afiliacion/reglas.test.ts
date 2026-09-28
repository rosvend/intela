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

  it("exige una clave de al menos 8 caracteres", () => {
    const d = {
      ...datosVacios,
      nombre: "Ana",
      email: "ana@redes.co",
      documentoIdentidad: "123",
      clave: "corta",
      claveConfirmacion: "corta",
    };
    expect(errorDelPaso(0, d)).toMatch(/8 y 72/i);
  });

  it("exige que las dos claves coincidan", () => {
    const d = {
      ...datosVacios,
      nombre: "Ana",
      email: "ana@redes.co",
      documentoIdentidad: "123",
      clave: "secret12",
      claveConfirmacion: "secret99",
    };
    expect(errorDelPaso(0, d)).toMatch(/no coinciden/i);
  });
});
