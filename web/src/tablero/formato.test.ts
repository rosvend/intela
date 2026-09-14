import { describe, expect, it } from "vitest";
import { formatearEntero, formatearImporte } from "./formato";

describe("formatearEntero", () => {
  it("agrupa miles con locale es-CO", () => {
    expect(formatearEntero(1234)).toBe("1.234");
  });
});

describe("formatearImporte", () => {
  it("agrupa miles y antepone $ sin convertir el decimal a number", () => {
    expect(formatearImporte("1234567.89")).toBe("$ 1.234.567,89");
  });

  it("conserva la parte decimal tal cual llega", () => {
    expect(formatearImporte("1000")).toBe("$ 1.000");
    expect(formatearImporte("0.50")).toBe("$ 0,50");
  });

  it("respeta el signo negativo", () => {
    expect(formatearImporte("-50.5")).toBe("-$ 50,5");
  });
});
