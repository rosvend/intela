import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import ListaOni from "./ListaOni";
import { sugerenciaNinguna, type CasoIdentificacion } from "./tipos";

function json(cuerpo: unknown, status = 200): Response {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

const casoPendiente = {
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
  candidatos: [],
  obra_asignada: null,
  resuelto_por: null,
  resuelto_en: null,
  ultima_actualizacion: "2024-11-05T10:00:00Z",
  nota: null,
  sugerencia: sugerenciaNinguna(),
} satisfies CasoIdentificacion;

const casoAsignado = {
  id: "caso-2",
  titulo: "Magazine de farándula",
  titulo_original: "",
  fuente: "RCN",
  modalidad: "tv",
  reporte_id: "ING-2024-0891",
  periodo: "2024-12",
  ids_fuente: "",
  evidencia: "",
  estado: "asignado",
  candidatos: [],
  obra_asignada: { id: "obra-12", titulo: "La Niña T3" },
  resuelto_por: { id: "usr-admin", nombre: "Ana Pérez" },
  resuelto_en: "2026-09-27T15:04:05Z",
  ultima_actualizacion: "2026-09-27T15:04:05Z",
  nota: "coincide la ficha técnica con la declaración",
  sugerencia: sugerenciaNinguna(),
} satisfies CasoIdentificacion;

const casoDescartado = {
  id: "caso-3",
  titulo: "Noticiero central",
  titulo_original: "",
  fuente: "RCN",
  modalidad: "tv",
  reporte_id: "ING-2024-0892",
  periodo: "2024-12",
  ids_fuente: "",
  evidencia: "",
  estado: "descartado",
  candidatos: [],
  obra_asignada: null,
  resuelto_por: { id: "usr-admin", nombre: "Ana Pérez" },
  resuelto_en: "2026-09-27T16:00:00Z",
  ultima_actualizacion: "2026-09-27T16:00:00Z",
  nota: "no es un uso del repertorio de REDES",
  sugerencia: sugerenciaNinguna(),
} satisfies CasoIdentificacion;

/** Un servidor falso que registra cada consulta a la cola de casos. */
function instalarServidor(paginas: {
  porDefecto: CasoIdentificacion[];
  porUrl?: Record<string, CasoIdentificacion[]>;
}) {
  const consultas: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (entrada: RequestInfo | URL) => {
      const url = String(entrada);
      if (!url.startsWith("/api/identificacion/casos")) {
        throw new Error(`peticion no manejada en el test: ${url}`);
      }
      consultas.push(url);
      const casos = paginas.porUrl?.[url] ?? paginas.porDefecto;
      return json({ pendientes: casos.length, casos });
    }),
  );
  return consultas;
}

