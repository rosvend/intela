import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";
import { Tarjeta } from "./Tarjeta";
import { Recurso } from "./tipos";

const listo: Recurso<{ total: number }> = {
  tipo: "listo",
  datos: { total: 7 },
};

function montar(recurso: Recurso<{ total: number }>, to?: string) {
  return render(
    <MemoryRouter>
      <Tarjeta
        titulo="ONI"
        recurso={recurso}
        to={to}
        mensajeAusente="Sin datos todavía"
      >
        {(datos) => <p className="tarjeta-valor">{datos.total}</p>}
      </Tarjeta>
    </MemoryRouter>,
  );
}

describe("Tarjeta", () => {
  afterEach(() => {
    cleanup();
  });

  it("con datos pinta el valor que le pasa el hijo", () => {
    montar(listo);
    expect(screen.getByRole("heading", { name: "ONI" })).toBeTruthy();
    expect(screen.getByText("7")).toBeTruthy();
  });

  it("sin backend muestra el estado vacio y no un error", () => {
    montar({ tipo: "ausente" });
    expect(screen.getByText("Sin datos todavía")).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.queryByText("7")).toBeNull();
  });

  it("con error muestra una alerta y no se rompe", () => {
    montar({ tipo: "error", mensaje: "la base esta caida" });
    expect(screen.getByRole("alert").textContent).toBe("la base esta caida");
  });

  it("mientras carga anuncia el estado sin dejar el cuerpo vacio", () => {
    montar({ tipo: "cargando" });
    expect(screen.getByRole("status").textContent).toBe("Cargando…");
  });

  it("si hay destino, expone un enlace para el panel que la rellena", () => {
    montar(listo, "/anomalias");
    const enlace = screen.getByRole("link");
    expect(enlace.getAttribute("href")).toBe("/anomalias");
  });
});
