import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import { setToken, token } from "./api";
import { ProveedorDeSesion } from "./sesion";

function montar(initialEntries: string[] = ["/"]) {
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <ProveedorDeSesion>
        <App />
      </ProveedorDeSesion>
    </MemoryRouter>,
  );
}

function respuestaLogin() {
  return new Response(
    JSON.stringify({
      token: "tok-flujo",
      expira: "2026-01-01T00:00:00Z",
      usuario: {
        id: "usr-1",
        email: "admin@redes.co",
        nombre: "Admin Intela",
        rol: "administrador",
        titular_id: "",
      },
    }),
    { headers: { "content-type": "application/json" } },
  );
}

describe("flujo de autenticacion (integracion)", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("visita sin token -> login -> login OK -> dashboard, sin pantalla en blanco en ningun punto", async () => {
    montar();

    // Sin token, RutaProtegida redirige de inmediato a /login.
    await waitFor(() =>
      expect(
        screen.getByRole("heading", { name: "Iniciar sesión" }),
      ).toBeTruthy(),
    );
    expect(document.body.textContent).not.toBe("");

    // Login exitoso: guarda el token y muestra el dashboard con el usuario real.
    vi.mocked(fetch).mockResolvedValueOnce(respuestaLogin());
    fireEvent.change(screen.getByLabelText("Correo electrónico"), {
      target: { value: "admin@redes.co" },
    });
    fireEvent.change(screen.getByLabelText("Contraseña"), {
      target: { value: "intela-dev" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Ingresar" }));

    await waitFor(() =>
      expect(
        screen.getByRole("heading", { name: "Panel de control" }),
      ).toBeTruthy(),
    );
    expect(screen.getByText("Admin Intela")).toBeTruthy();
    expect(token()).toBe("tok-flujo");
  });

  it("un token que la API ya no reconoce fuerza el re-login sin pantalla en blanco", async () => {
    // Simula reabrir la app con un token guardado que el servidor ya revoco
    // o dejo expirar: es el momento real en que esta pantalla dispara un 401
    // fuera del propio login (ProveedorDeSesion resolviendo la sesion).
    setToken("token-vencido");
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ error: "sesion invalida o expirada" }), {
        status: 401,
      }),
    );

    montar();

    // Nunca una pantalla en blanco: pasa por el estado de carga...
    await waitFor(() => expect(screen.getByText("Cargando…")).toBeTruthy());
    // ...y termina en /login, no en un contenedor vacio.
    await waitFor(() =>
      expect(
        screen.getByRole("heading", { name: "Iniciar sesión" }),
      ).toBeTruthy(),
    );
    expect(token()).toBe("");
  });
});
