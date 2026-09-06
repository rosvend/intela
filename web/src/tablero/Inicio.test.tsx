import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Inicio from "../Inicio";
import { setToken } from "../api";
import { ProveedorDeSesion, Rol } from "../sesion";

function json(cuerpo: unknown, status = 200) {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function usuario(rol: Rol, nombre = "Persona de Prueba") {
  return {
    id: "usr-1",
    email: "x@redes.co",
    nombre,
    rol,
    titular_id: rol === "titular" ? "tit-1" : "",
  };
}

function montar() {
  return render(
    <MemoryRouter>
      <ProveedorDeSesion>
        <Inicio />
      </ProveedorDeSesion>
    </MemoryRouter>,
  );
}

describe("Inicio — seleccion de tablero por rol", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    setToken("tok");
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("el administrador ve el panel de control y no la liquidacion del titular", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      if (String(input) === "/api/auth/session") {
        return json(usuario("administrador", "Admin Intela"));
      }
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await waitFor(() =>
      expect(
        screen.getByRole("heading", { name: "Panel de control" }),
      ).toBeTruthy(),
    );
    expect(screen.getByText("Cargas pendientes")).toBeTruthy();
    expect(screen.getByText("Obras en reserva")).toBeTruthy();
    expect(screen.getByText("ONI")).toBeTruthy();
    expect(screen.getByText("Última corrida")).toBeTruthy();
    expect(
      screen.queryByRole("heading", { name: "Mi liquidación" }),
    ).toBeNull();
  });

  it("el titular ve su liquidacion y no los KPIs de administrador", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      if (String(input) === "/api/auth/session") {
        return json(usuario("titular", "Ana Escritora"));
      }
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await waitFor(() =>
      expect(
        screen.getByRole("heading", { name: "Mi liquidación" }),
      ).toBeTruthy(),
    );
    expect(screen.getByText("Ana Escritora")).toBeTruthy();
    expect(screen.getByText("Mis obras")).toBeTruthy();
    expect(screen.getByText("Última liquidación")).toBeTruthy();
    expect(screen.getByText("Detalle por obra")).toBeTruthy();
    expect(screen.queryByText("Cargas pendientes")).toBeNull();
    expect(
      screen.queryByRole("heading", { name: "Panel de control" }),
    ).toBeNull();
  });

  it("cada widget muestra el vacio cuando su backend no existe, sin alerta", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      if (String(input) === "/api/auth/session") {
        return json(usuario("administrador"));
      }
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await waitFor(() =>
      expect(screen.getAllByText("Sin datos todavía").length).toBeGreaterThan(
        0,
      ),
    );
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("un widget con datos pinta el conteo y uno con 500 se queda en alerta sin tumbar el resto", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") {
        return json(usuario("administrador"));
      }
      if (path === "/api/tablero/oni") {
        return json({ total: 12 });
      }
      if (path === "/api/tablero/cargas-pendientes") {
        return json({ error: "la base esta caida" }, 500);
      }
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await waitFor(() => expect(screen.getByText("12")).toBeTruthy());
    expect(screen.getByRole("alert").textContent).toBe("la base esta caida");
    expect(screen.getAllByText("Sin datos todavía").length).toBeGreaterThan(0);
    expect(
      screen.getByRole("heading", { name: "Panel de control" }),
    ).toBeTruthy();
  });
});
