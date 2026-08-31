import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Login from "./Login";
import { token } from "./api";
import { ProveedorDeSesion, useSesion } from "./sesion";

/** Expone la URL actual completa para poder afirmar sobre query y fragmento. */
function Ubicacion() {
  const { pathname, search, hash } = useLocation();
  return <span data-testid="ubicacion">{pathname + search + hash}</span>;
}

function montar(
  initialEntries: (string | { pathname: string; state?: unknown })[] = [
    "/login",
  ],
) {
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <ProveedorDeSesion>
        <Ubicacion />
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

  it("restaura la URL COMPLETA: conserva query y fragmento, no solo la ruta", async () => {
    // Los filtros de tabla viven en la query (Sprint 3): quedarse con el
    // pathname devolveria /catalogo cuando se habia pedido la pagina 2.
    vi.mocked(fetch).mockResolvedValue(respuestaLogin());

    montar([
      {
        pathname: "/login",
        state: {
          from: {
            pathname: "/catalogo",
            search: "?pagina=2&genero=serie",
            hash: "#resultados",
          },
        },
      },
    ]);
    await completarYEnviar("admin@redes.co", "intela-dev");

    await waitFor(() =>
      expect(screen.getByText("pantalla de catálogo")).toBeTruthy(),
    );
    expect(screen.getByTestId("ubicacion").textContent).toBe(
      "/catalogo?pagina=2&genero=serie#resultados",
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

  it("avisa si la salida anterior no fue revocada por el servidor", async () => {
    // El aviso vive en el contexto de sesion, no en Layout: al salir, Layout
    // se desmonta y nadie alcanzaria a leerlo. Se muestra aqui, que es donde
    // se aterriza despues de salir.
    function ConSalida() {
      const { salir, salidaSinRevocar } = useSesion();
      return (
        <>
          <button onClick={() => void salir()}>forzar salida</button>
          {salidaSinRevocar && <Login />}
        </>
      );
    }

    // El DELETE falla: el servidor no confirma la revocacion.
    vi.mocked(fetch).mockRejectedValue(new TypeError("Failed to fetch"));

    render(
      <MemoryRouter initialEntries={["/login"]}>
        <ProveedorDeSesion>
          <ConSalida />
        </ProveedorDeSesion>
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByText("forzar salida"));

    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toContain(
        "no confirmó la revocación",
      ),
    );
  });

  it("no muestra el aviso de revocacion en un login normal", async () => {
    montar();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("el botón de recuperar clave esta deshabilitado: no hay pantalla detrás (M-8)", () => {
    montar();
    const boton = screen.getByRole("button", {
      name: "¿Olvidaste tu contraseña?",
    }) as HTMLButtonElement;
    expect(boton.disabled).toBe(true);
  });
});
