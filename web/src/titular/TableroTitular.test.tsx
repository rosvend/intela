import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Inicio from "../Inicio";
import { setToken } from "../api";
import { ProveedorDeSesion } from "../sesion";

function json(cuerpo: unknown, status = 200) {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

const ana = {
  id: "usr-ana",
  email: "ana@redes.co",
  nombre: "Ana Escritora",
  rol: "titular",
  titular_id: "tit-ana",
};

const RESPUESTAS: Record<string, unknown> = {
  "/api/auth/session": ana,
  "/api/tablero/ultima-liquidacion": {
    periodo: "2025-01",
    neto: "2450000.00",
    obras: 2,
  },
  "/api/tablero/mis-obras": {
    obras: [
      {
        id: "obra-serie",
        titulo: "La Casa de las Dos Palmas",
        estado: "completa",
      },
      {
        id: "obra-sketch",
        titulo: "Sketch de Medianoche",
        estado: "incompleta",
      },
    ],
  },
  "/api/mis-liquidaciones/obras": {
    titular_id: "tit-ana",
    periodo: "",
    lineas: [
      {
        periodo: "2025-01",
        obra_id: "obra-serie",
        titulo: "La Casa de las Dos Palmas",
        bruto: "2000000.00",
        admin: "0",
        social: "0",
        reserva: "0",
        neto: "1500000.00",
      },
      {
        periodo: "2024-12",
        obra_id: "obra-serie",
        titulo: "La Casa de las Dos Palmas",
        bruto: "600000.00",
        admin: "0",
        social: "0",
        reserva: "0",
        neto: "450000.00",
      },
      {
        periodo: "2025-01",
        obra_id: "obra-cine",
        titulo: "El Último Plano",
        bruto: "800000.00",
        admin: "0",
        social: "0",
        reserva: "0",
        neto: "500000.00",
      },
    ],
    totales: {
      bruto: "3400000.00",
      admin: "0",
      social: "0",
      reserva: "0",
      neto: "2450000.00",
    },
  },
  "/api/mis-ingresos": { ingresos: [] },
};

function conRespuestas(sobre: Record<string, unknown> = {}) {
  const tabla = { ...RESPUESTAS, ...sobre };
  vi.mocked(fetch).mockImplementation(async (input) => {
    const url = String(input);
    if (url in tabla) {
      const r = tabla[url];
      return r instanceof Response ? r : json(r);
    }
    return json({ error: "ruta no encontrada" }, 404);
  });
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

function panel(titulo: string): HTMLElement {
  const h = screen.getByRole("heading", { name: titulo });
  const el = h.closest("article");
  if (!el) throw new Error(`sin panel "${titulo}"`);
  return el;
}

describe("TableroTitular", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    setToken("tok");
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("saluda por el primer nombre", async () => {
    conRespuestas();
    montar();
    expect(
      await screen.findByRole("heading", { name: "Hola, Ana" }),
    ).toBeTruthy();
  });

  it("el heroe muestra el neto exacto, el periodo en palabras y las obras", async () => {
    conRespuestas();
    montar();
    expect(await screen.findByLabelText("$ 2.450.000")).toBeTruthy();
    const heroe = panel("Tu última liquidación");
    expect(heroe.textContent).toContain("enero 2025");
    expect(heroe.textContent).toContain("2 obras");
  });

  it("ingresos por obra: total, una barra por obra que suma el total, y leyenda", async () => {
    conRespuestas();
    montar();
    const barra = await screen.findByRole("group", {
      name: "Ingresos por obra",
    });
    const tarjeta = panel("Ingresos por obra");
    expect(tarjeta.textContent).toContain("$ 2.450.000");
    const segmentos = within(barra).getAllByRole("button");
    expect(segmentos).toHaveLength(2);
    expect(segmentos[0].getAttribute("aria-label")).toContain(
      "La Casa de las Dos Palmas: $ 1.950.000",
    );
    const leyenda = within(tarjeta).getByRole("list");
    expect(leyenda.textContent).toContain("El Último Plano");
    expect(leyenda.textContent).toContain("$ 500.000");

    fireEvent.click(segmentos[1]);
    const dialogo = screen.getByRole("dialog", { name: "El Último Plano" });
    expect(dialogo.textContent).toContain("$ 500.000");
    expect(dialogo.textContent).toMatch(/20,41 %/);
    expect(
      within(tarjeta).getByRole("link", { name: /Ver mis liquidaciones/ }),
    ).toBeTruthy();
  });

  it("mis obras: lista para pagar o en reserva, con el porque en lenguaje llano", async () => {
    conRespuestas();
    montar();
    await screen.findByText("Sketch de Medianoche");
    const obras = panel("Mis obras");
    expect(within(obras).getByText("Lista para pagar")).toBeTruthy();
    fireEvent.click(
      within(obras).getByRole("button", {
        name: "En reserva: falta completar la declaración",
      }),
    );
    const dialogo = screen.getByRole("dialog");
    expect(dialogo.textContent).toContain("100 %");
    expect(dialogo.textContent).toContain("RD 13.1.3");
  });

  it("sin backend todavia, cada tarjeta muestra un vacio amable y no una alerta", async () => {
    conRespuestas({
      "/api/tablero/ultima-liquidacion": json({ error: "x" }, 404),
      "/api/tablero/mis-obras": json({ error: "x" }, 404),
      "/api/mis-liquidaciones/obras": json({ error: "x" }, 404),
    });
    montar();
    expect(
      await screen.findByText("Tu primera liquidación aparecerá aquí"),
    ).toBeTruthy();
    expect(screen.getByText("Aún no hay obras a tu nombre")).toBeTruthy();
    expect(screen.getByText("Aún no hay ingresos por obra")).toBeTruthy();
    expect(screen.queryByText("Sin datos todavía")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("un 500 en una tarjeta se anuncia sin tumbar las demas", async () => {
    conRespuestas({
      "/api/tablero/mis-obras": json({ error: "la base esta caida" }, 500),
    });
    montar();
    expect((await screen.findByRole("alert")).textContent).toContain(
      "la base esta caida",
    );
    expect(await screen.findByLabelText("$ 2.450.000")).toBeTruthy();
  });

  it("mientras carga pinta esqueletos", async () => {
    vi.mocked(fetch).mockImplementation(async (input) =>
      String(input) === "/api/auth/session"
        ? json(ana)
        : new Promise<Response>(() => {}),
    );
    montar();
    await screen.findByRole("heading", { name: "Hola, Ana" });
    expect(
      screen.getAllByRole("status", { name: "Cargando" }).length,
    ).toBeGreaterThan(0);
  });
});
