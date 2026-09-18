import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ListaCargas, { type Carga } from "./ListaCargas";
import type { Rechazo } from "./TablaRechazos";

function json(cuerpo: unknown, status = 200): Response {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

/** El path de la llamada `n` a fetch (0 = la primera). */
function pedido(n: number): unknown {
  return vi.mocked(fetch).mock.calls[n]?.[0];
}

/** La fila del listado que contiene `texto`. */
function filaCon(texto: string): HTMLElement {
  const fila = screen.getByText(texto).closest("tr");
  if (!fila) throw new Error(`"${texto}" no esta dentro de una fila`);
  return fila;
}

// Fixtures tipados con el contrato generado: si `Carga` o `Rechazo` cambian
// en api/openapi.yaml, `tsc` rompe aqui antes que en pantalla. Datos
// sinteticos, no de ningun canal real.
const SHA_CARACOL =
  "9f2c4e1a7b3d5f60a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718";
const SHA_NETFLIX =
  "7a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f607189f2c4e1a7b3d5f6";

const cargaSinRechazos = {
  id: "rep-9f2c4e1a",
  fuente: "caracol",
  periodo: "2026-01",
  sha256: SHA_CARACOL,
  clave_objeto: "reportes/9f2c4e1a",
  nbytes: 21032,
  recibido: "2026-02-03T14:05:00Z",
  aceptados: 1234,
  rechazados: 0,
} satisfies Carga;

const cargaConRechazos = {
  id: "rep-7a1b2c3d",
  fuente: "netflix",
  periodo: "2026-01",
  sha256: SHA_NETFLIX,
  clave_objeto: "reportes/7a1b2c3d",
  nbytes: 4096,
  recibido: "2026-02-04T09:30:00Z",
  aceptados: 58,
  rechazados: 3,
} satisfies Carga;

const rechazos = [
  {
    id: "rep-7a1b2c3d-2",
    titulo: "Obra sintetica A",
    ids_fuente: "id_netflix=80000001",
    motivo: 'fila 2, titulo (columna "Title"): vacio',
  },
  {
    id: "rep-7a1b2c3d-5",
    titulo: "Obra sintetica B",
    ids_fuente: "id_netflix=80000002",
    motivo:
      'fila 5, horas_vistas (columna "Hours Viewed"): "muchas" no es un numero',
  },
  {
    id: "rep-7a1b2c3d-11",
    titulo: "Obra sintetica C",
    ids_fuente: "id_netflix=80000003",
    motivo: 'fila 11, titulo (columna "Title"): vacio',
  },
] satisfies Rechazo[];

describe("ListaCargas", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("pide las cargas del periodo en la query", async () => {
    vi.mocked(fetch).mockResolvedValue(json([]));

    render(<ListaCargas periodo="2026-01" />);

    await screen.findByText(
      "No hay cargas registradas para el periodo 2026-01.",
    );
    expect(pedido(0)).toBe("/api/reportes?periodo=2026-01");
  });

  it("codifica el periodo para que no parta la query", async () => {
    vi.mocked(fetch).mockResolvedValue(json([]));

    render(<ListaCargas periodo="2026-01&fuente=x" />);

    await screen.findByText(/No hay cargas registradas/);
    expect(pedido(0)).toBe("/api/reportes?periodo=2026-01%26fuente%3Dx");
  });

  it("sin periodo pide todas las cargas y lo dice si no hay ninguna", async () => {
    vi.mocked(fetch).mockResolvedValue(json([]));

    render(<ListaCargas periodo="" />);

    await screen.findByText("Aún no hay cargas registradas.");
    expect(pedido(0)).toBe("/api/reportes");
  });

  it("mientras llega la respuesta muestra que esta cargando", () => {
    vi.mocked(fetch).mockReturnValue(new Promise<Response>(() => {}));

    render(<ListaCargas periodo="2026-01" />);

    expect(screen.getByRole("status")).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("un error del backend se muestra con su mensaje", async () => {
    const mensaje =
      'reporte invalido: periodo "2026-1", se esperaba AAAA o AAAA-MM';
    vi.mocked(fetch).mockResolvedValue(json({ error: mensaje }, 400));

    render(<ListaCargas periodo="2026-1" />);

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain(mensaje);
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("cada carga muestra fuente, periodo, huella corta, recuentos y fecha", async () => {
    vi.mocked(fetch).mockResolvedValue(
      json([cargaSinRechazos, cargaConRechazos]),
    );

    render(<ListaCargas periodo="2026-01" />);

    const tabla = await screen.findByRole("table", { name: "Cargas hechas" });
    const encabezados = within(tabla)
      .getAllByRole("columnheader")
      .map((th) => th.textContent);
    expect(encabezados).toEqual([
      "Recibido",
      "Fuente",
      "Periodo",
      "Huella",
      "Aceptadas",
      "Rechazadas",
      "",
    ]);

    const fila = filaCon("caracol");
    const celdas = within(fila)
      .getAllByRole("cell")
      .map((td) => td.textContent);
    // La fecha depende de la zona horaria del runner: el esperado sale del
    // mismo formato, no de un string fijo.
    const recibido = new Intl.DateTimeFormat("es-CO", {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(cargaSinRechazos.recibido));
    expect(celdas).toEqual([
      recibido,
      "caracol",
      "2026-01",
      SHA_CARACOL.slice(0, 12),
      "1.234",
      "0",
      "",
    ]);
    // La huella completa queda a mano para volver a la evidencia.
    expect(within(fila).getByTitle(SHA_CARACOL).textContent).toBe(
      SHA_CARACOL.slice(0, 12),
    );
    // El instante exacto viaja en el atributo, sin depender de la zona.
    // (`getByText(recibido)` no sirve: es-CO mete espacios finos U+202F que
    // el normalizador colapsa en el DOM pero no en el string buscado.)
    expect(fila.querySelector("time")?.getAttribute("datetime")).toBe(
      cargaSinRechazos.recibido,
    );
  });

  it("una fecha que no se puede leer se muestra tal cual, sin romper el listado", async () => {
    vi.mocked(fetch).mockResolvedValue(
      json([{ ...cargaSinRechazos, recibido: "ayer" }]),
    );

    render(<ListaCargas periodo="2026-01" />);

    await screen.findByRole("table", { name: "Cargas hechas" });
    expect(within(filaCon("caracol")).getByText("ayer")).toBeTruthy();
  });

  it("solo las cargas con rechazos ofrecen verlos, y nada se pide antes de abrir", async () => {
    vi.mocked(fetch).mockResolvedValue(
      json([cargaSinRechazos, cargaConRechazos]),
    );

    render(<ListaCargas periodo="2026-01" />);

    await screen.findByRole("table", { name: "Cargas hechas" });
    expect(within(filaCon("caracol")).queryByRole("button")).toBeNull();

    const boton = within(filaCon("netflix")).getByRole("button", {
      name: "Ver rechazos (3)",
    });
    expect(boton.getAttribute("aria-expanded")).toBe("false");
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("al abrir pide el log de esa carga y lo pinta entero; al cerrar lo oculta", async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(json([cargaSinRechazos, cargaConRechazos]))
      .mockResolvedValueOnce(json(rechazos));

    render(<ListaCargas periodo="2026-01" />);

    await screen.findByRole("table", { name: "Cargas hechas" });
    fireEvent.click(screen.getByRole("button", { name: "Ver rechazos (3)" }));

    const log = await screen.findByRole("table", { name: "Filas rechazadas" });
    expect(pedido(1)).toBe("/api/reportes/rep-7a1b2c3d/rechazos");
    expect(fetch).toHaveBeenCalledTimes(2);
    // Invariante 3: todas las filas del log, ninguna recortada.
    expect(within(log).getAllByRole("row").slice(1)).toHaveLength(3);
    for (const rechazo of rechazos) {
      expect(within(log).getByText(rechazo.motivo)).toBeTruthy();
    }

    const boton = screen.getByRole("button", { name: "Ocultar rechazos" });
    expect(boton.getAttribute("aria-expanded")).toBe("true");
    // aria-controls apunta a la fila que contiene el log.
    const controlada = document.getElementById(
      boton.getAttribute("aria-controls") ?? "",
    );
    expect(controlada?.contains(log)).toBe(true);

    fireEvent.click(boton);

    expect(
      screen.queryByRole("table", { name: "Filas rechazadas" }),
    ).toBeNull();
    expect(
      screen
        .getByRole("button", { name: "Ver rechazos (3)" })
        .getAttribute("aria-expanded"),
    ).toBe("false");
  });

  it("varias cargas pueden tener su log abierto a la vez", async () => {
    const otra = {
      ...cargaConRechazos,
      id: "rep-5e6f7a8b",
      fuente: "cine",
      rechazados: 1,
    } satisfies Carga;
    vi.mocked(fetch)
      .mockResolvedValueOnce(json([cargaConRechazos, otra]))
      .mockResolvedValueOnce(json(rechazos))
      .mockResolvedValueOnce(json([rechazos[0]]));

    render(<ListaCargas periodo="2026-01" />);

    await screen.findByRole("table", { name: "Cargas hechas" });
    fireEvent.click(screen.getByRole("button", { name: "Ver rechazos (3)" }));
    await screen.findByRole("table", { name: "Filas rechazadas" });
    fireEvent.click(screen.getByRole("button", { name: "Ver rechazos (1)" }));

    await vi.waitFor(() =>
      expect(
        screen.getAllByRole("table", { name: "Filas rechazadas" }),
      ).toHaveLength(2),
    );
    expect(pedido(2)).toBe("/api/reportes/rep-5e6f7a8b/rechazos");
  });

  it("si la carga ya no existe, el error queda dentro de su fila", async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(json([cargaSinRechazos, cargaConRechazos]))
      .mockResolvedValueOnce(json({ error: "esa carga no existe" }, 404));

    render(<ListaCargas periodo="2026-01" />);

    await screen.findByRole("table", { name: "Cargas hechas" });
    const boton = screen.getByRole("button", { name: "Ver rechazos (3)" });
    fireEvent.click(boton);

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain("esa carga no existe");
    const fila = document.getElementById(
      boton.getAttribute("aria-controls") ?? "",
    );
    expect(fila?.contains(alerta)).toBe(true);
    // El resto del listado sigue en pie.
    expect(filaCon("caracol")).toBeTruthy();
  });
});
