import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Inicio from "../Inicio";
import { setToken } from "../api";
import { resumenDePrueba } from "../reparto/resumenDePrueba";
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

/** Responde la sesion con `rol` y el resto con `rutas` (o 404). */
function servir(
  rol: Rol,
  rutas: Record<string, () => Response | Promise<Response>> = {},
  nombre?: string,
) {
  vi.mocked(fetch).mockImplementation(async (input) => {
    const path = String(input);
    if (path === "/api/auth/session") return json(usuario(rol, nombre));
    const ruta = Object.keys(rutas).find(
      (r) => path === r || path.startsWith(`${r}?`),
    );
    return ruta ? rutas[ruta]() : json({ error: "ruta no encontrada" }, 404);
  });
}

/** La tarjeta (o panel) por el titulo que pinta en su `h2`. */
function bloque(titulo: string): HTMLElement {
  const contenedor = screen
    .getByRole("heading", { name: titulo })
    .closest("article, section");
  if (!contenedor) throw new Error(`no se encontro "${titulo}"`);
  return contenedor as HTMLElement;
}

/** Igual que `bloque`, esperando a que la sesion monte el tablero. */
async function encontrar(titulo: string): Promise<HTMLElement> {
  await screen.findByRole("heading", { name: titulo });
  return bloque(titulo);
}

function alertaDe(titulo: string): HTMLElement {
  return within(bloque(titulo)).getByRole("alert");
}

const KPIS = [
  "Cargas por procesar",
  "Obras en reserva",
  "Obras sin identificar",
  "Última distribución",
];

const PROCESO = {
  id: "proc-2025",
  circuito: "nacional",
  etapa: "verificacion",
  periodo: "2025",
  revision: 1,
  firmas: [{ rol: "distribucion", actor_id: "usr-d", sobre_rev: 1 }],
};

