import { cleanup, fireEvent, render, screen } from "@testing-library/react";
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

  it("un destino que es solo fragmento usa un ancla nativa", () => {
    montar(listo, "#ingresos");
    const enlace = screen.getByRole("link");
    expect(enlace.getAttribute("href")).toBe("#ingresos");
  });

  it("mientras carga pinta un esqueleto, no un texto suelto", () => {
    const { container } = montar({ tipo: "cargando" });
    expect(container.querySelector(".esqueleto")).toBeTruthy();
  });

  it("con ayuda ofrece un detalle que explica el indicador", () => {
    render(
      <MemoryRouter>
        <Tarjeta
          titulo="Obras en reserva"
          recurso={listo}
          ayuda="La declaración no suma 100%."
        >
          {(datos) => <p>{datos.total}</p>}
        </Tarjeta>
      </MemoryRouter>,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Qué significa Obras en reserva" }),
    );
    expect(screen.getByRole("dialog").textContent).toContain(
      "La declaración no suma 100%.",
    );
  });

  it("con icono lo pinta decorativo, fuera del nombre accesible", () => {
    const { container } = render(
      <MemoryRouter>
        <Tarjeta titulo="ONI" recurso={listo} icono={<svg data-icono />}>
          {(datos) => <p>{datos.total}</p>}
        </Tarjeta>
      </MemoryRouter>,
    );
    const icono = container.querySelector(".tarjeta-icono");
    expect(icono?.getAttribute("aria-hidden")).toBe("true");
    expect(screen.getByRole("heading", { name: "ONI" })).toBeTruthy();
  });
});
