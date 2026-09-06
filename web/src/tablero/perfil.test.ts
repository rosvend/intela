import { describe, expect, it } from "vitest";
import { perfilDeTablero } from "./perfil";

describe("perfilDeTablero", () => {
  it("el titular ve su propio tablero", () => {
    expect(perfilDeTablero("titular")).toBe("titular");
  });

  it.each([
    "administrador",
    "distribucion",
    "contabilidad",
    "auditor",
  ] as const)("el rol %s aterriza en el tablero de administrador", (rol) => {
    expect(perfilDeTablero(rol)).toBe("administrador");
  });
});
