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
import { useFijarPendientes } from "./identificacion/pendientes";
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

function json(cuerpo: unknown, status = 200) {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function urlDe(input: Parameters<typeof fetch>[0]): string {
  return typeof input === "string" ? input : input.toString();
}

/** Responde el conteo de pendientes segun la URL, y la sesion en lo demas. */
function conFetchDelBadge(rol: string, pendientes: number) {
  vi.mocked(fetch).mockImplementation((input) => {
    if (urlDe(input).includes("/identificacion/casos")) {
      return Promise.resolve(json({ pendientes, casos: [] }));
    }
    return Promise.resolve(respuestaUsuario(rol));
  });
}

/** Ruta hija de prueba: empuja un nuevo conteo al badge del shell. */
function HijoQueFijaPendientes({ valor }: { valor: number }) {
  const fijarPendientes = useFijarPendientes();
  return (
    <button type="button" onClick={() => fijarPendientes(valor)}>
      fijar pendientes
    </button>
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

  it("con rol titular el sidebar tiene Inicio y Mis liquidaciones", async () => {
    setToken("tok");
    vi.mocked(fetch).mockResolvedValue(respuestaUsuario("titular"));

    montar();

    await waitFor(() => expect(screen.getAllByRole("link").length).toBe(2));
    expect(screen.getByRole("link", { name: "Inicio" })).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "Mis liquidaciones" }),
    ).toBeTruthy();
    expect(screen.queryByText("Configuración")).toBeNull();
  });

  it("ofrece la burbuja del asistente a cualquier rol, tambien al titular", async () => {
    setToken("tok");
    vi.mocked(fetch).mockResolvedValue(respuestaUsuario("titular"));

    montar();

    expect(
      await screen.findByRole("button", { name: "Abrir asistente" }),
    ).toBeTruthy();
  });

  it("con rol administrador el sidebar tiene once enlaces en sus dos secciones", async () => {
    setToken("tok");
    vi.mocked(fetch).mockResolvedValue(respuestaUsuario("administrador"));

    montar();

    await waitFor(() => expect(screen.getAllByRole("link").length).toBe(11));
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

  it("el perfil abre un menu con Configuración y Salir, con iconos", async () => {
    setToken("tok");
    vi.mocked(fetch).mockResolvedValue(respuestaUsuario("administrador"));

    montar();

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Abrir menú de usuario" }),
      ).toBeTruthy(),
    );
    // Cada enlace del sidebar y el perfil traen su icono Heroicons.
    expect(
      screen.getByRole("link", { name: "Inicio" }).querySelector("svg"),
    ).not.toBeNull();
    expect(
      screen
        .getByRole("button", { name: "Abrir menú de usuario" })
        .querySelector("svg"),
    ).not.toBeNull();
    // Cerrado al inicio: no hay menu ni item de salida a la vista.
    expect(screen.queryByRole("menu")).toBeNull();
    expect(screen.queryByRole("menuitem", { name: "Salir" })).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "Abrir menú de usuario" }),
    );

    const configuracion = screen.getByRole("menuitem", {
      name: "Configuración",
    });
    const salir = screen.getByRole("menuitem", { name: "Salir" });
    expect(configuracion.querySelector("svg")).not.toBeNull();
    expect(salir.querySelector("svg")).not.toBeNull();
  });

  it("Configuración lleva al primer modulo de esa seccion (Deducciones)", async () => {
    setToken("tok");
    vi.mocked(fetch).mockResolvedValue(respuestaUsuario("administrador"));

    render(
      <MemoryRouter initialEntries={["/"]}>
        <ProveedorDeSesion>
          <Routes>
            <Route element={<Layout />}>
              <Route index element={<p>contenido de inicio</p>} />
              <Route
                path="/deducciones"
                element={<p>pantalla de deducciones</p>}
              />
            </Route>
          </Routes>
        </ProveedorDeSesion>
      </MemoryRouter>,
    );

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Abrir menú de usuario" }),
      ).toBeTruthy(),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Abrir menú de usuario" }),
    );
    fireEvent.click(screen.getByRole("menuitem", { name: "Configuración" }));

    await waitFor(() =>
      expect(screen.getByText("pantalla de deducciones")).toBeTruthy(),
    );
  });

  it("un fallo de render queda dentro de la pagina, y navegar fuera la recupera", async () => {
    setToken("tok");
    vi.mocked(fetch).mockResolvedValue(respuestaUsuario("auditor"));
    const silencio = vi.spyOn(console, "error").mockImplementation(() => {});
    function PaginaRota(): never {
      throw new RangeError("importe ilegible");
    }

    render(
      <MemoryRouter initialEntries={["/auditoria"]}>
        <ProveedorDeSesion>
          <Routes>
            <Route element={<Layout />}>
              <Route index element={<p>contenido de inicio</p>} />
              <Route path="/auditoria" element={<PaginaRota />} />
            </Route>
          </Routes>
        </ProveedorDeSesion>
      </MemoryRouter>,
    );

    await waitFor(() =>
      expect(
        screen.getByRole("heading", {
          name: "No se pudo mostrar esta pantalla",
        }),
      ).toBeTruthy(),
    );
    // El shell sigue en pie: el fallo no tumba la app.
    expect(screen.getByRole("link", { name: "Inicio" })).toBeTruthy();

    fireEvent.click(screen.getByRole("link", { name: "Inicio" }));

    await waitFor(() =>
      expect(screen.getByText("contenido de inicio")).toBeTruthy(),
    );
    expect(screen.queryByRole("alert")).toBeNull();
    silencio.mockRestore();
  });

  it("logout llama a DELETE /auth/session y limpia el token", async () => {
    // Con rol administrador, `Layout` tambien pide el conteo de pendientes
    // (badge de /identificacion) apenas monta: encolar por ORDEN asumiria que
    // la segunda llamada es el DELETE del logout, y esa peticion del badge se
    // la roba. Se responde por URL y METODO -lo que de verdad identifica cada
    // peticion-, y el DELETE se busca igual, no por indice de `mock.calls`.
    setToken("tok");
    vi.mocked(fetch).mockImplementation((input, init) => {
      const url = typeof input === "string" ? input : input.toString();
      const metodo = init?.method ?? "GET";
      if (metodo === "DELETE" && url.includes("/api/auth/session")) {
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      return Promise.resolve(respuestaUsuario("administrador"));
    });

    montar();
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Abrir menú de usuario" }),
      ).toBeTruthy(),
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Abrir menú de usuario" }),
    );
    fireEvent.click(screen.getByRole("menuitem", { name: "Salir" }));

    await waitFor(() => expect(token()).toBe(""));
    const llamadaDelete = vi.mocked(fetch).mock.calls.find(([input, init]) => {
      const url = typeof input === "string" ? input : input.toString();
      return init?.method === "DELETE" && url.includes("/api/auth/session");
    });
    expect(llamadaDelete).toBeTruthy();
    expect(llamadaDelete?.[1]?.method).toBe("DELETE");
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

  it("el administrador ve el badge de Identificación con el conteo de pendientes", async () => {
    setToken("tok");
    conFetchDelBadge("administrador", 3);

    montar();

    const enlace = await screen.findByRole("link", {
      name: /Identificación/,
    });
    await waitFor(() => expect(enlace.textContent).toContain("3"));
    // El numero es lo unico visible; " pendientes" es solo para el lector.
    expect(enlace.querySelector(".solo-lector")?.textContent).toBe(
      " pendientes",
    );
  });

  it("un rol que no es administrador no pide el conteo de pendientes en absoluto", async () => {
    setToken("tok");
    vi.mocked(fetch).mockResolvedValue(respuestaUsuario("titular"));

    montar();

    await waitFor(() => expect(screen.getAllByRole("link").length).toBe(2));
    const rutasPedidas = vi
      .mocked(fetch)
      .mock.calls.map(([input]) => urlDe(input));
    expect(
      rutasPedidas.some((ruta) => ruta.includes("/identificacion/casos")),
    ).toBe(false);
  });

  it("con pendientes en 0 el badge de Identificación no se pinta", async () => {
    setToken("tok");
    conFetchDelBadge("administrador", 0);

    montar();

    const enlace = await screen.findByRole("link", {
      name: /Identificación/,
    });
    await waitFor(() =>
      expect(
        vi
          .mocked(fetch)
          .mock.calls.some(([input]) =>
            urlDe(input).includes("/identificacion/casos"),
          ),
      ).toBe(true),
    );
    await waitFor(() =>
      expect(enlace.querySelector(".sidebar-badge")).toBeNull(),
    );
  });

  it("con un 404 en el conteo de pendientes el badge de Identificación no se pinta", async () => {
    setToken("tok");
    vi.mocked(fetch).mockImplementation((input) => {
      if (urlDe(input).includes("/identificacion/casos")) {
        return Promise.resolve(json({ error: "no encontrado" }, 404));
      }
      return Promise.resolve(respuestaUsuario("administrador"));
    });

    montar();

    const enlace = await screen.findByRole("link", {
      name: /Identificación/,
    });
    await waitFor(() =>
      expect(
        vi
          .mocked(fetch)
          .mock.calls.some(([input]) =>
            urlDe(input).includes("/identificacion/casos"),
          ),
      ).toBe(true),
    );
    await waitFor(() =>
      expect(enlace.querySelector(".sidebar-badge")).toBeNull(),
    );
  });

  it("una ruta hija que llama a useFijarPendientes cambia el badge del shell (D1)", async () => {
    setToken("tok");
    conFetchDelBadge("administrador", 2);

    render(
      <MemoryRouter initialEntries={["/identificacion"]}>
        <ProveedorDeSesion>
          <Routes>
            <Route element={<Layout />}>
              <Route
                path="/identificacion"
                element={<HijoQueFijaPendientes valor={9} />}
              />
            </Route>
          </Routes>
        </ProveedorDeSesion>
      </MemoryRouter>,
    );

    const enlace = await screen.findByRole("link", {
      name: /Identificación/,
    });
    await waitFor(() => expect(enlace.textContent).toContain("2"));

    fireEvent.click(screen.getByRole("button", { name: "fijar pendientes" }));

    await waitFor(() => expect(enlace.textContent).toContain("9"));
  });
});
