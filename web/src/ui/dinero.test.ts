import { describe, expect, it } from "vitest";
import {
  aNumero,
  formatearCOP,
  formatearCOPCompacto,
  proporciones,
  sumarImportes,
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

  it("quita los ceros a la izquierda", () => {
    expect(formatearCOP("0001234.50")).toBe("$ 1.234,50");
    expect(formatearCOP("000")).toBe("$ 0");
  });

  it("redondea a centavos lejos de cero, igual que sumarImportes", () => {
    expect(formatearCOP("1.999")).toBe("$ 2");
    expect(formatearCOP("1.994")).toBe("$ 1,99");
    expect(formatearCOP("1.995")).toBe("$ 2");
    expect(formatearCOP("-1.995")).toBe("-$ 2");
    expect(formatearCOP("999.999")).toBe("$ 1.000");
    expect(sumarImportes(["1.999"])).toBe("2.00");
  });

  it("el cero nunca lleva signo", () => {
    expect(formatearCOP("-0.001")).toBe("$ 0");
    expect(formatearCOP("-0")).toBe("$ 0");
    expect(formatearCOP("-0.00")).toBe("$ 0");
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

  it("conserva el signo negativo", () => {
    expect(formatearCOPCompacto("-2450000")).toBe("-$ 2,5 M");
    expect(formatearCOPCompacto("-13746")).toBe("-$ 13,7 mil");
    expect(formatearCOPCompacto("-850")).toBe("-$ 850");
  });

  it("el cero redondeado no lleva signo", () => {
    expect(formatearCOPCompacto("-0.4")).toBe("$ 0");
  });

  it("sube de unidad cuando el redondeo alcanza el umbral", () => {
    expect(formatearCOPCompacto("999999.99")).toBe("$ 1 M");
    expect(formatearCOPCompacto("999950")).toBe("$ 1 M");
    expect(formatearCOPCompacto("999949")).toBe("$ 999,9 mil");
    expect(formatearCOPCompacto("999.5")).toBe("$ 1 mil");
    expect(formatearCOPCompacto("999.4")).toBe("$ 999");
    expect(formatearCOPCompacto("-999999.99")).toBe("-$ 1 M");
    expect(formatearCOPCompacto("1000000")).toBe("$ 1 M");
    expect(formatearCOPCompacto("1000")).toBe("$ 1 mil");
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

describe("sumarImportes", () => {
  it("suma strings decimales en centavos exactos, sin pasar por float", () => {
    expect(sumarImportes(["0.10", "0.20"])).toBe("0.30");
    expect(sumarImportes(["600000000.00", "350000000", "40000000.5"])).toBe(
      "990000000.50",
    );
    expect(sumarImportes(["123456789012345678.99", "0.01"])).toBe(
      "123456789012345679.00",
    );
  });

  it("una lista vacia suma cero", () => {
    expect(sumarImportes([])).toBe("0.00");
  });

  it("respeta los negativos", () => {
    expect(sumarImportes(["4800.00", "-1200.00"])).toBe("3600.00");
    expect(sumarImportes(["-5.25", "1"])).toBe("-4.25");
  });

  it("suma con toda la precision y redondea una vez, lejos de cero, como StringFixed(2)", () => {
    expect(sumarImportes(["1.239"])).toBe("1.24");
    expect(sumarImportes(["0.004", "0.004"])).toBe("0.01");
    expect(sumarImportes(["0.005"])).toBe("0.01");
    expect(sumarImportes(["-0.005"])).toBe("-0.01");
    expect(sumarImportes(["0.0049"])).toBe("0.00");
    expect(sumarImportes(["60.0000"])).toBe("60.00");
  });

  it("un total que redondea a cero no lleva signo", () => {
    expect(sumarImportes(["-0.004"])).toBe("0.00");
    expect(sumarImportes(["-0.001", "-0.003"])).toBe("0.00");
  });

  it("los ceros a la izquierda no cambian la suma", () => {
    expect(sumarImportes(["007.50", "0003"])).toBe("10.50");
  });

  it("un importe ilegible lanza: el dinero no se descarta en silencio", () => {
    for (const ilegible of ["abc", "", "1e5", "1,5", "NaN", "1."]) {
      expect(() => sumarImportes(["10", ilegible])).toThrow(RangeError);
    }
  });
});