describe("Inicio de staff", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    setToken("tok");
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("saluda por el nombre y muestra los cuatro indicadores y los paneles", async () => {
    servir("administrador", {}, "Admin Intela");

    montar();

    expect(
      await screen.findByRole("heading", { level: 1, name: "Hola, Admin" }),
    ).toBeTruthy();
    for (const kpi of KPIS) {
      expect(screen.getByRole("heading", { name: kpi })).toBeTruthy();
    }
    for (const panel of [
      "Recaudo del periodo por fuente",
      "Distribución en curso",
      "Alertas",
      "Actividad reciente",
    ]) {
      expect(screen.getByRole("heading", { name: panel })).toBeTruthy();
    }
    expect(screen.getByRole("link", { name: "Nueva ingesta" })).toBeTruthy();
  });

  it("contabilidad no ve Nueva ingesta ni la bitacora que no puede leer", async () => {
    servir("contabilidad");

    montar();

    await screen.findByRole("heading", { name: "Cargas por procesar" });
    expect(screen.queryByRole("link", { name: "Nueva ingesta" })).toBeNull();
    expect(
      screen.queryByRole("heading", { name: "Actividad reciente" }),
    ).toBeNull();
  });

  it("cada widget sin backend muestra el vacio, sin alerta", async () => {
    servir("administrador");

    montar();

    await waitFor(() =>
      expect(
        within(bloque("Cargas por procesar")).getByText("Sin datos todavía"),
      ).toBeTruthy(),
    );
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("un indicador con datos pinta el conteo y uno con 500 alerta solo en su tarjeta", async () => {
    servir("administrador", {
      "/api/tablero/oni": () => json({ total: 12 }),
      "/api/tablero/cargas-pendientes": () =>
        json({ error: "la base esta caida" }, 500),
    });

    montar();

    await waitFor(() =>
      expect(
        within(bloque("Obras sin identificar")).getByText("12"),
      ).toBeTruthy(),
    );
    expect(alertaDe("Cargas por procesar").textContent).toBe(
      "la base esta caida",
    );
    expect(screen.getAllByRole("alert")).toHaveLength(1);
  });

  it("un 2xx con el cuerpo ilegible se ve con su mensaje, y un 500 con el suyo", async () => {
    const cuerpoIlegible = '{"total": 12';
    servir("administrador", {
      "/api/tablero/oni": () =>
        new Response(cuerpoIlegible, {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      "/api/tablero/cargas-pendientes": () =>
        json({ error: "la base esta caida" }, 500),
    });

    montar();

    await waitFor(() => expect(screen.getAllByRole("alert").length).toBe(2));
    expect(alertaDe("Obras sin identificar").textContent).toBe(
      "la respuesta llegó sin un cuerpo legible",
    );
    expect(alertaDe("Cargas por procesar").textContent).toBe(
      "la base esta caida",
    );
  });

  it("control negativo: un fallo de red sigue siendo el vacio, no una alerta", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") return json(usuario("administrador"));
      if (path === "/api/tablero/oni") throw new TypeError("Failed to fetch");
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await waitFor(() =>
      expect(
        within(bloque("Obras sin identificar")).getByText("Sin datos todavía"),
      ).toBeTruthy(),
    );
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("Obras en reserva explica que la obra entera queda retenida", async () => {
    servir("administrador", {
      "/api/tablero/obras-en-reserva": () => json({ total: 3 }),
    });

    montar();

    await within(await encontrar("Obras en reserva")).findByText("3");
    fireEvent.click(
      screen.getByRole("button", { name: "Qué significa Obras en reserva" }),
    );
    expect(screen.getByRole("dialog").textContent).toMatch(/no suman 100%/);
  });

  it("sin bolsas registradas el recaudo usa datos de demostración y lo dice", async () => {
    servir("administrador");

    montar();

    const recaudo = await encontrar("Recaudo del periodo por fuente");
    await within(recaudo).findByText("Datos de demostración");
    expect(
      within(within(recaudo).getByRole("list")).getByText("Caracol"),
    ).toBeTruthy();
    expect(within(recaudo).getByText("$ 1.890.000.000")).toBeTruthy();
    expect(
      within(recaudo).getByRole("group", { name: "Recaudo por fuente" }),
    ).toBeTruthy();
  });

  it("con bolsas reales pinta su total por fuente, sin el chip de demostración", async () => {
    servir("administrador", {
      "/api/bolsas": () =>
        json([
          {
            id: "b1",
            usuario_id: "caracol",
            periodo: "2025",
            circuito: "nacional",
            bruto: "1000000.00",
          },
          {
            id: "b2",
            usuario_id: "rcn",
            periodo: "2025",
            circuito: "nacional",
            bruto: "500000.00",
          },
        ]),
    });

    montar();

    const recaudo = await encontrar("Recaudo del periodo por fuente");
    await within(recaudo).findByText("$ 1.500.000");
    expect(within(recaudo).queryByText("Datos de demostración")).toBeNull();
    expect(
      within(within(recaudo).getByRole("list")).getByText("RCN"),
    ).toBeTruthy();
  });

  it("la distribución en curso muestra la etapa y la firma que falta", async () => {
    servir("administrador", {
      "/api/procesos": () => json([PROCESO]),
      "/api/alertas/resumen": () => json(resumenDePrueba()),
    });

    montar();

    const panel = await encontrar("Distribución en curso");
    await within(panel).findByText("Etapa 6 de 9");
    expect(
      within(panel).getByRole("listitem", { current: "step" }).textContent,
    ).toBe("Verificación");
    expect(
      within(panel).getByText(/Falta la firma de Contabilidad/),
    ).toBeTruthy();
    expect(
      within(panel)
        .getByRole("link", { name: "Abrir distribución" })
        .getAttribute("href"),
    ).toBe("/distribucion/proc-2025");
  });

  it("las alertas resumen las abiertas y dicen si alguna bloquea", async () => {
    servir("administrador", {
      "/api/procesos": () => json([PROCESO]),
      "/api/alertas/resumen": () =>
        json(
          resumenDePrueba({
            abiertas: 5,
            criticas_abiertas: 2,
            porTipo: { oni: 3, duplicado_registro: 2 },
          }),
        ),
    });

    montar();

    const panel = await encontrar("Alertas");
    await within(panel).findByText("2 críticas bloquean la distribución");
    expect(within(panel).getByText("5")).toBeTruthy();
    expect(within(panel).getByText("ONI")).toBeTruthy();
    expect(
      within(panel)
        .getByRole("link", { name: "Resolver alertas" })
        .getAttribute("href"),
    ).toBe("/anomalias?periodo=2025");
  });

  it("la actividad reciente habla en lenguaje llano y con tiempo relativo", async () => {
    const haceUnRato = new Date(Date.now() - 5 * 60_000).toISOString();
    servir("administrador", {
      "/api/auditoria/asientos": () =>
        json([
          {
            id: "a1",
            hecho: "proceso.etapa_avanzada",
            ref_tipo: "proceso",
            ref_id: "proc-2025",
            actor: "usr-admin",
            payload: { etapa: "verificacion" },
            cuando: haceUnRato,
          },
        ]),
    });

    montar();

    const panel = await encontrar("Actividad reciente");
    await within(panel).findByText("La distribución avanzó a Verificación");
    expect(within(panel).getByText(/hace 5 minutos/)).toBeTruthy();
  });
});

describe("Inicio del titular", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    setToken("tok");
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("el titular no ve el panel de staff ni dispara sus pedidos", async () => {
    servir("titular", {}, "Ana Escritora");

    montar();

    await waitFor(() =>
      expect(
        vi
          .mocked(fetch)
          .mock.calls.some(([u]) => String(u) !== "/api/auth/session"),
      ).toBe(true),
    );
    expect(
      screen.queryByRole("heading", { name: "Cargas por procesar" }),
    ).toBeNull();
    const paths = vi.mocked(fetch).mock.calls.map(([u]) => String(u));
    expect(paths).not.toContain("/api/tablero/cargas-pendientes");
    expect(paths).not.toContain("/api/procesos");
    expect(paths).not.toContain("/api/bolsas");
  });
});
