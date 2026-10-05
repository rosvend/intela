import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter, Outlet, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import BandejaIdentificacion from "./BandejaIdentificacion";
import type { ContextoIdentificacion } from "./pendientes";
import type { CasoIdentificacion } from "./tipos";

// Los 409 de la resolucion vistos desde la pantalla, de punta a punta: la
// clasificacion vive en resolucion.ts (clasificarError), esto prueba lo que la
// persona ve y si el caso vuelve o no a la cola.

function json(cuerpo: unknown, status = 200): Response {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

const casoUno = {
  id: "caso-1",
  titulo: "La Niña T3 E12",
  titulo_original: "La niña — Capítulo 12",
  fuente: "Caracol Televisión",
  modalidad: "tv",
  reporte_id: "ING-2024-0890",
  periodo: "2024-11",
  ids_fuente: "ID_Ficha=48213",
  evidencia: "Coincidencia parcial por título.",
  estado: "pendiente",
  candidatos: [
    {
      obra_id: "obra-1",
      titulo: "La Niña",
      anio: 2016,
      genero: "Drama",
      puntaje: 0.71,
      titulo_consultado: "La Niña T3 E12",
    },
  ],
  obra_asignada: null,
  resuelto_por: null,
  resuelto_en: null,
  ultima_actualizacion: "2024-11-05T10:00:00Z",
  nota: null,
  sugerencia: {
    decision: "asignar",
    obra_id: "obra-1",
    titulo: "La Niña",
    confianza: 0.5,
    motivo: "sin resoluciones anteriores de este titulo",
    orden: ["obra-1"],
    aceptada: null,
    sello: "sello-mostrado",
  },
} satisfies CasoIdentificacion;

const casoDos = {
  ...casoUno,
  id: "caso-2",
  titulo: "Otra Novela T1 E01",
  titulo_original: "",
  candidatos: [],
} satisfies CasoIdentificacion;

/** Cada GET de la cola devuelve la siguiente pagina; la ultima se repite. */
function instalarServidor(
  paginas: CasoIdentificacion[][],
  conflicto: string,
): { lecturas: () => number } {
  let lecturas = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (entrada: RequestInfo | URL, init?: RequestInit) => {
      const url = String(entrada);
      const metodo = init?.method ?? "GET";
      if (metodo === "GET" && url.startsWith("/api/identificacion/casos")) {
        const casos = paginas[Math.min(lecturas, paginas.length - 1)];
        lecturas++;
        return json({ pendientes: casos.length, casos });
      }
      if (metodo === "POST" && url.endsWith("/resolucion")) {
        return json({ error: conflicto }, 409);
      }
      throw new Error(`peticion no manejada en el test: ${metodo} ${url}`);
    }),
  );
  return { lecturas: () => lecturas };
}

function montar() {
  const contexto: ContextoIdentificacion = {
    pendientes: undefined,
    fijarPendientes: () => {},
  };
  return render(
    <MemoryRouter initialEntries={["/identificacion"]}>
      <Routes>
        <Route element={<Outlet context={contexto} />}>
          <Route path="/identificacion" element={<BandejaIdentificacion />} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

function enFoco(): string | null {
  return screen.queryByRole("heading", { level: 2 })?.textContent ?? null;
}

function cola(): string[] {
  const nav = screen.queryByRole("navigation", { name: "Cola de casos" });
  if (!nav) return [];
  return within(nav)
    .queryAllByRole("button")
    .map((b) => b.querySelector(".cola-titulo")?.textContent ?? "");
}

function unirConNota(nota: string) {
  fireEvent.click(screen.getByRole("radio", { name: /La Niña/ }));
  fireEvent.change(screen.getByLabelText("Nota para la bitácora"), {
    target: { value: nota },
  });
  fireEvent.click(screen.getByRole("button", { name: "Unir con esta obra" }));
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("VistaFusion en la bandeja: los 409 de la resolucion", () => {
  it('"ya resuelto": el caso sale de la cola y "Recargar caso" trae la cola de nuevo', async () => {
    const servidor = instalarServidor(
      [[casoUno, casoDos], [casoDos]],
      'otra persona ya resolvio este caso: el caso esta en "manual"',
    );
    montar();
    await waitFor(() => expect(enFoco()).toBe("La Niña T3 E12"));

    unirConNota("mi nota");

    const aviso = await screen.findByRole("alert");
    expect(
      within(aviso).getByText("Otra persona resolvió este caso antes"),
    ).not.toBeNull();
    expect(cola()).toEqual(["Otra Novela T1 E01"]);

    fireEvent.click(
      within(aviso).getByRole("button", { name: "Recargar caso" }),
    );

    await waitFor(() => expect(servidor.lecturas()).toBe(2));
    await waitFor(() => expect(enFoco()).toBe("Otra Novela T1 E01"));
    expect(
      screen.queryByText("Otra persona resolvió este caso antes"),
    ).toBeNull();
  });

  it('"no pendiente" tambien saca el caso y ofrece "Recargar caso"', async () => {
    instalarServidor([[casoUno, casoDos]], "el caso ya no esta pendiente");
    montar();
    await waitFor(() => expect(enFoco()).toBe("La Niña T3 E12"));

    unirConNota("mi nota");

    expect(
      await screen.findByText("Este caso ya no está pendiente"),
    ).not.toBeNull();
    expect(
      screen.getByRole("button", { name: "Recargar caso" }),
    ).not.toBeNull();
    expect(cola()).toEqual(["Otra Novela T1 E01"]);
  });

  it("alias en conflicto: el caso vuelve a la cola y al conteo, con la nota intacta", async () => {
    instalarServidor(
      [[casoUno]],
      'resolver el uso "x": ese identificador ya apunta a otra obra: id_ficha=48213 ya apunta a "obra-40"',
    );
    montar();
    await waitFor(() => expect(enFoco()).toBe("La Niña T3 E12"));
    expect(screen.getByText("1 por revisar")).not.toBeNull();

    unirConNota("mi nota");

    expect(
      await screen.findByText("Ese identificador ya apunta a otra obra"),
    ).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Recargar caso" })).toBeNull();
    expect(cola()).toEqual(["La Niña T3 E12"]);
    expect(screen.getByText("1 por revisar")).not.toBeNull();
    expect(
      (screen.getByLabelText("Nota para la bitácora") as HTMLInputElement)
        .value,
    ).toBe("mi nota");
    expect(
      screen.getByRole("button", { name: "Unir con esta obra" }),
    ).toHaveProperty("disabled", false);
  });
});
