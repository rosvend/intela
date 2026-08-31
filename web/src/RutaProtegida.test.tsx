import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import RutaProtegida from "./RutaProtegida";
import { setToken } from "./api";
import { ProveedorDeSesion } from "./sesion";

function ArbolDeRutas() {
  return (
    <ProveedorDeSesion>
      <Routes>
        <Route path="/login" element={<p>pantalla de login</p>} />
        <Route element={<RutaProtegida />}>
          <Route path="/" element={<p>panel de control</p>} />
        </Route>
      </Routes>
    </ProveedorDeSesion>
  );
}

function montar() {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <ArbolDeRutas />
    </MemoryRouter>,
  );
}

describe("RutaProtegida", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("sin token, redirige a /login de inmediato", () => {
    montar();

    expect(screen.getByText("pantalla de login")).toBeTruthy();
    expect(screen.queryByText("panel de control")).toBeNull();
  });

  it("mientras resuelve la sesion, muestra un estado de carga y no una pantalla en blanco", () => {
    setToken("token-valido");
    // fetch nunca resuelve en este test: se queda en el estado "cargando".
    vi.mocked(fetch).mockReturnValue(new Promise(() => {}));

    montar();

    expect(screen.getByText("Cargando…")).toBeTruthy();
    expect(screen.queryByText("pantalla de login")).toBeNull();
  });

  it("el estado de carga se centra en toda la pantalla: aqui no existe Layout todavia", () => {
    setToken("token-valido");
    vi.mocked(fetch).mockReturnValue(new Promise(() => {}));

    const { container } = montar();

    expect(container.querySelector(".pantalla-carga")).toBeTruthy();
  });

  it("con token y sesion valida, renderiza la ruta protegida", async () => {
    setToken("token-valido");
    vi.mocked(fetch).mockResolvedValue(
      new Response(
        JSON.stringify({
          id: "usr-1",
          email: "admin@redes.co",
          nombre: "Admin",
          rol: "administrador",
          titular_id: "",
        }),
        { headers: { "content-type": "application/json" } },
      ),
    );

    montar();

    await waitFor(() =>
      expect(screen.getByText("panel de control")).toBeTruthy(),
    );
  });

  it("con token pero sesion invalida (401), termina redirigiendo a /login", async () => {
    setToken("token-vencido");
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ error: "sesion invalida o expirada" }), {
        status: 401,
      }),
    );

    montar();

    await waitFor(() =>
      expect(screen.getByText("pantalla de login")).toBeTruthy(),
    );
  });
});
