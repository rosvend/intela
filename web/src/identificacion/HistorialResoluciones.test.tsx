import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Asiento } from "../auditoria/tipos";
import HistorialResoluciones from "./HistorialResoluciones";

function json(cuerpo: unknown, status = 200): Response {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

/**
 * El payload de `identificacion.asignada` segun la descripcion del esquema
 * `Asiento` del contrato provisional de #175 (snake_case), el mismo que usa
 * `tipos.test.ts` para `leerResolucionAsentada`. Titulos y fuentes
 * sinteticos, no de ningun canal real.
 */
function payloadDeResolucion(
  extra: Partial<Record<string, unknown>> = {},
): Record<string, unknown> {
  return {
    uso_id: "uso-1",
    decision: "asignar",
    obra_id: "obra-9",
    titulo: "Serie de Prueba T3 E12",
    fuente: "canal-prueba",
    periodo: "2024-11",
    reporte_id: "rep-1",
    nota: "coincide la ficha técnica y el elenco",
    actor_nombre: "Ana Pérez",
    ...extra,
  };
}

const RECIENTE: Asiento = {
  id: "a-2",
  hecho: "identificacion.asignada",
  ref_tipo: "obra",
  ref_id: "obra-9",
  actor: "usr-2",
  payload: payloadDeResolucion({
    uso_id: "uso-2",
    titulo: "Serie de Prueba T4 E01",
    reporte_id: "rep-2",
    nota: "misma ficha tecnica que la anterior",
    actor_nombre: "Luis Gómez",
  }),
  cuando: "2026-05-10T09:00:00Z",
};

const ANTIGUO: Asiento = {
  id: "a-1",
  hecho: "identificacion.asignada",
  ref_tipo: "obra",
  ref_id: "obra-9",
  actor: "usr-1",
  payload: payloadDeResolucion(),
  cuando: "2026-01-05T09:00:00Z",
};

/** Un asiento de otra familia: no se cuenta ni se pinta aqui. */
const OTRO_HECHO: Asiento = {
  id: "a-0",
  hecho: "obra.registrada",
  ref_tipo: "obra",
  ref_id: "obra-9",
  actor: "usr-1",
  payload: { titulo: "Serie de Prueba" },
  cuando: "2026-01-01T09:00:00Z",
};

/** `identificacion.asignada` pero sin `actor_nombre`: `leerResolucionAsentada` lo rechaza entero. */
const ILEGIBLE: Asiento = {
  id: "a-3",
  hecho: "identificacion.asignada",
  ref_tipo: "obra",
  ref_id: "obra-9",
  actor: "usr-3",
  payload: { uso_id: "uso-3", titulo: "Sin actor" },
  cuando: "2026-06-01T09:00:00Z",
};

function simularServidor(respuesta: () => Response) {
  vi.mocked(fetch).mockImplementation((entrada) => {
    if (String(entrada) === "/api/auditoria/obra/obra-9") {
      return Promise.resolve(respuesta());
    }
    return Promise.resolve(json({ error: "ruta no encontrada" }, 404));
  });
}

function montar() {
  return render(<HistorialResoluciones obraId="obra-9" />);
}

describe("HistorialResoluciones", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("pinta el titulo, el subtitulo y el contador", async () => {
    simularServidor(() => json([ANTIGUO, RECIENTE]));

    montar();

    expect(
      await screen.findByRole("heading", {
        name: "Historial de resoluciones manuales",
      }),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Decisiones que vincularon registros de uso con esta obra.",
      ),
    ).toBeTruthy();
    expect(screen.getByText("2 resoluciones")).toBeTruthy();
  });

  it("ordena del mas reciente al mas antiguo, no en el orden en que llego la API", async () => {
    // La API los manda en orden de cadena (el antiguo primero): si el
    // componente no reordenara, este test seguiria pasando por casualidad.
    simularServidor(() => json([ANTIGUO, RECIENTE]));

    montar();

    const filas = await screen.findAllByRole("listitem");
    expect(filas).toHaveLength(2);
    expect(filas[0]?.textContent).toContain("Luis Gómez");
    expect(filas[0]?.textContent).toContain("Serie de Prueba T4 E01");
    expect(filas[1]?.textContent).toContain("Ana Pérez");
    expect(filas[1]?.textContent).toContain("Serie de Prueba T3 E12");
  });

  it("excluye los asientos que no son de una asignacion manual", async () => {
    simularServidor(() => json([OTRO_HECHO, ANTIGUO]));

    montar();

    expect(await screen.findByText("1 resoluciones")).toBeTruthy();
    expect(screen.queryByText(/Serie de Prueba$/)).toBeNull();
  });

  it("interpola el titulo, la fuente y el periodo del payload en el texto de la entrada", async () => {
    simularServidor(() => json([ANTIGUO]));

    montar();

    expect(
      await screen.findByText(
        "Asignó “Serie de Prueba T3 E12” de canal-prueba, 2024-11, a esta obra.",
      ),
    ).toBeTruthy();
  });

  it("la nota y la referencia de origen (uso_id · reporte_id) son visibles", async () => {
    simularServidor(() => json([ANTIGUO]));

    montar();

    expect(
      await screen.findByText("coincide la ficha técnica y el elenco"),
    ).toBeTruthy();
    expect(screen.getByText("uso-1 · rep-1")).toBeTruthy();
  });

  it("un payload que no se puede leer no esconde la fila: se muestra con su propio aviso", async () => {
    simularServidor(() => json([ILEGIBLE]));

    montar();

    const filas = await screen.findAllByRole("listitem");
    expect(filas).toHaveLength(1);
    expect(
      screen.getByText("Resolución manual con un detalle que no se pudo leer."),
    ).toBeTruthy();
    // Sin `actor_nombre` legible, la cabecera cae al `actor` del asiento en
    // vez de quedarse sin ningun indicio de quien decidio.
    expect(filas[0]?.textContent).toContain("usr-3");
  });

  it("el estado vacio dice que la obra no tiene resoluciones manuales", async () => {
    simularServidor(() => json([]));

    montar();

    expect(
      await screen.findByText("Esta obra no tiene resoluciones manuales"),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Las decisiones futuras aparecerán aquí con su nota y referencia de origen.",
      ),
    ).toBeTruthy();
    expect(screen.queryByRole("listitem")).toBeNull();
  });

  it("un error del servidor se dice como tal, sin fingir una lista vacia", async () => {
    simularServidor(() => json({ error: "no se pudo consultar" }, 500));

    montar();

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain(
      "No se pudo consultar el historial de resoluciones:",
    );
    expect(alerta.textContent).toContain("no se pudo consultar");
    expect(
      screen.queryByText("Esta obra no tiene resoluciones manuales"),
    ).toBeNull();
  });
});
