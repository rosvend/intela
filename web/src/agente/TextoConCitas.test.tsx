import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import TextoConCitas from "./TextoConCitas";

const pintar = (texto: string) =>
  render(<TextoConCitas texto={texto} />).container;

describe("TextoConCitas con markdown", () => {
  afterEach(cleanup);

  it("negrita, cursiva y codigo en linea", () => {
    const c = pintar("Es **clave**, *util* y `x_1`.");
    expect(c.querySelector("strong")?.textContent).toBe("clave");
    expect(c.querySelector("em")?.textContent).toBe("util");
    expect(c.querySelector("code")?.textContent).toBe("x_1");
    expect(c.textContent).toBe("Es clave, util y x_1.");
  });

  it("parrafos separados por linea en blanco", () => {
    const c = pintar("Uno\n\nDos");
    expect([...c.querySelectorAll("p")].map((p) => p.textContent)).toEqual([
      "Uno",
      "Dos",
    ]);
  });

  it("listas con vinetas y numeradas", () => {
    const c = pintar("Pasos:\n- a\n- **b**\n\n1. uno\n2. dos");
    expect(c.querySelectorAll("ul > li")).toHaveLength(2);
    expect(c.querySelector("ul strong")?.textContent).toBe("b");
    expect(
      [...c.querySelectorAll("ol > li")].map((l) => l.textContent),
    ).toEqual(["uno", "dos"]);
  });

  it("las citas siguen siendo pildoras dentro de negrita y de listas", () => {
    const c = pintar("**Ver RD 13.1.3** y\n- asiento p1:obra-7");
    expect(c.querySelectorAll(".cita")).toHaveLength(2);
    expect(c.querySelector("strong .cita")).toBeTruthy();
    expect(c.querySelector("li .cita")).toBeTruthy();
  });

  it("nunca inyecta HTML crudo ni enlaces", () => {
    const c = pintar(
      "<img src=x onerror=alert(1)> [clic](javascript:alert(1))",
    );
    expect(c.querySelector("img")).toBeNull();
    expect(c.querySelector("a")).toBeNull();
    expect(c.textContent).toContain("<img");
  });

  it("asteriscos sueltos quedan como texto", () => {
    expect(pintar("2 * 3 * 4").textContent).toBe("2 * 3 * 4");
  });
});
