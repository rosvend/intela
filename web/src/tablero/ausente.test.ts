import { describe, expect, it } from "vitest";
import { ApiError, ErrorDeRed } from "../api";
import { esAusente } from "./ausente";

describe("esAusente", () => {
  it("un 404 o 501 significa que el backend del widget todavia no existe", () => {
    expect(esAusente(new ApiError(404, "ruta no encontrada"))).toBe(true);
    expect(esAusente(new ApiError(501, "no implementado"))).toBe(true);
  });

  it("un fallo de red no tumba la tarjeta: el backend esta ausente", () => {
    expect(esAusente(new ErrorDeRed(new TypeError("Failed to fetch")))).toBe(
      true,
    );
  });

  it("un 500 es un error de verdad, no un vacio", () => {
    expect(esAusente(new ApiError(500, "la base esta caida"))).toBe(false);
  });

  it("un 401 no se clasifica aqui: api() ya redirige al login", () => {
    expect(esAusente(new ApiError(401, "sesion invalida o expirada"))).toBe(
      false,
    );
  });
});
