import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import Paginador from "./Paginador";

afterEach(cleanup);

function montar(desplazamiento: number, cuantas: number, limite = 20) {
  const onIrA = vi.fn();
  render(
    <Paginador
      etiqueta="Obras"
      desplazamiento={desplazamiento}
      cuantas={cuantas}
      limite={limite}
      onIrA={onIrA}
    />,
  );
  return onIrA;
}

describe("Paginador", () => {
  it("los botones son redondos, con icono decorativo y nombre accesible", () => {
    montar(20, 20);

    for (const nombre of ["Página anterior", "Página siguiente"]) {
      const boton = screen.getByRole("button", { name: nombre });
      expect(boton.getAttribute("title")).toBe(nombre);
      expect(boton.textContent).toBe("");
      const svg = boton.querySelector("svg");
      expect(svg?.getAttribute("aria-hidden")).toBe("true");
      expect(svg?.getAttribute("focusable")).toBe("false");
    }
    // El texto del rango se queda.
    expect(screen.getByText("Obras 21 a 40")).toBeTruthy();
  });

  it("en la primera pagina no hay anterior, y una pagina incompleta no ofrece siguiente", () => {
    montar(0, 5);

    expect(
      screen.getByRole("button", { name: "Página anterior" }),
    ).toHaveProperty("disabled", true);
    expect(
      screen.getByRole("button", { name: "Página siguiente" }),
    ).toHaveProperty("disabled", true);
  });

  it("anterior y siguiente mueven el desplazamiento de a un limite", () => {
    const onIrA = montar(20, 20);

    fireEvent.click(screen.getByRole("button", { name: "Página anterior" }));
    fireEvent.click(screen.getByRole("button", { name: "Página siguiente" }));

    expect(onIrA).toHaveBeenNthCalledWith(1, 0);
    expect(onIrA).toHaveBeenNthCalledWith(2, 40);
  });
});