function montar(ruta = "/lista-oni") {
  return render(
    <MemoryRouter initialEntries={[ruta]}>
      <Routes>
        <Route path="/lista-oni" element={<ListaOni />} />
        <Route path="/identificacion" element={<p>bandeja</p>} />
        <Route path="/catalogo/:id" element={<p>detalle de obra</p>} />
      </Routes>
    </MemoryRouter>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("ListaOni", () => {
  it("pinta el estado y el responsable de cada fila, con guion cuando no hay responsable", async () => {
    instalarServidor({ porDefecto: [casoPendiente, casoAsignado] });
    montar();

    const filaPendiente = (await screen.findByText("La Niña T3 E12")).closest(
      "tr",
    )!;
    expect(within(filaPendiente).getByText("Pendiente")).not.toBeNull();
    expect(within(filaPendiente).getByText("—")).not.toBeNull();

    const filaAsignada = screen
      .getByText("Magazine de farándula")
      .closest("tr")!;
    expect(within(filaAsignada).getByText("Asignada")).not.toBeNull();
    expect(within(filaAsignada).getByText("Ana Pérez")).not.toBeNull();
  });

  it("solo el caso pendiente ofrece 'Resolver', y enlaza a /identificacion", async () => {
    instalarServidor({ porDefecto: [casoPendiente, casoAsignado] });
    montar();

    await screen.findByText("La Niña T3 E12");
    const filaPendiente = screen.getByText("La Niña T3 E12").closest("tr")!;
    const filaAsignada = screen
      .getByText("Magazine de farándula")
      .closest("tr")!;

    const resolver = within(filaPendiente).getByRole("link", {
      name: "Resolver",
    });
    expect(resolver.getAttribute("href")).toBe("/identificacion");
    expect(
      within(filaAsignada).queryByRole("link", { name: "Resolver" }),
    ).toBeNull();

    fireEvent.click(resolver);
    expect(await screen.findByText("bandeja")).not.toBeNull();
  });

  it("cada filtro se refleja en la URL y en la consulta real al servidor", async () => {
    const consultas = instalarServidor({ porDefecto: [casoAsignado] });
    montar();
    await screen.findByText("Magazine de farándula");

    fireEvent.click(
      within(
        screen.getByRole("group", { name: "Filtrar por estado" }),
      ).getByRole("button", { name: /^Asignada/ }),
    );
    await screen.findByText("Magazine de farándula");
    expect(consultas.some((url) => url.includes("estado=asignado"))).toBe(true);

    fireEvent.change(screen.getByLabelText("Filtrar por fuente"), {
      target: { value: "RCN" },
    });
    await screen.findByText("Magazine de farándula");
    const ultima = consultas.at(-1)!;
    expect(ultima).toContain("estado=asignado");
    expect(ultima).toContain("fuente=RCN");

    // "Limpiar filtros" vacia la URL y vuelve a pedir sin ningun filtro.
    fireEvent.click(screen.getByRole("button", { name: "Limpiar filtros" }));
    await screen.findByText("Magazine de farándula");
    const sinFiltros = consultas.at(-1)!;
    expect(sinFiltros).not.toContain("estado=");
    expect(sinFiltros).not.toContain("fuente=");
  });

  it("acumula en el selector de fuente los valores vistos en las paginas cargadas, sin perder los anteriores", async () => {
    instalarServidor({
      porDefecto: [casoPendiente], // fuente: Caracol Televisión
      porUrl: {
        "/api/identificacion/casos?estado=asignado&limite=50": [
          casoAsignado, // fuente: RCN
        ],
      },
    });
    montar();
    await screen.findByText("La Niña T3 E12");

    const opcionesDeFuente = () =>
      within(screen.getByLabelText("Filtrar por fuente") as HTMLSelectElement)
        .getAllByRole("option")
        .map((o) => o.textContent);

    // La primera pagina (sin filtros) solo trajo "Caracol Televisión".
    expect(opcionesDeFuente()).toEqual([
      "Todas las fuentes",
      "Caracol Televisión",
    ]);

    // Cambiar el ESTADO -un enum siempre disponible, no una fuente- trae una
    // pagina con una fuente distinta (RCN). D4 acumula: no reemplaza la ya
    // vista, la suma.
    fireEvent.click(
      within(
        screen.getByRole("group", { name: "Filtrar por estado" }),
      ).getByRole("button", { name: /^Asignada/ }),
    );
    await screen.findByText("Magazine de farándula");

    expect(opcionesDeFuente()).toEqual([
      "Todas las fuentes",
      "Caracol Televisión",
      "RCN",
    ]);
  });

  it("el selector de fuente conserva la fuente seleccionada aunque la página actual no la traiga", async () => {
    instalarServidor({
      porDefecto: [casoPendiente],
      porUrl: { "/api/identificacion/casos?fuente=RCN&limite=50": [] },
    });
    montar("/lista-oni?fuente=RCN");

    // La consulta filtrada por "RCN" no trajo ningun caso -por eso no hay
    // ninguno del que leer esa fuente-, y aun asi el selector la sigue
    // ofreciendo: es la fuente ACTIVA, y D4 la cuenta aunque no este en la
    // pagina.
    await screen.findByText("No hay coincidencias");
    const selectorFuente = screen.getByLabelText(
      "Filtrar por fuente",
    ) as HTMLSelectElement;
    expect(selectorFuente.value).toBe("RCN");
    expect(
      within(selectorFuente)
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["Todas las fuentes", "RCN"]);
  });

  it("pagina con Paginador: la etiqueta es 'Registros' y pide el siguiente desplazamiento", async () => {
    const cincuentaCasos = Array.from({ length: 50 }, (_, i) => ({
      ...casoAsignado,
      id: `caso-pagina-${i}`,
      titulo: `Obra ${i}`,
    }));
    const consultas = instalarServidor({
      porDefecto: cincuentaCasos,
      porUrl: {
        "/api/identificacion/casos?limite=50&desplazamiento=50": [
          casoPendiente,
        ],
      },
    });
    montar();

    expect(await screen.findByText("Registros 1 a 50")).not.toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Página siguiente" }));

    await screen.findByText("La Niña T3 E12");
    expect(consultas.some((url) => url.includes("desplazamiento=50"))).toBe(
      true,
    );
    expect(screen.getByText("Registros 51 a 51")).not.toBeNull();
  });

  it("el historial de un registro pendiente muestra la cascada y la evidencia, sin peticion adicional", async () => {
    const consultas = instalarServidor({ porDefecto: [casoPendiente] });
    montar();

    fireEvent.click(
      await screen.findByRole("button", { name: "Ver historial" }),
    );
    const dialogo = await screen.findByRole("dialog");

    expect(
      within(dialogo).getByText("Registro enviado a revisión"),
    ).not.toBeNull();
    // Linea de tiempo: reportado -> en revision, con la evidencia en llano.
    const pasos = within(dialogo).getAllByRole("listitem");
    expect(pasos.map((p) => p.querySelector("strong")?.textContent)).toEqual([
      "Reportado",
      "En revisión",
    ]);
    expect(within(dialogo).getByText(casoPendiente.evidencia)).not.toBeNull();
    // Los ids, detras del detalle tecnico.
    expect(within(dialogo).queryByText("ID_Ficha=48213")).toBeNull();
    fireEvent.click(
      within(dialogo).getByRole("button", { name: "Detalle técnico" }),
    );
    expect(within(dialogo).getByText("ID_Ficha=48213")).not.toBeNull();

    // Ninguna peticion nueva: el modal se armo con lo que la fila ya traia.
    expect(consultas).toHaveLength(1);

    fireEvent.click(
      within(dialogo).getByRole("button", { name: "Cerrar historial" }),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("el historial de un registro asignado muestra la obra, el responsable, la nota y el enlace a la obra", async () => {
    instalarServidor({ porDefecto: [casoAsignado] });
    montar();

    fireEvent.click(
      await screen.findByRole("button", { name: "Ver historial" }),
    );
    const dialogo = await screen.findByRole("dialog");

    expect(
      within(dialogo).getByText("Registro asignado a “La Niña T3”"),
    ).not.toBeNull();
    expect(within(dialogo).getByText("Ana Pérez")).not.toBeNull();
    expect(
      within(dialogo).getByText("coincide la ficha técnica con la declaración"),
    ).not.toBeNull();

    const enlace = within(dialogo).getByRole("link", { name: "Ver la obra" });
    expect(enlace.getAttribute("href")).toBe("/catalogo/obra-12");

    fireEvent.click(enlace);
    expect(await screen.findByText("detalle de obra")).not.toBeNull();
  });

  it("el historial de un registro descartado muestra el responsable y la nota, sin enlace a ninguna obra", async () => {
    instalarServidor({ porDefecto: [casoDescartado] });
    montar();

    fireEvent.click(
      await screen.findByRole("button", { name: "Ver historial" }),
    );
    const dialogo = await screen.findByRole("dialog");

    expect(within(dialogo).getByText("Registro descartado")).not.toBeNull();
    expect(within(dialogo).getByText("Ana Pérez")).not.toBeNull();
    expect(
      within(dialogo).getByText("no es un uso del repertorio de REDES"),
    ).not.toBeNull();
    expect(
      within(dialogo).queryByRole("link", { name: "Ver la obra" }),
    ).toBeNull();
  });

  it("los chips de estado cuentan los registros, filtran y marcan el activo", async () => {
    const consultas = instalarServidor({
      porDefecto: [casoPendiente, casoAsignado, casoDescartado],
    });
    montar();
    await screen.findByText("La Niña T3 E12");

    const grupo = screen.getByRole("group", { name: "Filtrar por estado" });
    expect(
      within(grupo).getByRole("button", { name: "Pendiente: 1" }),
    ).not.toBeNull();
    expect(
      within(grupo).getByRole("button", { name: "Asignada: 1" }),
    ).not.toBeNull();
    const todos = within(grupo).getByRole("button", { name: "Todos: 3" });
    expect(todos.getAttribute("aria-pressed")).toBe("true");

    // La distribucion: un segmento por estado presente.
    const barra = screen.getByRole("group", {
      name: "Distribución por estado",
    });
    expect(within(barra).getAllByRole("button")).toHaveLength(3);

    fireEvent.click(within(grupo).getByRole("button", { name: /^Descartada/ }));
    await screen.findByText("Noticiero central");
    expect(consultas.at(-1)).toContain("estado=descartado");
    expect(
      within(grupo)
        .getByRole("button", { name: /^Descartada/ })
        .getAttribute("aria-pressed"),
    ).toBe("true");
  });

  it("sin filtros y sin registros: 'No hay registros ONI'", async () => {
    instalarServidor({ porDefecto: [] });
    montar();

    expect(await screen.findByText("No hay registros ONI")).not.toBeNull();
    expect(screen.queryByText("No hay coincidencias")).toBeNull();
  });

  it("con filtros activos y sin coincidencias: 'No hay coincidencias' y el texto de ajustar filtros", async () => {
    instalarServidor({
      porDefecto: [casoPendiente],
      porUrl: { "/api/identificacion/casos?fuente=RCN&limite=50": [] },
    });
    montar("/lista-oni?fuente=RCN");

    expect(await screen.findByText("No hay coincidencias")).not.toBeNull();
    expect(
      screen.getByText(
        "Ajusta los filtros para consultar otros registros ONI.",
      ),
    ).not.toBeNull();
  });
});
