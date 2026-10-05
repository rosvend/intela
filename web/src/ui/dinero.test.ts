import { describe, expect, it } from "vitest";
import {
  aNumero,
  formatearCOP,
  formatearCOPCompacto,
  proporciones,
} from "./dinero";

describe("formatearCOP", () => {
  it("agrupa miles con punto y omite centavos en cero", () => {
    expect(formatearCOP("13746.00")).toBe("$ 13.746");
    expect(formatearCOP("1500000000")).toBe("$ 1.500.000.000");
  });

  it("muestra centavos con coma cuando no son cero", () => {
    expect(formatearCOP("1234567.89")).toBe("$ 1.234.567,89");
    expect(formatearCOP("0.5")).toBe("$ 0,50");
  });

  it("respeta el signo sin aritmetica de punto flotante", () => {
    expect(formatearCOP("-50.10")).toBe("-$ 50,10");
    expect(formatearCOP("123456789012345678.99")).toBe(
      "$ 123.456.789.012.345.678,99",
    );
  });

  it("devuelve el texto tal cual si no es un decimal", () => {
    expect(formatearCOP("abc")).toBe("abc");
  });
});

describe("formatearCOPCompacto", () => {
  it("abrevia millones y miles para etiquetas de grafica", () => {
    expect(formatearCOPCompacto("1500000000.00")).toBe("$ 1.500 M");
    expect(formatearCOPCompacto("2450000")).toBe("$ 2,5 M");
    expect(formatearCOPCompacto("13746")).toBe("$ 13,7 mil");
    expect(formatearCOPCompacto("850")).toBe("$ 850");
  });
});

describe("proporciones", () => {
  it("convierte importes en porcentajes que suman 100", () => {
    const p = proporciones(["300.00", "100.00"]);
    expect(p).toEqual([75, 25]);
  });

  it("todo cero da ceros, no NaN", () => {
    expect(proporciones(["0", "0.00"])).toEqual([0, 0]);
  });

  it("ignora negativos", () => {
    expect(proporciones(["-5", "5"])).toEqual([0, 100]);
  });
});

describe("aNumero", () => {
  it("lee el decimal o devuelve 0", () => {
    expect(aNumero("12.5")).toBe(12.5);
    expect(aNumero("x")).toBe(0);
  });
});
