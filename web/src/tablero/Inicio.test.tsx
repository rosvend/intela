import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
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

/** La tarjeta de un KPI, por el titulo que `Tarjeta` pinta en su `h2`. */
function tarjeta(titulo: string): HTMLElement {
  const articulo = screen
    .getByRole("heading", { name: titulo })
    .closest("article");
  if (!articulo) throw new Error(`no se encontro la tarjeta "${titulo}"`);
  return articulo;
}

/** El mensaje que una tarjeta muestra cuando su recurso fallo (Tarjeta.tsx). */
function alertaDe(titulo: string): HTMLElement {
  return within(tarjeta(titulo)).getByRole("alert");
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

  it("el titular ve su liquidacion y el panel de ingresos montado en #ingresos", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url === "/api/auth/session") {
        return json(usuario("titular", "Ana Escritora"));
      }
      if (url.startsWith("/api/mis-ingresos")) {
        return json({
          ingresos: [
            {
              ref: "proc-2026-01:obra-completa:tit-1",
              obra_id: "obra-completa",
              obra: "La Casa de las Dos Palmas",
              fuente: "caracol",
              periodo: "2026-01",
              neto: "3600.00",
            },
          ],
        });
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
    expect(
      screen.getByRole("heading", { name: "Mis ingresos" }),
    ).toBeTruthy();
    expect(
      await screen.findByRole("cell", { name: "La Casa de las Dos Palmas" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Explicar esta cifra" }),
    ).toBeTruthy();
    expect(document.getElementById("ingresos")).toBeTruthy();
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

  it("un 2xx con el cuerpo ilegible se ve con su mensaje, y un 500 con el suyo", async () => {
    // `useRecurso` clasifica el error de cada widget con la union de errores
    // tipados. Un 2xx cuyo cuerpo no parsea llega como `ErrorDeCuerpoIlegible`
    // -el servidor contesto bien y lo que no llego fue el cuerpo- y sin entrar
    // en la union la tarjeta lo cambiaba por el generico "no se pudo cargar
    // este indicador", que dice menos de lo que se sabe.
    const cuerpoIlegible = '{"total": 12';
    const respuestaOni = new Response(cuerpoIlegible, {
      status: 200,
      headers: { "content-type": "application/json" },
    });

    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") {
        return json(usuario("administrador"));
      }
      if (path === "/api/tablero/oni") {
        return respuestaOni;
      }
      if (path === "/api/tablero/cargas-pendientes") {
        return json({ error: "la base esta caida" }, 500);
      }
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await waitFor(() => expect(screen.getAllByRole("alert").length).toBe(2));

    // Precondicion: el doble contesto un 2xx con un cuerpo que no parsea. Un
    // doble que devolviera un 500 mediria el camino del `ApiError` y la prueba
    // pasaria por el motivo equivocado.
    expect(respuestaOni.ok).toBe(true);
    expect(respuestaOni.status).toBe(200);
    expect(respuestaOni.headers.get("content-type")).toContain("json");
    await expect(
      new Response(cuerpoIlegible, {
        headers: { "content-type": "application/json" },
      }).json(),
    ).rejects.toThrow();

    expect(alertaDe("ONI").textContent).toBe(
      "la respuesta llegó sin un cuerpo legible",
    );

    // Control negativo: un 500 con mensaje propio sigue llegando con el suyo,
    // en su propia tarjeta. Sin esto, un arreglo que metiera los dos errores en
    // el mismo saco pasaria la afirmacion de arriba.
    expect(alertaDe("Cargas pendientes").textContent).toBe(
      "la base esta caida",
    );
    expect(alertaDe("ONI").textContent).not.toBe(
      alertaDe("Cargas pendientes").textContent,
    );
  });

  it("control negativo: un fallo de red sigue siendo el vacio, no una alerta con mensaje", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") {
        return json(usuario("administrador"));
      }
      if (path === "/api/tablero/oni") {
        throw new TypeError("Failed to fetch");
      }
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await waitFor(() =>
      expect(
        within(tarjeta("ONI")).getByText("Sin datos todavía"),
      ).toBeTruthy(),
    );
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
