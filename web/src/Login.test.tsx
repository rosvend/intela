import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Login from "./Login";
import { token } from "./api";
import { ProveedorDeSesion } from "./sesion";

function montar(
  initialEntries: (string | { pathname: string; state?: unknown })[] = [
    "/login",
  ],
) {
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <ProveedorDeSesion>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/" element={<p>panel de control</p>} />
          <Route path="/catalogo" element={<p>pantalla de catálogo</p>} />
        </Routes>
      </ProveedorDeSesion>
    </MemoryRouter>,
  );
}

function respuestaLogin() {
  return new Response(
    JSON.stringify({
      token: "tok-123",
      expira: "2026-01-01T00:00:00Z",
      usuario: {
        id: "usr-1",
        email: "admin@redes.co",
        nombre: "Admin",
        rol: "administrador",
        titular_id: "",
      },
    }),
    { headers: { "content-type": "application/json" } },
  );
}

async function completarYEnviar(email: string, clave: string) {
  fireEvent.change(screen.getByLabelText("Correo electrónico"), {
    target: { value: email },
  });
  fireEvent.change(screen.getByLabelText("Contraseña"), {
    target: { value: clave },
  });
  fireEvent.click(screen.getByRole("button", { name: "Ingresar" }));
}

describe("Login", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("envia POST /api/auth/session con el body correcto y guarda el token", async () => {
    vi.mocked(fetch).mockResolvedValue(respuestaLogin());

    montar();
    await completarYEnviar("admin@redes.co", "intela-dev");

    await waitFor(() =>
      expect(screen.getByText("panel de control")).toBeTruthy(),
    );

    const [path, init] = vi.mocked(fetch).mock.calls[0];
    expect(path).toBe("/api/auth/session");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(init?.body as string)).toEqual({
      email: "admin@redes.co",
      clave: "intela-dev",
    });
    expect(token()).toBe("tok-123");
  });

  it("navega al destino previo (state.from) en vez de a /", async () => {
    vi.mocked(fetch).mockResolvedValue(respuestaLogin());

    montar([
      { pathname: "/login", state: { from: { pathname: "/catalogo" } } },
    ]);
    await completarYEnviar("admin@redes.co", "intela-dev");

    await waitFor(() =>
      expect(screen.getByText("pantalla de catálogo")).toBeTruthy(),
    );
  });

  it("un 401 muestra 'credenciales invalidas' y NO redirige (regresión de D2)", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ error: "credenciales invalidas" }), {
        status: 401,
      }),
    );

    montar();
    await completarYEnviar("admin@redes.co", "mala");

    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
    expect(screen.getByRole("alert").textContent).toBe(
      "credenciales invalidas",
    );
    expect(screen.queryByText("panel de control")).toBeNull();
    expect(token()).toBe("");
  });

  it("un fallo de red muestra el mensaje de ErrorDeRed", async () => {
    vi.mocked(fetch).mockRejectedValue(new TypeError("Failed to fetch"));

    montar();
    await completarYEnviar("admin@redes.co", "intela-dev");

    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
    expect(screen.getByRole("alert").textContent).toBe(
      "no se pudo contactar al servidor",
    );
  });

  it("el botón de recuperar clave esta deshabilitado: no hay pantalla detrás (M-8)", () => {
    montar();
    const boton = screen.getByRole("button", {
      name: "¿Olvidaste tu contraseña?",
    }) as HTMLButtonElement;
    expect(boton.disabled).toBe(true);
  });
});
