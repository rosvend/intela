import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Layout from "./Layout";
import { setToken, token } from "./api";
import { ProveedorDeSesion } from "./sesion";

function montar() {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <ProveedorDeSesion>
        <Routes>
          <Route element={<Layout />}>
            <Route index element={<p>contenido de inicio</p>} />
          </Route>
        </Routes>
      </ProveedorDeSesion>
    </MemoryRouter>,
  );
}

function respuestaUsuario(rol: string, nombre = "Persona de Prueba") {
  return new Response(
    JSON.stringify({
      id: "usr-1",
      email: "x@redes.co",
      nombre,
      rol,
      titular_id: rol === "titular" ? "tit-1" : "",
    }),
    { headers: { "content-type": "application/json" } },
  );
}

describe("Layout", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("con rol titular el sidebar tiene un solo enlace (Inicio)", async () => {
    setToken("tok");
    vi.mocked(fetch).mockResolvedValue(respuestaUsuario("titular"));

    montar();

    await waitFor(() => expect(screen.getAllByRole("link").length).toBe(1));
    expect(screen.getByRole("link", { name: "Inicio" })).toBeTruthy();
    expect(screen.queryByText("Configuración")).toBeNull();
  });

  it("con rol administrador el sidebar tiene nueve enlaces en sus dos secciones", async () => {
    setToken("tok");
    vi.mocked(fetch).mockResolvedValue(respuestaUsuario("administrador"));

    montar();

    await waitFor(() => expect(screen.getAllByRole("link").length).toBe(9));
    expect(screen.getByText("Principal")).toBeTruthy();
    expect(screen.getByText("Configuración")).toBeTruthy();
  });

  it("el sidebar abre con el logo de la marca, no con texto plano", async () => {
    setToken("tok");
    vi.mocked(fetch).mockResolvedValue(respuestaUsuario("administrador"));

    montar();

    await waitFor(() =>
      expect(screen.getByRole("img", { name: "Intela" })).toBeTruthy(),
    );
  });

  it("pinta el nombre y el rol legible en el pie del sidebar", async () => {
    // Rol "auditor" a proposito: "distribucion" choca de nombre con el
    // modulo de nav "Distribución" (M-3) y volveria ambiguo el query.
    setToken("tok");
    vi.mocked(fetch).mockResolvedValue(
      respuestaUsuario("auditor", "Beto Revisor"),
    );

    montar();

    await waitFor(() => expect(screen.getByText("Beto Revisor")).toBeTruthy());
    expect(screen.getByText("Auditor")).toBeTruthy();
  });

  it("logout llama a DELETE /auth/session y limpia el token", async () => {
    setToken("tok");
    vi.mocked(fetch).mockResolvedValueOnce(respuestaUsuario("administrador"));
    vi.mocked(fetch).mockResolvedValueOnce(new Response(null, { status: 204 }));

    montar();
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Cerrar sesión" }),
      ).toBeTruthy(),
    );

    fireEvent.click(screen.getByRole("button", { name: "Cerrar sesión" }));

    await waitFor(() => expect(token()).toBe(""));
    const llamadaDelete = vi.mocked(fetch).mock.calls[1];
    expect(llamadaDelete[1]?.method).toBe("DELETE");
  });

  it("una ruta fuera de la nav del rol actual muestra 'No autorizado' (guard cosmetico)", async () => {
    setToken("tok");
    vi.mocked(fetch).mockResolvedValue(respuestaUsuario("titular"));

    render(
      <MemoryRouter initialEntries={["/auditoria"]}>
        <ProveedorDeSesion>
          <Routes>
            <Route element={<Layout />}>
              <Route path="/auditoria" element={<p>pantalla de auditoría</p>} />
            </Route>
          </Routes>
        </ProveedorDeSesion>
      </MemoryRouter>,
    );

    await waitFor(() => expect(screen.getByText("No autorizado")).toBeTruthy());
    expect(screen.queryByText("pantalla de auditoría")).toBeNull();
  });

  it("una ruta que no es un modulo del mockup (/estado) no la bloquea el guard", async () => {
    setToken("tok");
    vi.mocked(fetch).mockResolvedValue(respuestaUsuario("titular"));

    render(
      <MemoryRouter initialEntries={["/estado"]}>
        <ProveedorDeSesion>
          <Routes>
            <Route element={<Layout />}>
              <Route path="/estado" element={<p>pantalla de estado</p>} />
            </Route>
          </Routes>
        </ProveedorDeSesion>
      </MemoryRouter>,
    );

    await waitFor(() =>
      expect(screen.getByText("pantalla de estado")).toBeTruthy(),
    );
    expect(screen.queryByText("No autorizado")).toBeNull();
  });
});
