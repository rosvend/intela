import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
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
    detalle:
      'la cascada no reconocio "La reina del flow" (fuente "caracol", entrega "r-1"): queda en la cola manual',
    referencia: "uso:uso-9",
    ref_id: "uso-9",
  }),
  alerta({ id: "al-2", tipo: "oni", detalle: "Otra ONI", ref_id: "u-2" }),
  alerta({
    id: "al-3",
    tipo: "duplicado_archivo",
    detalle: "Mismo SHA-256",
    ref_tipo: "reporte",
    ref_id: "rep-1",
    critica: true,
  }),
];

const RESERVA_60 =
  'la declaracion vigente de la obra "obra-serie" no esta completa: lo declarado suma 60% en 1 parte(s) y R-04 exige 100 exactos';
const COAUTOR =
  'el coautor con IPI IPI-00000002 figura en el catalogo de la obra "obra-serie" y no tiene parte en la declaracion vigente';

const deSerieY: Alerta[] = [
  alerta({
    id: "s-1",
    tipo: "reserva_declaracion_incompleta",
    detalle: RESERVA_60,
    ref_tipo: "obra",
    ref_id: "obra-serie",
  }),
  alerta({
    id: "s-2",
    tipo: "titular_sin_porcentaje",
    detalle: COAUTOR,
    ref_tipo: "obra",
    ref_id: "obra-serie",
    ref_titular: "ipi:IPI-00000002",
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
  obras?: Record<string, string>;
};

function servir({
  rol = "administrador",
  resumen = () => json(resumen2025),
  lista = () => json(alertas),
  evaluar = () => new Response(null, { status: 204 }),
  obras = {},
}: Rutas = {}) {
  vi.mocked(fetch).mockImplementation(async (input, init) => {
    const path = String(input);
    if (path === "/api/auth/session") return json(sesion(rol));
    if (path === RUTAS_REPARTO.procesos) return json([proc2025, proc2024]);
    if (path === RUTAS_REPARTO.evaluarAlertas && init?.method === "POST")
      return evaluar();
    if (path === RUTAS_REPARTO.resumenAlertas("2025")) return resumen();
    if (path === RUTAS_REPARTO.alertas("2025")) return lista();
    const obra = /^\/api\/obras\/([^/]+)$/.exec(path)?.[1];
    if (obra && obras[decodeURIComponent(obra)])
      return json({ id: obra, titulo: obras[decodeURIComponent(obra)] });
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

  it("resume en chips cuantas bloquean y cuantas avisan, con la barra por tipo", async () => {
    servir();

    montar();

    await screen.findByText("La reina del flow");
    const totales = screen.getByRole("group", { name: "Totales del periodo" });
    expect(within(totales).getByText("1 bloquea")).toBeTruthy();
    expect(within(totales).getByText("2 avisos")).toBeTruthy();

    const barra = screen.getByRole("group", { name: "Alertas por tipo" });
    // Solo los tipos con abiertas tienen segmento.
    expect(within(barra).getAllByRole("button")).toHaveLength(2);

    const leyenda = screen.getByRole("list", { name: "Tipos de alerta" });
    expect(within(leyenda).getAllByRole("listitem")).toHaveLength(6);
    // Los tres tipos criticos llevan el candado, sin subtitulo.
    expect(
      within(leyenda).getAllByRole("img", { name: "Bloquea el reparto" }),
    ).toHaveLength(3);
    expect(screen.queryByText("Bloquea el reparto")).toBeNull();
    expect(screen.queryByText(/crítica abierta bloquea/)).toBeNull();
  });

  it("sin criticas el resumen dice que nada bloquea", async () => {
    servir({
      resumen: () =>
        json(resumenDePrueba({ abiertas: 2, porTipo: { oni: 2 } })),
    });

    montar();

    const totales = await screen.findByRole("group", {
      name: "Totales del periodo",
    });
    expect(within(totales).getByText("Nada bloquea")).toBeTruthy();
    expect(within(totales).getByText("2 avisos")).toBeTruthy();
  });

  it("cada tipo explica que significa y que hacer en un Detalle", async () => {
    servir();

    montar();

    await screen.findByText("La reina del flow");
    fireEvent.click(
      screen.getByRole("button", {
        name: "Qué significa: Declaración que no suma 100 %",
      }),
    );
    const detalle = screen.getByRole("dialog", {
      name: "Declaración que no suma 100 %",
    });
    expect(detalle.textContent).toContain("Qué hacer");
    expect(detalle.textContent).toContain("reserva");
  });

  it("agrupa por obra con su titulo real y un chip por problema", async () => {
    servir({ lista: () => json(deSerieY), obras: { "obra-serie": "Serie Y" } });

    montar();

    const titulo = await screen.findByRole("heading", { name: "Serie Y" });
    const tarjeta = titulo.closest("li") as HTMLElement;
    expect(within(tarjeta).getByText("Declaración al 60 %")).toBeTruthy();
    expect(within(tarjeta).getByText("Coautor sin porcentaje")).toBeTruthy();
    expect(
      within(tarjeta).getByRole("img", { name: "60 de 100 % declarado" }),
    ).toBeTruthy();
    expect(
      within(tarjeta).getByRole("img", {
        name: "IPI-00000002: porcentaje sin declarar",
      }),
    ).toBeTruthy();

    const lista = screen.getByRole("list", { name: "Registros afectados" });
    expect(lista.querySelectorAll(":scope > li")).toHaveLength(1);
  });

  it("la frase del servidor no se pinta: vive en el Detalle de cada problema", async () => {
    servir({ lista: () => json(deSerieY), obras: { "obra-serie": "Serie Y" } });

    montar();

    await screen.findByRole("heading", { name: "Serie Y" });
    expect(screen.queryByText(RESERVA_60)).toBeNull();
    expect(screen.queryByText("obra:obra-serie")).toBeNull();
    fireEvent.click(
      screen.getByRole("button", { name: "Detalle: Declaración al 60 %" }),
    );
    expect(screen.getByRole("dialog").textContent).toContain(RESERVA_60);
  });

  it("sin titulo en el catalogo la tarjeta usa el id de la obra", async () => {
    servir({ lista: () => json(deSerieY) });

    montar();

    expect(
      await screen.findByRole("heading", { name: "obra-serie" }),
    ).toBeTruthy();
  });

  it("la declaracion y el coautor llevan a abrir la declaracion de la obra", async () => {
    servir({ lista: () => json(deSerieY), obras: { "obra-serie": "Serie Y" } });

    montar();

    await screen.findByRole("heading", { name: "Serie Y" });
    const enlaces = screen.getAllByRole("link", { name: "Abrir declaración" });
    expect(enlaces).toHaveLength(1);
    expect(enlaces[0].getAttribute("href")).toBe(
      "/catalogo/obra-serie/declaracion",
    );
  });

  it("lo que bloquea va primero, con borde de acento y chip relleno con candado", async () => {
    servir();

    montar();

    const lista = await screen.findByRole("list", {
      name: "Registros afectados",
    });
    const tarjetas = Array.from(
      lista.querySelectorAll<HTMLElement>(":scope > li"),
    );
    expect(tarjetas).toHaveLength(3);
    expect(within(tarjetas[0]).getByRole("heading").textContent).toBe("rep-1");
    expect(tarjetas[0].className).toContain("afectado-bloquea");
    expect(tarjetas[1].className).not.toContain("afectado-bloquea");

    const chip = within(tarjetas[0]).getByText("Archivo duplicado");
    expect(chip.closest(".chip")?.className).toContain("problema-chip-bloquea");
    expect(
      within(tarjetas[0]).getByRole("img", { name: "Bloquea el reparto" }),
    ).toBeTruthy();
    const aviso = within(tarjetas[1]).getByText("Sin obra identificada");
    expect(aviso.closest(".chip")?.className).toContain("chip-alerta");
  });

  it("no hereda la clase .revision del <dl> de afiliacion, que lo parte en una columna de 11rem", async () => {
    servir();

    const { container } = montar();

    await screen.findByText("La reina del flow");
    const pantalla = container.querySelector("section.anomalias");
    expect(pantalla?.classList.contains("revision")).toBe(false);
    expect(pantalla?.classList.contains("revision-pantalla")).toBe(true);
  });

  it("no pone subtitulo de relleno ni la linea de corridas del periodo", async () => {
    servir();

    montar();

    await screen.findByText("La reina del flow");
    expect(screen.queryByText(/Lo que conviene revisar/)).toBeNull();
    expect(screen.queryByText(/Corridas del periodo/)).toBeNull();
  });

  it("el panel de corridas es una pildora secundaria en la cabecera", async () => {
    servir();

    montar();

    await screen.findByText("La reina del flow");
    const volver = screen.getByRole("link", { name: "Panel de corridas" });
    expect(volver.getAttribute("href")).toBe("/distribucion");
    expect(volver.className).toContain("boton-secundario");
    expect(volver.closest("header")).not.toBeNull();
  });

  it("una ONI enlaza a la bandeja de identificacion sin perder el periodo de la vista", async () => {
    servir();

    montar();

    await screen.findByText("La reina del flow");
    const enlaces = screen.getAllByRole("link", { name: "Identificar" });
    expect(enlaces).toHaveLength(2);
    expect(enlaces[0]?.getAttribute("href")).toBe("/identificacion");
    expect(
      screen
        .getByRole("link", { name: "Revisar en Ingesta" })
        .getAttribute("href"),
    ).toBe("/ingesta");
    expect((screen.getByLabelText("Periodo") as HTMLSelectElement).value).toBe(
      "2025",
    );
  });

  it("una alerta resuelta se ve apagada y sin accion", async () => {
    servir({
      lista: () =>
        json([
          alerta({
            id: "al-9",
            tipo: "oni",
            detalle: 'la cascada no reconocio "Ya cerrada" (fuente "x")',
            resuelta: true,
          }),
        ]),
    });

    montar();

    const texto = await screen.findByText("Ya cerrada");
    const tarjeta = texto.closest("li") as HTMLElement;
    expect(tarjeta.className).toContain("afectado-cerrado");
    expect(within(tarjeta).getByText("Resuelta")).toBeTruthy();
    expect(within(tarjeta).queryByRole("link")).toBeNull();
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

    await screen.findByText("La reina del flow");
    const pedidas = vi.mocked(fetch).mock.calls.map(([url]) => String(url));
    expect(pedidas).not.toContain(RUTAS_REPARTO.alertas());
    expect(pedidas).not.toContain(RUTAS_REPARTO.resumenAlertas(""));
  });

  it("sin backend de alertas muestra el vacio", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      if (String(input) === "/api/auth/session")
        return json(sesion("administrador"));
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar("/anomalias");

    await waitFor(() =>
      expect(screen.getAllByText("Sin datos todavía.").length).toBeGreaterThan(
        0,
      ),
    );
    const pedidas = vi.mocked(fetch).mock.calls.map(([url]) => String(url));
    expect(pedidas).not.toContain(RUTAS_REPARTO.alertas());
  });

  it("un periodo sin evaluar no se presenta como limpio", async () => {
    servir({
      resumen: () => json(resumenDePrueba({ ultima_evaluacion: null })),
      lista: () => json([]),
    });

    montar();

    await screen.findByText(/no se ha evaluado todavía: los conteos/);
    expect(screen.queryByText(/no hay alertas abiertas/i)).toBeNull();
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

    await screen.findByText("1 bloquea");
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
      lista: () => json([]),
    });

    montar();

    await waitFor(() =>
      expect(screen.getAllByRole("alert").length).toBeGreaterThan(0),
    );
    expect(screen.queryByText("Sin datos todavía.")).toBeNull();
    expect(screen.queryByText(/no hay alertas abiertas/i)).toBeNull();
  });

  it("no dice 'No hay alertas' mientras el resumen no ha llegado", async () => {
    let resolver: (r: Response) => void = () => {};
    servir({
      resumen: () => new Promise<Response>((r) => (resolver = r)) as never,
      lista: () => json([]),
    });

    montar();

    await waitFor(() =>
      expect(
        vi
          .mocked(fetch)
          .mock.calls.some(
            ([u]) => String(u) === RUTAS_REPARTO.alertas("2025"),
          ),
      ).toBe(true),
    );
    await new Promise((r) => setTimeout(r, 20));
    expect(screen.queryByText(/no hay alertas abiertas/i)).toBeNull();

    resolver(json(resumenDePrueba({ periodo: "2025" })));
    await screen.findByText(
      "Todo en orden: no hay alertas abiertas en este periodo",
    );
  });

  it("cuenta en singular una crítica aceptada sin corregir", async () => {
    servir({
      resumen: () => json(resumenDePrueba({ criticas_aceptadas: 1 })),
    });

    montar();

    await screen.findByText(/1 aceptada sin corregir/);
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
