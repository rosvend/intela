import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import { setToken, token } from "./api";
import { ProveedorDeSesion } from "./sesion";

/** Sonda de ubicacion real, para afirmar sobre a donde se aterrizo tras el login. */
function Ubicacion() {
  const { pathname } = useLocation();
  return <span data-testid="ubicacion">{pathname}</span>;
}

function montar(initialEntries: string[] = ["/"]) {
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <ProveedorDeSesion>
        <Ubicacion />
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

  it("un token que caduca CON la pantalla ya abierta conserva a donde volver, no solo el caso sin token", async () => {
    // Reproduce en vivo: estando en /estado, un 401 real (sesion borrada en
    // el servidor) debia devolver a /estado tras volver a entrar, y volvia a
    // "/" -el handler de api.ts navegaba el mismo sin `state.from`, ganando
    // la carrera contra el <Navigate> de RutaProtegida. Este es el camino
    // distinto al de la prueba anterior: aqui la sesion es valida AL MONTAR
    // y caduca despues, mientras una pantalla ya esta usando el token -pasa
    // por `alExpirarSesion` dentro de `api()`, no por el catch inicial de
    // ProveedorDeSesion.
    setToken("token-que-va-a-caducar");
    vi.mocked(fetch).mockResolvedValueOnce(respuestaUsuario("administrador"));
    // El fetch de Estado.tsx (via useApi) llega despues: ahi es donde caduca.
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "sesion invalida o expirada" }), {
        status: 401,
      }),
    );

    montar(["/estado"]);

    await waitFor(() =>
      expect(
        screen.getByRole("heading", { name: "Iniciar sesión" }),
      ).toBeTruthy(),
    );

    // Re-login: se vuelve a /estado, no a "/".
    vi.mocked(fetch).mockResolvedValueOnce(respuestaLogin());
    fireEvent.change(screen.getByLabelText("Correo electrónico"), {
      target: { value: "admin@redes.co" },
    });
    fireEvent.change(screen.getByLabelText("Contraseña"), {
      target: { value: "intela-dev" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Ingresar" }));

    await waitFor(() =>
      expect(screen.getByTestId("ubicacion").textContent).toBe("/estado"),
    );
  });
});

function respuestaUsuario(rol: string) {
  return new Response(
    JSON.stringify({
      id: "usr-1",
      email: "admin@redes.co",
      nombre: "Admin Intela",
      rol,
      titular_id: "",
    }),
    { headers: { "content-type": "application/json" } },
  );
}
