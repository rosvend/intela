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
import type { CandidatoIdentificacion, CasoIdentificacion } from "./tipos";

function json(cuerpo: unknown, status = 200): Response {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

const candidataUno = {
  obra_id: "obra-1",
  titulo: "La Niña",
  anio: 2016,
  genero: "Drama",
  puntaje: 0.71,
  titulo_consultado: "La Niña T3 E12",
} satisfies CandidatoIdentificacion;

const candidataDos = {
  obra_id: "obra-2",
  titulo: "La Promesa",
  anio: 2019,
  genero: "Drama",
  puntaje: 0.43,
  titulo_consultado: "La niña — Capítulo 12",
} satisfies CandidatoIdentificacion;

const casoUno = {
  id: "caso-1",
  titulo: "La Niña T3 E12",
  titulo_original: "La niña — Capítulo 12",
  fuente: "Caracol Televisión",
  modalidad: "tv",
  reporte_id: "ING-2024-0890",
  periodo: "2024-11",
  ids_fuente: "ID_Ficha=48213\nID_Emision=991204",
  evidencia:
    "Coincidencia parcial por título y número de episodio. La temporada no existe como obra independiente en el catálogo.",
  estado: "pendiente",
  candidatos: [candidataUno, candidataDos],
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
    orden: ["obra-1", "obra-2"],
    aceptada: null,
    sello: "sello-mostrado",
  },
} satisfies CasoIdentificacion;

type Resolutor = (id: string, cuerpo: unknown) => Response | Promise<Response>;

/** Un servidor falso minimo: la cola de casos, la resolucion y la busqueda. */
function instalarServidor(opciones: {
  casos: CasoIdentificacion[];
  pendientes?: number;
  resolver?: Resolutor;
}) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (entrada: RequestInfo | URL, init?: RequestInit) => {
      const url = String(entrada);
      const metodo = init?.method ?? "GET";

      if (metodo === "GET" && url.startsWith("/api/identificacion/casos")) {
        return json({
          pendientes: opciones.pendientes ?? opciones.casos.length,
          casos: opciones.casos,
        });
      }

      const resolucion =
        /^\/api\/identificacion\/casos\/([^/]+)\/resolucion$/.exec(url);
      if (metodo === "POST" && resolucion) {
        const id = resolucion[1];
        const cuerpo: unknown = JSON.parse(String(init?.body));
        return opciones.resolver ? opciones.resolver(id, cuerpo) : json({});
      }

      throw new Error(`peticion no manejada en el test: ${metodo} ${url}`);
    }),
  );
}

/** Deja que `BandejaIdentificacion` empuje su conteo por `useOutletContext`. */
function EnvoltorioDelShell({
  fijarPendientes,
}: {
  fijarPendientes: (valor: number) => void;
}) {
  const contexto: ContextoIdentificacion = {
    pendientes: undefined,
    fijarPendientes,
  };
  return <Outlet context={contexto} />;
}

