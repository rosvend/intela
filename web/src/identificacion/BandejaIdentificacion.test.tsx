import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter, Outlet, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Obra } from "../catalogo/tipos";
import { DEBOUNCE_TECLEO_MS } from "../useValorDiferido";
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
  },
} satisfies CasoIdentificacion;

const obraDeBusqueda = {
  id: "obra-9",
  titulo: "Otra Novela",
  genero: "Drama",
  anio: 2020,
  tipo: "serie",
  coautores: [],
  estado_declaracion: "completa",
  suma_porcentajes: 100,
  version_vigente: 1,
} satisfies Obra;

type Resolutor = (id: string, cuerpo: unknown) => Response | Promise<Response>;

/** Un servidor falso minimo: la cola de casos, la resolucion y la busqueda. */
function instalarServidor(opciones: {
  casos: CasoIdentificacion[];
  pendientes?: number;
  resolver?: Resolutor;
  obras?: Obra[];
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

      if (metodo === "GET" && url.startsWith("/api/obras")) {
        return json(opciones.obras ?? []);
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

/** El titulo del caso es un <h2>: unico en la tarjeta, no en el panel (que lo pinta como <p>). */
function tituloDeLaTarjeta() {
  return screen.queryByRole("heading", { level: 2, name: "La Niña T3 E12" });
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
  it("muestra la tarjeta expandida con las candidatas y sus puntajes, sin preseleccion", async () => {
    instalarServidor({ casos: [casoUno] });
    montar();

    expect(await screen.findByText("Bandeja de identificación")).not.toBeNull();
    expect(tituloDeLaTarjeta()).not.toBeNull();

    const cabecera = screen.getByRole("button", {
      name: /La Niña T3 E12/,
    });
    expect(cabecera.getAttribute("aria-expanded")).toBe("true");

    expect(screen.getByText("0,71")).not.toBeNull();
    expect(screen.getByText("0,43")).not.toBeNull();
    expect(screen.getByText("2 coincidencias")).not.toBeNull();
    expect(screen.getByText("Sugerencia: asignar a La Niña.")).not.toBeNull();
    expect(
      screen.getByText("Confírmala o elige otra. Nada se asigna solo."),
    ).not.toBeNull();

    // ADR 0007: ninguna candidata viene marcada o resaltada por defecto. Las
    // dos ofrecen exactamente el mismo boton, sin distincion visual de
    // "recomendada".
    const botones = screen.getAllByRole("button", {
      name: /^Asignar a esta obra:/,
    });
    expect(botones).toHaveLength(2);
    expect(
      screen.getByRole("button", { name: "Asignar a esta obra: La Niña" }),
    ).not.toBeNull();
    expect(
      screen.getByRole("button", { name: "Asignar a esta obra: La Promesa" }),
    ).not.toBeNull();
  });

  it("asignar desde una candidata: POST a la ruta del caso con decision=asignar, su obra_id y la nota recortada", async () => {
    let capturado: { id: string; cuerpo: unknown } | null = null;
    instalarServidor({
      casos: [casoUno],
      resolver: (id, cuerpo) => {
        capturado = { id, cuerpo };
        return json({});
      },
    });
    montar();

    fireEvent.click(
      await screen.findByRole("button", {
        name: "Asignar a esta obra: La Niña",
      }),
    );
    const dialogo = await screen.findByRole("dialog");
    fireEvent.change(within(dialogo).getByLabelText("Nota *"), {
      target: { value: "  coincide la ficha tecnica  " },
    });
    fireEvent.click(
      within(dialogo).getByRole("button", { name: "Asignar a La Niña" }),
    );

    await waitFor(() => expect(capturado).not.toBeNull());
    expect(capturado).toEqual({
      id: "caso-1",
      cuerpo: {
        decision: "asignar",
        obra_id: "obra-1",
        nota: "coincide la ficha tecnica",
      },
    });
  });

  it("buscar otra obra: espera el debounce, pide /api/obras?titulo=&limite=5 y al elegir hace POST con ese obra_id", async () => {
    let capturado: { id: string; cuerpo: unknown } | null = null;
    instalarServidor({
      casos: [casoUno],
      obras: [obraDeBusqueda],
      resolver: (id, cuerpo) => {
        capturado = { id, cuerpo };
        return json({});
      },
    });
    montar();

    fireEvent.click(
      await screen.findByRole("button", { name: "Buscar otra obra" }),
    );
    const dialogo = await screen.findByRole("dialog");
    const campo = within(dialogo).getByLabelText("Título de la obra");

    // Los temporizadores falsos SOLO alrededor del tecleo y su avance: un
    // `findBy`/`waitFor` de testing-library con temporizadores falsos activos
    // encolaria su sondeo en el mismo reloj congelado y colgaria el test.
    vi.useFakeTimers();
    fireEvent.change(campo, { target: { value: "Otra" } });
    // Nada llega mientras el debounce no vence.
    expect(within(dialogo).queryByText("Otra Novela")).toBeNull();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(DEBOUNCE_TECLEO_MS);
    });
    vi.useRealTimers();

    const peticionesDeObras = vi
      .mocked(fetch)
      .mock.calls.map(([entrada]) => String(entrada))
      .filter((url) => url.startsWith("/api/obras"));
    expect(peticionesDeObras).toContain("/api/obras?titulo=Otra&limite=5");

    fireEvent.click(
      await within(dialogo).findByRole("button", { name: /Otra Novela/ }),
    );
    fireEvent.change(within(dialogo).getByLabelText("Nota *"), {
      target: { value: "obra correcta del catálogo" },
    });
    fireEvent.click(
      within(dialogo).getByRole("button", { name: "Asignar a Otra Novela" }),
    );

    await waitFor(() => expect(capturado).not.toBeNull());
    expect(capturado).toEqual({
      id: "caso-1",
      cuerpo: {
        decision: "asignar",
        obra_id: "obra-9",
        nota: "obra correcta del catálogo",
      },
    });
  });

  it("descartar: POST con decision=descartar, sin obra_id, y la nota recortada", async () => {
    let capturado: { id: string; cuerpo: unknown } | null = null;
    instalarServidor({
      casos: [casoUno],
      resolver: (id, cuerpo) => {
        capturado = { id, cuerpo };
        return json({});
      },
    });
    montar();

    fireEvent.click(
      await screen.findByRole("button", { name: "Descartar registro" }),
    );
    const dialogo = await screen.findByRole("dialog");
    fireEvent.change(within(dialogo).getByLabelText("Nota *"), {
      target: { value: " no es del repertorio " },
    });
    fireEvent.click(
      within(dialogo).getByRole("button", { name: "Descartar registro" }),
    );

    await waitFor(() => expect(capturado).not.toBeNull());
    expect(capturado).toEqual({
      id: "caso-1",
      cuerpo: { decision: "descartar", nota: "no es del repertorio" },
    });
  });

  it("remocion optimista: el caso sale de la lista en cuanto se envia, sin esperar la respuesta", async () => {
    let liberar: (respuesta: Response) => void = () => {};
    const enVuelo = new Promise<Response>((resolver) => {
      liberar = resolver;
    });
    instalarServidor({ casos: [casoUno], resolver: () => enVuelo });
    montar();

    fireEvent.click(
      await screen.findByRole("button", { name: "Descartar registro" }),
    );
    const dialogo = await screen.findByRole("dialog");
    fireEvent.change(within(dialogo).getByLabelText("Nota *"), {
      target: { value: "en revision" },
    });
    fireEvent.click(
      within(dialogo).getByRole("button", { name: "Descartar registro" }),
    );

    // La tarjeta de fondo ya no esta, aunque la red no ha contestado: la
    // unica mencion del titulo que queda es la del panel abierto.
    await waitFor(() => expect(tituloDeLaTarjeta()).toBeNull());
    expect(
      within(dialogo).getByRole("button", { name: "Guardando…" }),
    ).not.toBeNull();

    liberar(json({}));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(
      await screen.findByText("Registro “La Niña T3 E12” descartado"),
    ).not.toBeNull();
  });

  it("si falla por un error de red, el caso vuelve a la lista y el panel conserva la nota", async () => {
    instalarServidor({
      casos: [casoUno],
      resolver: () => {
        throw new TypeError("Failed to fetch");
      },
    });
    montar();

    fireEvent.click(
      await screen.findByRole("button", { name: "Descartar registro" }),
    );
    const dialogo = await screen.findByRole("dialog");
    fireEvent.change(within(dialogo).getByLabelText("Nota *"), {
      target: { value: "nota de prueba" },
    });
    fireEvent.click(
      within(dialogo).getByRole("button", { name: "Descartar registro" }),
    );

    await within(dialogo).findByText(
      "No pudimos guardar la resolución. Revisa tu conexión e inténtalo de nuevo; tu nota sigue aquí.",
    );

    expect(tituloDeLaTarjeta()).not.toBeNull();
    expect(
      (within(dialogo).getByLabelText("Nota *") as HTMLTextAreaElement).value,
    ).toBe("nota de prueba");
  });

  it('409 "ya resuelto": el panel ofrece "Recargar caso" y el caso NO vuelve a la lista', async () => {
    instalarServidor({
      casos: [casoUno],
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

    fireEvent.click(
      await screen.findByRole("button", { name: "Descartar registro" }),
    );
    const dialogo = await screen.findByRole("dialog");
    fireEvent.change(within(dialogo).getByLabelText("Nota *"), {
      target: { value: "x" },
    });
    fireEvent.click(
      within(dialogo).getByRole("button", { name: "Descartar registro" }),
    );

    expect(
      await within(dialogo).findByText("Otra persona resolvió este caso antes"),
    ).not.toBeNull();
    expect(
      within(dialogo).getByRole("button", { name: "Recargar caso" }),
    ).not.toBeNull();
    expect(tituloDeLaTarjeta()).toBeNull();
  });

  it('409 "no pendiente": el panel ofrece "Recargar caso" y el caso NO vuelve a la lista', async () => {
    instalarServidor({
      casos: [casoUno],
      resolver: () =>
        json(
          { error: 'el caso ya no esta pendiente: el caso esta en "difuso"' },
          409,
        ),
    });
    montar();

    fireEvent.click(
      await screen.findByRole("button", { name: "Descartar registro" }),
    );
    const dialogo = await screen.findByRole("dialog");
    fireEvent.change(within(dialogo).getByLabelText("Nota *"), {
      target: { value: "x" },
    });
    fireEvent.click(
      within(dialogo).getByRole("button", { name: "Descartar registro" }),
    );

    expect(
      await within(dialogo).findByText("Este caso ya no está pendiente"),
    ).not.toBeNull();
    expect(
      within(dialogo).getByRole("button", { name: "Recargar caso" }),
    ).not.toBeNull();
    expect(tituloDeLaTarjeta()).toBeNull();
  });

  it('409 de alias: el panel explica el conflicto SIN "Recargar caso", y el caso SI vuelve a la lista', async () => {
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

    fireEvent.click(
      await screen.findByRole("button", { name: "Descartar registro" }),
    );
    const dialogo = await screen.findByRole("dialog");
    fireEvent.change(within(dialogo).getByLabelText("Nota *"), {
      target: { value: "x" },
    });
    fireEvent.click(
      within(dialogo).getByRole("button", { name: "Descartar registro" }),
    );

    expect(
      await within(dialogo).findByText(
        "Ese identificador ya apunta a otra obra",
      ),
    ).not.toBeNull();
    expect(
      within(dialogo).queryByRole("button", { name: "Recargar caso" }),
    ).toBeNull();
    expect(tituloDeLaTarjeta()).not.toBeNull();
  });

  it("un 409 sin causa reconocida: el panel dice el mensaje del servidor, ofrece recargar y el caso SI vuelve a la lista", async () => {
    // El cuarto desenlace del 409 (plano seccion 5), el que no se puede
    // confundir con "ya resuelto": el servidor no dijo que el caso este fuera
    // de la cola, asi que sacarlo de la lista afirmaria algo que nadie dijo.
    instalarServidor({
      casos: [casoUno],
      resolver: () =>
        json({ error: "el uso tiene una marca de auditoria inesperada" }, 409),
    });
    montar();

    fireEvent.click(
      await screen.findByRole("button", { name: "Descartar registro" }),
    );
    const dialogo = await screen.findByRole("dialog");
    fireEvent.change(within(dialogo).getByLabelText("Nota *"), {
      target: { value: "x" },
    });
    fireEvent.click(
      within(dialogo).getByRole("button", { name: "Descartar registro" }),
    );

    expect(
      await within(dialogo).findByText("El caso cambió mientras lo revisabas"),
    ).not.toBeNull();
    expect(
      within(dialogo).getByText(
        "el uso tiene una marca de auditoria inesperada",
      ),
    ).not.toBeNull();
    expect(
      within(dialogo).getByRole("button", { name: "Recargar caso" }),
    ).not.toBeNull();
    expect(tituloDeLaTarjeta()).not.toBeNull();
  });

  it("empuja el conteo de pendientes al shell (useFijarPendientes) al cargar y al remover un caso", async () => {
    const fijarPendientes = vi.fn();
    instalarServidor({ casos: [casoUno], pendientes: 3 });
    montar(fijarPendientes);

    await waitFor(() => expect(tituloDeLaTarjeta()).not.toBeNull());
    await waitFor(() => expect(fijarPendientes).toHaveBeenCalledWith(3));

    fireEvent.click(screen.getByRole("button", { name: "Descartar registro" }));
    const dialogo = await screen.findByRole("dialog");
    fireEvent.change(within(dialogo).getByLabelText("Nota *"), {
      target: { value: "x" },
    });
    fireEvent.click(
      within(dialogo).getByRole("button", { name: "Descartar registro" }),
    );

    // La remocion optimista baja el conteo ANTES de que conteste la red.
    await waitFor(() => expect(fijarPendientes).toHaveBeenCalledWith(2));
  });
});
