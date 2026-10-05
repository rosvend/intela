import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import Detalle from "./Detalle";

describe("Detalle", () => {
  afterEach(cleanup);

  it("el disparador anuncia si esta abierto y el panel se cierra con Escape", () => {
    render(
      <Detalle disparador="¿Por qué?" titulo="Gastos">
        <p>Explicación</p>
      </Detalle>,
    );
    const boton = screen.getByRole("button", { name: "¿Por qué?" });
    expect(boton.getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(boton);
    expect(boton.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByRole("dialog", { name: "Gastos" })).toBeTruthy();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("un clic fuera lo cierra", () => {
    render(
      <div>
        <span>fuera</span>
        <Detalle disparador="Abrir" titulo="T">
          <p>x</p>
        </Detalle>
      </div>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Abrir" }));
    fireEvent.mouseDown(screen.getByText("fuera"));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
