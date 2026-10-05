import { describe, expect, it } from "vitest";
import { partirCitas } from "./citas";

describe("partirCitas", () => {
  it("un texto sin citas queda en un solo fragmento", () => {
    expect(partirCitas("Hola, en que te ayudo?")).toEqual([
      { tipo: "texto", texto: "Hola, en que te ayudo?" },
    ]);
  });

  it("separa numerales de los cuatro reglamentos sin comerse la puntuacion final", () => {
    expect(
      partirCitas("Se retiene (RD 13.1.3). Ver RT 5, RS 12.2 y RA 3."),
    ).toEqual([
      { tipo: "texto", texto: "Se retiene (" },
      { tipo: "cita", clase: "reglamento", valor: "RD 13.1.3" },
      { tipo: "texto", texto: "). Ver " },
      { tipo: "cita", clase: "reglamento", valor: "RT 5" },
      { tipo: "texto", texto: ", " },
      { tipo: "cita", clase: "reglamento", valor: "RS 12.2" },
      { tipo: "texto", texto: " y " },
      { tipo: "cita", clase: "reglamento", valor: "RA 3" },
      { tipo: "texto", texto: "." },
    ]);
  });

  it("una cita de asiento guarda solo la ref, sin la palabra ni el punto final", () => {
    expect(
      partirCitas("Sale del asiento p-2026.01:obra-7:tit-3. Y RD 9.1.1"),
    ).toEqual([
      { tipo: "texto", texto: "Sale del " },
      { tipo: "cita", clase: "asiento", valor: "p-2026.01:obra-7:tit-3" },
      { tipo: "texto", texto: ". Y " },
      { tipo: "cita", clase: "reglamento", valor: "RD 9.1.1" },
    ]);
  });

  it("reconoce Asiento con mayuscula y una ref sin punto final", () => {
    expect(partirCitas("Asiento p1:obra-7")).toEqual([
      { tipo: "cita", clase: "asiento", valor: "p1:obra-7" },
    ]);
    expect(partirCitas("asiento p1:obra-7.")).toEqual([
      { tipo: "cita", clase: "asiento", valor: "p1:obra-7" },
      { tipo: "texto", texto: "." },
    ]);
  });

  it.each([
    "Toda cifra se explica hasta su asiento de auditoria.",
    "Revisa el asiento que firmo Contabilidad.",
    "Asiento contable del proceso p1.",
    "Asiento 42",
    "asiento a:b:c:d",
  ])("no hace pildora de una palabra suelta tras asiento: %s", (texto) => {
    expect(partirCitas(texto)).toEqual([{ tipo: "texto", texto }]);
  });

  it("no confunde siglas dentro de otras palabras ni un RD sin numeral", () => {
    expect(partirCitas("HARD 9 y el RD vigente")).toEqual([
      { tipo: "texto", texto: "HARD 9 y el RD vigente" },
    ]);
  });
});
