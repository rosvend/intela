import { describe, expect, it } from "vitest";
import {
  citarReglamento,
  citaDeConcepto,
  citaDeRetencion,
  nombreConcepto,
} from "./reglamento";

describe("citarReglamento", () => {
  it("separa un reglamento compuesto y resuelve cada token por su cuenta", () => {
    const citas = citarReglamento("RD 9.1.1+RD-IX-seed-sintetico");
    expect(citas).toHaveLength(2);
    expect(citas[0].token).toBe("RD 9.1.1");
    expect(citas[0].titulo).toContain("Television abierta");
    expect(citas[0].texto).toContain("Total puntos por obra");
    expect(citas[1].token).toBe("RD-IX-seed-sintetico");
    expect(citas[1].titulo).toContain("provisional de siembra");
  });

  it("no inventa texto para una cita que no esta en la tabla curada", () => {
    const [cita] = citarReglamento("RD-NUNCA-VISTA");
    expect(cita.token).toBe("RD-NUNCA-VISTA");
    expect(cita.titulo).toBe("RD-NUNCA-VISTA");
    expect(cita.texto).toBe("");
  });

  it("un reglamento vacio no produce citas", () => {
    expect(citarReglamento("")).toHaveLength(0);
  });

  it.each([
    ["RD 9.2", "taquilla"],
    ["RD 9.3", "taquilla"],
    ["RD 9.4", "exhibiciones"],
    ["RD 9.5", "9.1.1"],
    ["RD 9.6", "numeral 9.5"],
    ["RD 9.7", "Puntaje base"],
    ["RD 13.1.3", "declaracion discriminada del 100%"],
  ])("cubre la modalidad %s con su extracto verbatim", (token, fragmento) => {
    const [cita] = citarReglamento(token);
    expect(cita.texto).toContain(fragmento);
  });
});

describe("citaDeConcepto", () => {
  it("mapea los tres conceptos de deduccion a R-06/R-07", () => {
    expect(citaDeConcepto("gastos_administrativos")?.titulo).toContain("R-06");
    expect(citaDeConcepto("bienestar_social")?.titulo).toContain("R-06");
    expect(citaDeConcepto("reserva_errores_tecnicos")?.titulo).toContain(
      "R-07",
    );
  });

  it("un concepto desconocido no tiene cita", () => {
    expect(citaDeConcepto("concepto-inventado")).toBeUndefined();
  });
});

describe("nombreConcepto", () => {
  it("traduce los tres codigos de concepto a lenguaje llano", () => {
    expect(nombreConcepto("gastos_administrativos")).toBe(
      "Gastos administrativos",
    );
    expect(nombreConcepto("bienestar_social")).toBe("Bienestar social");
    expect(nombreConcepto("reserva_errores_tecnicos")).toBe(
      "Reserva para errores tecnicos",
    );
  });

  it("un concepto sin traduccion se devuelve tal cual", () => {
    expect(nombreConcepto("otro-concepto")).toBe("otro-concepto");
  });
});

describe("citaDeRetencion", () => {
  it("es siempre R-04 / RD 13.1.3", () => {
    const cita = citaDeRetencion();
    expect(cita.titulo).toContain("R-04");
    expect(cita.texto).toContain("reserva el total del importe");
  });
});
