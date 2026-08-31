import { cleanup, render, screen, waitFor } from "@testing-library/react";
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
});
