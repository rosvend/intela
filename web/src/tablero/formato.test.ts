import { describe, expect, it } from "vitest";
import {
  formatearEntero,
  formatearImporte,
  sumarImportes,
  tiempoRelativo,
} from "./formato";

describe("formatearEntero", () => {
  it("agrupa miles con locale es-CO", () => {
    expect(formatearEntero(1234)).toBe("1.234");
  });
});

describe("formatearImporte", () => {
  it("agrupa miles y antepone $ sin convertir el decimal a number", () => {
    expect(formatearImporte("1234567.89")).toBe("$ 1.234.567,89");
  });

  it("muestra centavos solo si no son cero", () => {
    expect(formatearImporte("1000")).toBe("$ 1.000");
    expect(formatearImporte("0.50")).toBe("$ 0,50");
    expect(formatearImporte("13746.00")).toBe("$ 13.746");
  });

  it("respeta el signo negativo", () => {
    expect(formatearImporte("-50.5")).toBe("-$ 50,50");
  });
});

describe("sumarImportes", () => {
  it("suma strings decimales sin perder centavos", () => {
    expect(sumarImportes(["0.10", "0.20"])).toBe("0.30");
    expect(sumarImportes(["600000000.00", "350000000", "40000000.5"])).toBe(
      "990000000.50",
    );
  });

  it("una lista vacia suma cero y un importe ilegible no cuenta", () => {
    expect(sumarImportes([])).toBe("0.00");
    expect(sumarImportes(["10", "abc"])).toBe("10.00");
  });

  it("respeta los negativos", () => {
    expect(sumarImportes(["-5.25", "1"])).toBe("-4.25");
  });
});

describe("tiempoRelativo", () => {
  const ahora = new Date("2026-10-05T12:00:00Z");

  it("dice hace un momento para menos de un minuto", () => {
    expect(tiempoRelativo("2026-10-05T11:59:40Z", ahora)).toBe(
      "hace un momento",
    );
  });

  it("usa minutos, horas y dias en lenguaje natural", () => {
    expect(tiempoRelativo("2026-10-05T11:55:00Z", ahora)).toBe(
      "hace 5 minutos",
    );
    expect(tiempoRelativo("2026-10-05T09:00:00Z", ahora)).toBe("hace 3 horas");
    expect(tiempoRelativo("2026-10-04T12:00:00Z", ahora)).toBe("ayer");
    expect(tiempoRelativo("2026-10-01T12:00:00Z", ahora)).toBe("hace 4 días");
  });

  it("una fecha ilegible se devuelve tal cual", () => {
    expect(tiempoRelativo("no-es-fecha", ahora)).toBe("no-es-fecha");
  });
});
