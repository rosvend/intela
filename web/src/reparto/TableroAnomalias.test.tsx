import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setToken } from "../api";
import { ProveedorDeSesion, Rol } from "../sesion";
import { resumenDePrueba } from "./resumenDePrueba";
import TableroAnomalias from "./TableroAnomalias";
import { Alerta, Proceso, ResumenDeAlertas, RUTAS_REPARTO } from "./tipos";

function sesion(rol: Rol) {
  return {
    id: "usr-1",
    email: "x@redes.co",
    nombre: "Persona de Prueba",
    rol,
    titular_id: "",
  };
}

const proc2025: Proceso = {
  id: "proc-1",
  circuito: "nacional",
  etapa: "verificacion",
  periodo: "2025",
  revision: 1,
  firmas: [],
};

const proc2024: Proceso = { ...proc2025, id: "proc-0", periodo: "2024" };

function json(cuerpo: unknown, status = 200) {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function alerta(
  cambios: Partial<Alerta> & Pick<Alerta, "id" | "tipo">,
): Alerta {
  return {
    detalle: "detalle",
    periodo: "2025",
    referencia: "uso:u-1",
    ref_tipo: "uso",
    ref_id: "u-1",
    critica: false,
    detectada: "2026-05-02T08:30:00Z",
    resuelta: false,
    autocerrada: false,
    ...cambios,
  };
}

// El servidor ya filtra las abiertas (`resueltas=false`).
const alertas: Alerta[] = [
  alerta({
    id: "al-1",
    tipo: "oni",
    detalle: "Título no identificado",
    referencia: "uso:uso-9",
  }),
  alerta({ id: "al-2", tipo: "oni", detalle: "Otra ONI" }),
  alerta({
    id: "al-3",
    tipo: "duplicado_archivo",
    detalle: "Mismo SHA-256",
    critica: true,
  }),
];

const resumen2025 = resumenDePrueba({
  abiertas: 3,
  criticas_abiertas: 1,
  porTipo: { oni: 2, duplicado_archivo: 1 },
});

function montar(ruta = "/anomalias?periodo=2025") {
  setToken("tok");
  return render(
    <MemoryRouter initialEntries={[ruta]}>
      <ProveedorDeSesion>
        <Routes>
          <Route path="/anomalias" element={<TableroAnomalias />} />
        </Routes>
      </ProveedorDeSesion>
    </MemoryRouter>,
  );
}

type Rutas = {
  rol?: Rol;
  resumen?: () => Response;
  lista?: () => Response;
  evaluar?: () => Response;
};

function servir({
  rol = "administrador",
  resumen = () => json(resumen2025),
  lista = () => json(alertas),
  evaluar = () => new Response(null, { status: 204 }),
}: Rutas = {}) {
  vi.mocked(fetch).mockImplementation(async (input, init) => {
    const path = String(input);
    if (path === "/api/auth/session") return json(sesion(rol));
    if (path === RUTAS_REPARTO.procesos) return json([proc2025, proc2024]);
    if (path === RUTAS_REPARTO.evaluarAlertas && init?.method === "POST")
      return evaluar();
    if (path === RUTAS_REPARTO.resumenAlertas("2025")) return resumen();
    if (path === RUTAS_REPARTO.alertas("2025")) return lista();
    return json({ error: "ruta no encontrada" }, 404);
  });
}

function llamadas(ruta: string) {
  return vi.mocked(fetch).mock.calls.filter(([url]) => String(url) === ruta);
}

describe("TableroAnomalias", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("muestra las seis tarjetas con los conteos del resumen y enlaza a resolucion", async () => {
    servir();

    montar();

    await screen.findByText("Título no identificado");
    expect(screen.getAllByText("ONI").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Duplicado de archivo").length).toBeGreaterThan(
      0,
    );
    expect(
      screen.getAllByText("Tipo de obra sin mapear").length,
    ).toBeGreaterThan(0);
    expect(
      screen.getAllByRole("link", { name: "Ir a resolución" }),
    ).toHaveLength(6);
    expect(screen.getAllByText("Abiertas · bloquean la corrida")).toHaveLength(
      3,
    );
    expect(screen.getByText(/1 crítica abierta bloquea/)).toBeTruthy();
    expect(
      screen.getAllByRole("link", { name: "Resolver" }).length,
    ).toBeGreaterThan(0);
  });

  it("Resolver no descarta el ?periodo ni repuebla la bandeja con otro", async () => {
    servir();

    montar();

    await screen.findByText("Título no identificado");
    const resolver = screen.getAllByRole("link", { name: "Resolver" })[0];
    expect(resolver?.getAttribute("href")).toBe("#bandeja");

    fireEvent.click(resolver as HTMLElement);

    expect(screen.getByText("Título no identificado")).toBeTruthy();
    expect((screen.getByLabelText("Periodo") as HTMLSelectElement).value).toBe(
      "2025",
    );
  });

  it("sin ?periodo no pide las alertas de todos los periodos", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") return json(sesion("administrador"));
      if (path === RUTAS_REPARTO.procesos) return json([proc2025, proc2024]);
      if (path === RUTAS_REPARTO.alertas("2024")) return json(alertas);
      if (path === RUTAS_REPARTO.resumenAlertas("2024"))
        return json(resumenDePrueba({ periodo: "2024" }));
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar("/anomalias");

    await screen.findByText("Título no identificado");
    const pedidas = vi.mocked(fetch).mock.calls.map(([url]) => String(url));
    expect(pedidas).not.toContain(RUTAS_REPARTO.alertas());
  });

  it("sin backend de alertas muestra el vacio", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      if (String(input) === "/api/auth/session")
        return json(sesion("administrador"));
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar("/anomalias");

    await waitFor(() =>
      expect(screen.getAllByText("Sin datos todavía").length).toBeGreaterThan(
        0,
      ),
    );
    const pedidas = vi.mocked(fetch).mock.calls.map(([url]) => String(url));
    expect(pedidas).not.toContain(RUTAS_REPARTO.alertas());
    expect(screen.getAllByText("Abiertas").length).toBeGreaterThan(0);
  });

  it("un periodo sin evaluar no se presenta como limpio", async () => {
    servir({
      resumen: () => json(resumenDePrueba({ ultima_evaluacion: null })),
      lista: () => json([]),
    });

    montar();

    await screen.findByText(/no se ha evaluado todavía: los conteos/);
    expect(screen.queryByText(/No hay alertas abiertas/)).toBeNull();
    expect(
      screen.getByText("Este periodo no se ha evaluado todavía."),
    ).toBeTruthy();
  });

  it.each<[Rol, boolean]>([
    ["administrador", true],
    ["distribucion", true],
    ["contabilidad", false],
    ["auditor", false],
  ])("el boton Evaluar periodo para %s: %s", async (rol, visible) => {
    servir({
      rol,
      resumen: () => json(resumenDePrueba({ ultima_evaluacion: null })),
      lista: () => json([]),
    });

    montar();

    await screen.findByText(/no se ha evaluado todavía: los conteos/);
    expect(!!screen.queryByRole("button", { name: "Evaluar periodo" })).toBe(
      visible,
    );
  });

  it("evaluar hace POST y vuelve a pedir el resumen", async () => {
    let evaluado = false;
    servir({
      resumen: () =>
        json(
          evaluado ? resumen2025 : resumenDePrueba({ ultima_evaluacion: null }),
        ),
      evaluar: () => {
        evaluado = true;
        return new Response(null, { status: 204 });
      },
    });

    montar();

    fireEvent.click(
      await screen.findByRole("button", { name: "Evaluar periodo" }),
    );

    await screen.findByText(/1 crítica abierta bloquea/);
    const post = llamadas(RUTAS_REPARTO.evaluarAlertas)[0];
    expect(post?.[1]?.method).toBe("POST");
    expect(post?.[1]?.body).toBe('{"periodo":"2025"}');
    expect(llamadas(RUTAS_REPARTO.resumenAlertas("2025")).length).toBe(2);
  });

  it("si evaluar falla muestra el error", async () => {
    servir({
      resumen: () => json(resumenDePrueba({ ultima_evaluacion: null })),
      lista: () => json([]),
      evaluar: () => json({ error: "la evaluacion fallo" }, 500),
    });

    montar();

    fireEvent.click(
      await screen.findByRole("button", { name: "Evaluar periodo" }),
    );

    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toBe("la evaluacion fallo"),
    );
  });

  it("un 503 del resumen es una caida, no 'Sin datos todavía'", async () => {
    servir({
      resumen: () => json({ error: "servicio no disponible" }, 503),
      lista: () => json({ error: "servicio no disponible" }, 503),
    });

    montar();

    await waitFor(() =>
      expect(screen.getAllByRole("alert").length).toBeGreaterThan(0),
    );
    expect(screen.queryByText("Sin datos todavía.")).toBeNull();
  });

  it("un 502 de /procesos sin ?periodo es una caida, no 'Sin datos todavía'", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      if (String(input) === "/api/auth/session")
        return json(sesion("administrador"));
      return json({ error: "servicio no disponible" }, 502);
    });

    montar("/anomalias");

    await waitFor(() =>
      expect(screen.getAllByRole("alert").length).toBeGreaterThan(0),
    );
    expect(screen.queryByText("Sin datos todavía.")).toBeNull();
  });

  it("avisa cuando la lista es una pagina de un total mayor", async () => {
    const cien = Array.from({ length: 100 }, (_, i) =>
      alerta({ id: `al-${i}`, tipo: "oni", detalle: `ONI ${i}` }),
    );
    const resumen: ResumenDeAlertas = resumenDePrueba({
      abiertas: 250,
      criticas_abiertas: 150,
      porTipo: { oni: 100, duplicado_registro: 150 },
    });
    servir({ resumen: () => json(resumen), lista: () => json(cien) });

    montar();

    await screen.findByText(/Mostrando 100 de 250 alertas abiertas/);
  });
});