function montar(fijarPendientes: (valor: number) => void = () => {}) {
  return render(
    <MemoryRouter initialEntries={["/identificacion"]}>
      <Routes>
        <Route
          element={<EnvoltorioDelShell fijarPendientes={fijarPendientes} />}
        >
          <Route path="/identificacion" element={<BandejaIdentificacion />} />
          <Route path="/lista-oni" element={<p>lista oni</p>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

const casoDos = {
  ...casoUno,
  id: "caso-2",
  titulo: "Otra Novela T1 E01",
  titulo_original: "",
  candidatos: [],
} satisfies CasoIdentificacion;

/** El caso en foco: su titulo es el unico <h2> de la bandeja. */
function enFoco(): string | null {
  return screen.queryByRole("heading", { level: 2 })?.textContent ?? null;
}

/** Los titulos que la cola lateral lista, en orden. */
function cola(): string[] {
  const nav = screen.queryByRole("navigation", { name: "Cola de casos" });
  if (!nav) return [];
  return within(nav)
    .queryAllByRole("button")
    .map((b) => b.querySelector(".cola-titulo")?.textContent ?? "");
}

/** Elige la primera candidata, escribe la nota y pulsa la accion. */
function resolver(accion: "Unir con esta obra" | "No es ninguna", nota = "x") {
  if (accion === "Unir con esta obra") {
    fireEvent.click(screen.getByRole("radio", { name: /La Niña/ }));
  }
  fireEvent.change(screen.getByLabelText("Nota para la bitácora"), {
    target: { value: nota },
  });
  fireEvent.click(screen.getByRole("button", { name: accion }));
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  // Red de seguridad: si un test con `vi.useFakeTimers()` fallara ANTES de
  // volver a los reales, todos los `waitFor`/`findBy*` siguientes -que
  // sondean con `setTimeout`- se quedarian esperando un reloj congelado.
  vi.useRealTimers();
});

describe("BandejaIdentificacion", () => {
  it("dice en una frase corta que hacer y cuenta los pendientes", async () => {
    instalarServidor({ casos: [casoUno], pendientes: 3 });
    const { container } = montar();

    expect(
      await screen.findByRole("heading", {
        level: 1,
        name: "Bandeja de identificación",
      }),
    ).not.toBeNull();
    expect(
      screen.getByText("Une cada uso reportado con su obra, o descártalo."),
    ).not.toBeNull();
    expect(await screen.findByText("3 por revisar")).not.toBeNull();
    // `.revision` es el grid 11rem/1fr del <dl> de afiliacion en styles.css.
    expect(
      container
        .querySelector("section.bandeja")
        ?.classList.contains("revision"),
    ).toBe(false);
  });

  it("'Ver lista ONI' es un boton, no texto enlazado", async () => {
    instalarServidor({ casos: [casoUno] });
    montar();

    const enlace = await screen.findByRole("link", { name: "Ver lista ONI" });
    expect(enlace.getAttribute("href")).toBe("/lista-oni");
    expect(enlace.className).toContain("boton-secundario");
  });

  it("un caso a la vez: el primero en foco, la cola al lado y el progreso", async () => {
    instalarServidor({ casos: [casoUno, casoDos] });
    montar();

    await waitFor(() => expect(enFoco()).toBe("La Niña T3 E12"));
    expect(screen.getAllByRole("heading", { level: 2 })).toHaveLength(1);
    expect(screen.getByText("Caso 1 de 2")).not.toBeNull();
    expect(cola()).toEqual(["La Niña T3 E12", "Otra Novela T1 E01"]);
    expect(
      screen.getByRole("button", { name: /La Niña T3 E12/, current: true }),
    ).not.toBeNull();
    // ADR 0007: ninguna candidata llega elegida.
    expect(
      screen
        .getAllByRole("radio")
        .every((r) => !(r as HTMLInputElement).checked),
    ).toBe(true);
  });

  it("las flechas del teclado y los botones pasan de caso; dentro de un campo no", async () => {
    instalarServidor({ casos: [casoUno, casoDos] });
    montar();
    await waitFor(() => expect(enFoco()).toBe("La Niña T3 E12"));

    fireEvent.keyDown(window, { key: "ArrowRight" });
    expect(enFoco()).toBe("Otra Novela T1 E01");
    expect(screen.getByText("Caso 2 de 2")).not.toBeNull();
    // En el extremo no da la vuelta.
    fireEvent.keyDown(window, { key: "ArrowRight" });
    expect(enFoco()).toBe("Otra Novela T1 E01");

    fireEvent.click(screen.getByRole("button", { name: "Caso anterior" }));
    expect(enFoco()).toBe("La Niña T3 E12");
    expect(
      (
        screen.getByRole("button", {
          name: "Caso anterior",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);

    const nota = screen.getByLabelText("Nota para la bitácora");
    fireEvent.keyDown(nota, { key: "ArrowRight" });
    expect(enFoco()).toBe("La Niña T3 E12");

    fireEvent.click(screen.getByRole("button", { name: "Caso siguiente" }));
    expect(enFoco()).toBe("Otra Novela T1 E01");
  });

  it("elegir un caso de la cola lo pone en foco", async () => {
    instalarServidor({ casos: [casoUno, casoDos] });
    montar();
    await waitFor(() => expect(cola()).toHaveLength(2));

    fireEvent.click(screen.getByRole("button", { name: /Otra Novela T1 E01/ }));
    expect(enFoco()).toBe("Otra Novela T1 E01");
  });

  it("mientras carga pinta esqueletos con un estado accesible", () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise<Response>(() => {})),
    );
    montar();

    expect(
      screen.getByRole("status", { name: "Cargando los casos pendientes" }),
    ).not.toBeNull();
    expect(enFoco()).toBeNull();
  });

  it("sin casos pendientes muestra el estado vacio amable", async () => {
    instalarServidor({ casos: [] });
    montar();

    expect(
      await screen.findByText(
        "Todo en orden: no hay usos pendientes por identificar",
      ),
    ).not.toBeNull();
  });

  it("si la cola no llega legible lo dice", async () => {
    instalarServidor({ casos: [{ ...casoUno, estado: "raro" } as never] });
    montar();

    expect(
      await screen.findByText(
        "La bandeja no llegó como una lista de casos legibles.",
      ),
    ).not.toBeNull();
  });

  it("si la carga falla ofrece intentar de nuevo", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => json({ error: "caido" }, 500)),
    );
    montar();

    expect(
      await screen.findByRole("button", { name: "Intentar de nuevo" }),
    ).not.toBeNull();
  });

  it("unir: POST a la ruta del caso con la obra elegida, la nota recortada y el sello", async () => {
    let capturado: { id: string; cuerpo: unknown } | null = null;
    instalarServidor({
      casos: [casoUno],
      resolver: (id, cuerpo) => {
        capturado = { id, cuerpo };
        return json({});
      },
    });
    montar();
    await waitFor(() => expect(enFoco()).toBe("La Niña T3 E12"));

    resolver("Unir con esta obra", "  coincide la ficha tecnica  ");

    await waitFor(() => expect(capturado).not.toBeNull());
    expect(capturado).toEqual({
      id: "caso-1",
      cuerpo: {
        decision: "asignar",
        obra_id: "obra-1",
        nota: "coincide la ficha tecnica",
        sello: "sello-mostrado",
      },
    });
  });

  it("remocion optimista: sale de la cola y del conteo al enviar; al confirmar avisa y sube el siguiente", async () => {
    let liberar: (respuesta: Response) => void = () => {};
    const enVuelo = new Promise<Response>((r) => {
      liberar = r;
    });
    const fijarPendientes = vi.fn();
    instalarServidor({ casos: [casoUno, casoDos], resolver: () => enVuelo });
    montar(fijarPendientes);
    await waitFor(() => expect(fijarPendientes).toHaveBeenCalledWith(2));

    resolver("Unir con esta obra");

    // Antes de que conteste la red: fuera de la cola, el conteo baja, y el
    // caso sigue en escena mientras se guarda.
    await waitFor(() => expect(cola()).toEqual(["Otra Novela T1 E01"]));
    expect(fijarPendientes).toHaveBeenCalledWith(1);
    expect(enFoco()).toBe("La Niña T3 E12");
    expect(screen.getByRole("button", { name: "Guardando…" })).not.toBeNull();

    liberar(json({}));

    expect(
      await screen.findByText("Registro asignado a “La Niña”"),
    ).not.toBeNull();
    await waitFor(() => expect(enFoco()).toBe("Otra Novela T1 E01"));
  });

  it("resolver el ultimo deja el estado vacio con el apoyo de lo registrado", async () => {
    instalarServidor({ casos: [casoUno] });
    montar();
    await waitFor(() => expect(enFoco()).toBe("La Niña T3 E12"));

    resolver("No es ninguna");

    expect(
      await screen.findByText("Registro “La Niña T3 E12” descartado"),
    ).not.toBeNull();
    expect(
      await screen.findByText("Cada decisión quedó registrada con su nota."),
    ).not.toBeNull();
    expect(enFoco()).toBeNull();
  });

  it("si falla por un error de red, el caso vuelve a la cola y conserva la nota", async () => {
    instalarServidor({
      casos: [casoUno, casoDos],
      resolver: () => {
        throw new TypeError("Failed to fetch");
      },
    });
    montar();
    await waitFor(() => expect(enFoco()).toBe("La Niña T3 E12"));

    resolver("No es ninguna", "nota de prueba");

    expect(
      await screen.findByText(
        "No pudimos guardar la decisión. Revisa tu conexión e inténtalo de nuevo; tu nota sigue aquí.",
      ),
    ).not.toBeNull();
    expect(cola()).toEqual(["La Niña T3 E12", "Otra Novela T1 E01"]);
    expect(enFoco()).toBe("La Niña T3 E12");
    expect(
      (screen.getByLabelText("Nota para la bitácora") as HTMLInputElement)
        .value,
    ).toBe("nota de prueba");
  });

  it('409 "ya resuelto": ofrece "Recargar caso" y el caso NO vuelve a la cola', async () => {
    instalarServidor({
      casos: [casoUno, casoDos],
      resolver: () =>
        json(
          {
            error:
              'otra persona ya resolvio este caso: el caso esta en "manual"',
          },
          409,
        ),
    });
    montar();
    await waitFor(() => expect(enFoco()).toBe("La Niña T3 E12"));

    resolver("No es ninguna");

    expect(
      await screen.findByText("Otra persona resolvió este caso antes"),
    ).not.toBeNull();
    expect(
      screen.getByRole("button", { name: "Recargar caso" }),
    ).not.toBeNull();
    expect(cola()).toEqual(["Otra Novela T1 E01"]);
    // Sin posicion en la cola, el progreso no inventa una.
    expect(screen.queryByText(/^Caso \d+ de/)).toBeNull();
    fireEvent.keyDown(window, { key: "ArrowRight" });
    expect(enFoco()).toBe("Otra Novela T1 E01");
  });

  it('409 de alias: lo explica sin "Recargar caso", y el caso SI vuelve a la cola', async () => {
    instalarServidor({
      casos: [casoUno],
      resolver: () =>
        json(
          {
            error:
              'ese identificador ya apunta a otra obra: caracol id_ficha=48213 ya apunta a "obra-40"',
          },
          409,
        ),
    });
    montar();
    await waitFor(() => expect(enFoco()).toBe("La Niña T3 E12"));

    resolver("No es ninguna");

    expect(
      await screen.findByText("Ese identificador ya apunta a otra obra"),
    ).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Recargar caso" })).toBeNull();
    expect(cola()).toEqual(["La Niña T3 E12"]);
  });

  it("con mas pendientes que los cargados ofrece cargar los siguientes", async () => {
    instalarServidor({ casos: [casoUno], pendientes: 40 });
    montar();

    expect(
      await screen.findByRole("button", {
        name: "Cargar los siguientes casos",
      }),
    ).not.toBeNull();
    expect(screen.getByText("Caso 1 de 40")).not.toBeNull();
  });
});
