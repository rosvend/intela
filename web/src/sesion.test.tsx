import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ProveedorDeSesion, useSesion } from "./sesion";
import { setToken } from "./api";

function Sonda() {
  const { usuario, cargando } = useSesion();
  if (cargando) return <p>cargando</p>;
  return <p>{usuario ? `hola ${usuario.nombre}` : "sin sesion"}</p>;
}

function montar() {
  return render(
    <MemoryRouter>
      <ProveedorDeSesion>
        <Sonda />
      </ProveedorDeSesion>
    </MemoryRouter>,
  );
}

function respuestaUsuario(nombre: string) {
  return new Response(
    JSON.stringify({
      id: "usr-1",
      email: "x@redes.co",
      nombre,
      rol: "administrador",
      titular_id: "",
    }),
    { headers: { "content-type": "application/json" } },
  );
}

/** Monta, espera a que resuelva la sesion, pulsa salir y espera a que termine. */
async function montarYSalir() {
  let termino = false;

  function ConBotonDeSalida() {
    const { salir, cargando } = useSesion();
    if (cargando) return <p>cargando</p>;
    return (
      <button
        onClick={() => {
          void salir().then(() => (termino = true));
        }}
      >
        salir
      </button>
    );
  }

  render(
    <MemoryRouter>
      <ProveedorDeSesion>
        <ConBotonDeSalida />
      </ProveedorDeSesion>
    </MemoryRouter>,
  );

  await waitFor(() => expect(screen.getByText("salir")).toBeTruthy());
  fireEvent.click(screen.getByText("salir"));
  await waitFor(() => expect(termino).toBe(true));
}

describe("ProveedorDeSesion", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("sin token guardado, termina de cargar sin sesion y sin llamar a la API", async () => {
    montar();

    await waitFor(() => expect(screen.getByText("sin sesion")).toBeTruthy());
    expect(fetch).not.toHaveBeenCalled();
  });

  it("con token guardado, resuelve la sesion contra GET /auth/session", async () => {
    setToken("token-valido");
    vi.mocked(fetch).mockResolvedValue(
      new Response(
        JSON.stringify({
          id: "usr-1",
          email: "ana@redes.co",
          nombre: "Ana Escritora",
          rol: "titular",
          titular_id: "tit-ana",
        }),
        { headers: { "content-type": "application/json" } },
      ),
    );

    montar();

    await waitFor(() =>
      expect(screen.getByText("hola Ana Escritora")).toBeTruthy(),
    );
  });

  it("con un token que la API ya no reconoce, termina sin sesion en vez de romper", async () => {
    setToken("token-vencido");
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ error: "sesion invalida o expirada" }), {
        status: 401,
      }),
    );

    montar();

    await waitFor(() => expect(screen.getByText("sin sesion")).toBeTruthy());
  });

  it("si otra pestana cierra sesion, esta se entera y deja de mostrar al usuario", async () => {
    setToken("token-valido");
    vi.mocked(fetch).mockResolvedValue(respuestaUsuario("Ana Escritora"));

    montar();
    await waitFor(() =>
      expect(screen.getByText("hola Ana Escritora")).toBeTruthy(),
    );

    // El evento `storage` es lo que el navegador dispara en las OTRAS pestanas.
    act(() => {
      localStorage.removeItem("intela.token");
      window.dispatchEvent(
        new StorageEvent("storage", { key: "intela.token", newValue: null }),
      );
    });

    await waitFor(() => expect(screen.getByText("sin sesion")).toBeTruthy());
  });

  it("si otra pestana entra con otra cuenta, esta reresuelve y no muestra al usuario viejo", async () => {
    setToken("token-de-ana");
    vi.mocked(fetch).mockResolvedValueOnce(respuestaUsuario("Ana Escritora"));

    montar();
    await waitFor(() =>
      expect(screen.getByText("hola Ana Escritora")).toBeTruthy(),
    );

    // Otra pestana inicia sesion como otro usuario: el token cambia.
    vi.mocked(fetch).mockResolvedValueOnce(respuestaUsuario("Admin Intela"));
    act(() => {
      localStorage.setItem("intela.token", "token-de-admin");
      window.dispatchEvent(
        new StorageEvent("storage", {
          key: "intela.token",
          newValue: "token-de-admin",
        }),
      );
    });

    await waitFor(() =>
      expect(screen.getByText("hola Admin Intela")).toBeTruthy(),
    );
  });

  it("ignora cambios de otras claves del almacen", async () => {
    setToken("token-valido");
    vi.mocked(fetch).mockResolvedValue(respuestaUsuario("Ana Escritora"));

    montar();
    await waitFor(() =>
      expect(screen.getByText("hola Ana Escritora")).toBeTruthy(),
    );

    act(() => {
      window.dispatchEvent(
        new StorageEvent("storage", { key: "otra.cosa", newValue: "x" }),
      );
    });

    expect(screen.getByText("hola Ana Escritora")).toBeTruthy();
  });
});

describe("salir()", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("pide la revocacion al servidor y limpia el token", async () => {
    setToken("tok");
    vi.mocked(fetch).mockResolvedValueOnce(respuestaUsuario("Admin"));
    vi.mocked(fetch).mockResolvedValueOnce(new Response(null, { status: 204 }));

    await montarYSalir();

    const [ruta, init] = vi.mocked(fetch).mock.calls[1];
    expect(String(ruta)).toContain("/api/auth/session");
    expect(init?.method).toBe("DELETE");
    expect(localStorage.getItem("intela.token")).toBeNull();
  });

  it("si el servidor no responde, sale igual en local", async () => {
    setToken("tok");
    vi.mocked(fetch).mockResolvedValueOnce(respuestaUsuario("Admin"));
    vi.mocked(fetch).mockRejectedValueOnce(new TypeError("Failed to fetch"));

    await montarYSalir();

    expect(localStorage.getItem("intela.token")).toBeNull();
  });

  it("un 500 en el DELETE tampoco deja el token en el equipo", async () => {
    setToken("tok");
    vi.mocked(fetch).mockResolvedValueOnce(respuestaUsuario("Admin"));
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "error interno" }), {
        status: 500,
      }),
    );

    await montarYSalir();

    expect(localStorage.getItem("intela.token")).toBeNull();
  });
});
