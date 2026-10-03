import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import ErrorBoundary from "./ErrorBoundary";

// Lo apaga el test justo antes de reintentar. Vive fuera del render: un
// contador dentro de la funcion se incrementa tambien en el reintento con el
// que React reconstruye la pila del error, y la pantalla "se recupera" sola.
let romper = true;

function Pantalla() {
  const { pathname } = useLocation();
  if (pathname === "/") return <p>inicio sano</p>;
  if (romper) {
    throw new Error("Cannot read properties of undefined (reading 'slice')");
  }
  return <p>pantalla recuperada</p>;
}

function montar() {
  return render(
    <MemoryRouter initialEntries={["/ingesta"]}>
      <Routes>
        <Route
          path="*"
          element={
            <ErrorBoundary>
              <Pantalla />
            </ErrorBoundary>
          }
        />
      </Routes>
    </MemoryRouter>,
  );
}

describe("ErrorBoundary", () => {
  beforeEach(() => {
    romper = true;
    vi.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("un fallo de render se ve, y reintentar remonta la pantalla", () => {
    montar();

    expect(
      screen.getByRole("heading", { name: "No se pudo mostrar esta pantalla" }),
    ).toBeTruthy();
    expect(
      screen.getByText("Cannot read properties of undefined (reading 'slice')"),
    ).toBeTruthy();
    expect(screen.queryByText("pantalla recuperada")).toBeNull();

    romper = false;
    fireEvent.click(screen.getByRole("button", { name: "Reintentar" }));

    expect(screen.getByText("pantalla recuperada")).toBeTruthy();
    expect(
      screen.queryByRole("heading", {
        name: "No se pudo mostrar esta pantalla",
      }),
    ).toBeNull();
  });

  it("volver al inicio sale del fallo y muestra el inicio", () => {
    montar();

    expect(screen.getByRole("alert")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Volver al inicio" }));

    expect(screen.getByText("inicio sano")).toBeTruthy();
    expect(
      screen.queryByRole("heading", {
        name: "No se pudo mostrar esta pantalla",
      }),
    ).toBeNull();
  });
});
