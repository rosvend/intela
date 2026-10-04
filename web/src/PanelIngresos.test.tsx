import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  FilaIngreso,
  PanelExplicacion,
  PanelIngresos,
  TablaIngresos,
} from "./PanelIngresos";
import type { Explicacion, Ingreso, ValorizacionDeUso } from "./ingresos";
import { api } from "./api";

vi.mock("./api", () => ({
  api: vi.fn(),
}));

const anaCasa: Ingreso = {
  ref: "proc-2026-01:obra-completa:tit-ana",
  obra_id: "obra-completa",
  obra: "La Casa de las Dos Palmas",
  fuente: "caracol",
  periodo: "2026-01",
  neto: "3600.00",
};

const anaSegundo: Ingreso = {
  ref: "proc-2026-01:obra-ana-2:tit-ana",
  obra_id: "obra-ana-2",
  obra: "El Segundo Guion",
  fuente: "caracol",
  periodo: "2026-01",
  neto: "750.00",
};

function usoTV(id: string): ValorizacionDeUso {
  return {
    uso_id: id,
    formula: "RD 9.1.1",
    puntos: "5616",
    terminos: [
      {
        producto: "5616",
        factores: [
          { nombre: "ponderacion", valor: "1.3", origen: "parametro" },
          { nombre: "duracion_min", valor: "48", origen: "uso" },
          { nombre: "rating", valor: "9", origen: "uso" },
          { nombre: "emisiones", valor: "10", origen: "uso" },
        ],
      },
    ],
  };
}

const lineaTV =
  "RD 9.1.1: ponderacion 1.3 × duracion (min) 48 × rating 9 × emisiones 10 = 5616 puntos";

const linaje: Explicacion = {
  ref: anaCasa.ref,
  neto: "3600.00",
  bruto: "4800.00",
  retenida: false,
  firmas: [],
  corrida: {
    proceso_id: "proc-2026-01",
    periodo: "2026-01",
    circuito: "nacional",
  },
  reporte: {
    id: "rpt-caracol-2026-01",
    fuente: "caracol",
    sha256: "aaaa",
  },
  obra: {
    id: "obra-completa",
    titulo: "La Casa de las Dos Palmas",
    escalon: "alias",
    puntaje: "1.00000",
    puntos: "5616",
  },
  valorizacion: [usoTV("u-1")],
  regla: { snapshot_id: "snap-2026-01", reglamento: "RD-IX" },
  split: {
    titular_id: "tit-ana",
    ipi: "IPI-00000001",
    porcentaje: "60.0000",
    version: 1,
  },
  deducciones: [
    {
      concepto: "gastos administrativos",
      porcentaje: "10.00",
      monto: "480.00",
    },
    { concepto: "bienestar social", porcentaje: "5.00", monto: "240.00" },
    { concepto: "reserva", porcentaje: "10.00", monto: "480.00" },
  ],
};

// Con los codigos de concepto y la cita compuesta reales (ver web/src/reglamento.ts),
// para probar el recibo de "mas detalles" contra datos que de verdad puede
// devolver GET /explicar/{ref}.
const linajeReal: Explicacion = {
  ...linaje,
  valorizacion: [],
  regla: {
    snapshot_id: "snap-2026-01",
    reglamento: "RD 9.1.1+RD-IX-seed-sintetico",
  },
  deducciones: [
    {
      concepto: "gastos_administrativos",
      porcentaje: "10.00",
      monto: "480.00",
    },
    { concepto: "bienestar_social", porcentaje: "5.00", monto: "240.00" },
    {
      concepto: "reserva_errores_tecnicos",
      porcentaje: "10.00",
      monto: "480.00",
    },
  ],
};

afterEach(() => {
  cleanup();
  vi.mocked(api).mockReset();
});

describe("PanelExplicacion", () => {
  it("pinta el linaje de ExplicarCifra tal cual llega", () => {
    render(<PanelExplicacion cifra={linaje} />);

    const panel = screen.getByRole("region", {
      name: "Explicacion de la cifra",
    });
    expect(panel.textContent).toContain("3600.00");
    expect(panel.textContent).toContain("4800.00");
    expect(panel.textContent).toContain("proc-2026-01");
    expect(panel.textContent).toContain("rpt-caracol-2026-01");
    expect(panel.textContent).toContain("caracol");
    expect(panel.textContent).toContain("escalon alias");
    expect(panel.textContent).toContain("puntaje 1.00000");
    expect(panel.textContent).toContain("RD-IX");
    expect(panel.textContent).toContain("snap-2026-01");
    expect(panel.textContent).toContain("60.0000%");
    expect(panel.textContent).toContain("declaracion v1");
    expect(panel.textContent).toContain("gastos administrativos");
    expect(panel.textContent).toContain("bienestar social");
    expect(panel.textContent).toContain("reserva");
  });

  it("no inventa una version de declaracion cuando la corrida no la persistio", () => {
    render(
      <PanelExplicacion
        cifra={{ ...linaje, split: { ...linaje.split, version: null } }}
      />,
    );
    const panel = screen.getByRole("region", {
      name: "Explicacion de la cifra",
    });
    expect(panel.textContent).not.toContain("declaracion v");
  });

  it("el recibo de 'mas detalles' empieza oculto", () => {
    render(<PanelExplicacion cifra={linajeReal} />);
    expect(screen.queryByLabelText("Recibo en lenguaje sencillo")).toBeNull();
    expect(screen.getByRole("button", { name: "Mas detalles" })).toBeTruthy();
  });

  it("'mas detalles' pinta bruto, cada deduccion en lenguaje llano con su cita, y neto", () => {
    render(<PanelExplicacion cifra={linajeReal} />);
    fireEvent.click(screen.getByRole("button", { name: "Mas detalles" }));

    const recibo = screen.getByLabelText("Recibo en lenguaje sencillo");
    // Orden bruto -> deducciones -> neto, en lenguaje llano.
    expect(recibo.textContent).toContain("Gastos administrativos");
    expect(recibo.textContent).toContain("gastos_administrativos");
    expect(recibo.textContent).toContain("10.00% = $ 480.00");
    expect(recibo.textContent).toContain("Bienestar social");
    expect(recibo.textContent).toContain("Reserva para errores tecnicos");
    // La cita R-06/R-07 verbatim, no solo el numeral.
    expect(recibo.textContent).toContain("gastos administrativos");
    expect(recibo.textContent).toContain("Ley 44 de 1993");
    expect(recibo.textContent).toContain("5% del recaudo nacional");
    // El split declarado del titular.
    expect(recibo.textContent).toContain("60.0000%");
    expect(recibo.textContent).toContain("IPI-00000001");
    // La cita de la modalidad y del marcador sintetico, con nombre humano.
    expect(recibo.textContent).toContain("Television abierta y radiodifundida");
    expect(recibo.textContent).toContain("Total puntos por obra");
    expect(recibo.textContent).toContain("Cifra provisional de siembra");

    fireEvent.click(
      screen.getByRole("button", { name: "Ocultar mas detalles" }),
    );
    expect(screen.queryByLabelText("Recibo en lenguaje sencillo")).toBeNull();
  });

  it("una obra retenida explica R-04 con la cita de RD 13.1.3", () => {
    render(
      <PanelExplicacion
        cifra={{
          ...linajeReal,
          retenida: true,
          motivo: "declaracion incompleta: falta un titular",
        }}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Mas detalles" }));
    const recibo = screen.getByLabelText("Recibo en lenguaje sencillo");
    expect(recibo.textContent).toContain("retuvo por completo");
    expect(recibo.textContent).toContain(
      "declaracion incompleta: falta un titular",
    );
    expect(recibo.textContent).toContain(
      "Retencion por declaracion incompleta",
    );
    expect(recibo.textContent).toContain("declaracion discriminada del 100%");
  });

  it("las firmas de la corrida salen en el recibo cuando llegan", () => {
    render(
      <PanelExplicacion
        cifra={{
          ...linajeReal,
          firmas: [
            {
              rol: "distribucion",
              actor_id: "user-distribucion-1",
              sobre_revision: 0,
              etapa: "verificacion",
              cuando: "2026-02-01T10:00:00Z",
            },
          ],
        }}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Mas detalles" }));
    const recibo = screen.getByLabelText("Recibo en lenguaje sencillo");
    expect(recibo.textContent).toContain("distribucion");
    expect(recibo.textContent).toContain("user-distribucion-1");
    expect(recibo.textContent).toContain("verificacion");
    expect(recibo.textContent).toContain("2026-02-01");
  });

  it("el recibo itemiza los puntos de la obra por factor", () => {
    render(<PanelExplicacion cifra={linaje} />);
    fireEvent.click(screen.getByRole("button", { name: "Mas detalles" }));

    expect(screen.getByText(lineaTV)).toBeTruthy();
    expect(
      screen.getByText(
        (_, el) =>
          el?.tagName === "P" &&
          el.textContent === "Total de la obra: 5616 puntos",
      ),
    ).toBeTruthy();
  });

  it("una cifra anterior al desglose dice que solo se conserva el total", () => {
    render(<PanelExplicacion cifra={{ ...linaje, valorizacion: [] }} />);
    fireEvent.click(screen.getByRole("button", { name: "Mas detalles" }));

    expect(
      screen.getByText(/se conserva solo el total: 5616 puntos/),
    ).toBeTruthy();
    expect(screen.queryByText(/= 5616 puntos/)).toBeNull();
  });

  it("con mas de 5 usos muestra 5 y un desplegable con el resto", () => {
    const usos = ["u-1", "u-2", "u-3", "u-4", "u-5", "u-6", "u-7"].map(usoTV);
    const { container } = render(
      <PanelExplicacion cifra={{ ...linaje, valorizacion: usos }} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Mas detalles" }));

    const items = () => container.querySelectorAll(".recibo-puntos ul li");
    expect(items().length).toBe(5);

    fireEvent.click(screen.getByRole("button", { name: "Ver los 7 usos" }));
    expect(items().length).toBe(7);
    expect(screen.getByRole("button", { name: "Ver menos" })).toBeTruthy();
  });
});

describe("FilaIngreso", () => {
  it("enseña el neto como cifra de cabecera, nunca el bruto", () => {
    render(
      <table>
        <tbody>
          <FilaIngreso
            fila={anaCasa}
            abierta={false}
            explicacion={null}
            error=""
            cargando={false}
            onExplicar={() => undefined}
          />
        </tbody>
      </table>,
    );
    expect(screen.getByText("$ 3600.00")).toBeTruthy();
    expect(screen.queryByText("$ 4800.00")).toBeNull();
    expect(screen.queryByRole("columnheader", { name: "Bruto" })).toBeNull();
  });

  it("expande a la explicacion con un clic", () => {
    const onExplicar = vi.fn();
    render(
      <table>
        <tbody>
          <FilaIngreso
            fila={anaCasa}
            abierta={false}
            explicacion={null}
            error=""
            cargando={false}
            onExplicar={onExplicar}
          />
        </tbody>
      </table>,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Explicar esta cifra" }),
    );
    expect(onExplicar).toHaveBeenCalledOnce();
  });
});

describe("PanelIngresos", () => {
  beforeEach(() => {
    vi.mocked(api).mockImplementation(async (path: string) => {
      if (path.startsWith("/api/mis-ingresos")) {
        const q = new URL(path, "http://local").searchParams;
        return {
          ingresos: [anaCasa, anaSegundo].filter((fila) => {
            if (q.get("obra") && fila.obra_id !== q.get("obra")) return false;
            if (q.get("fuente") && fila.fuente !== q.get("fuente"))
              return false;
            if (q.get("periodo") && fila.periodo !== q.get("periodo"))
              return false;
            return true;
          }),
        };
      }
      if (path.includes(encodeURIComponent(anaCasa.ref))) {
        return linaje;
      }
      if (path.includes("tit-beto")) {
        throw new Error('{"error":"no autorizado"}');
      }
      throw new Error("ruta no mockeada: " + path);
    });
  });

  it("carga como titular A y solo pinta las obras de A", async () => {
    render(<PanelIngresos />);

    expect(
      await screen.findByRole("cell", { name: "La Casa de las Dos Palmas" }),
    ).toBeTruthy();
    expect(screen.getByRole("cell", { name: "El Segundo Guion" })).toBeTruthy();
    expect(screen.queryByText("Solo de Beto")).toBeNull();
    expect(screen.queryByText("tit-beto")).toBeNull();

    const llamada = vi.mocked(api).mock.calls[0][0];
    expect(llamada).toBe("/api/mis-ingresos");
    expect(llamada).not.toContain("titular");
  });

  it("los tres filtros recortan la tabla", async () => {
    render(<PanelIngresos />);
    await screen.findByRole("cell", { name: "El Segundo Guion" });

    fireEvent.change(screen.getByLabelText("Filtrar por obra"), {
      target: { value: "obra-completa" },
    });
    await waitFor(() => {
      expect(
        vi
          .mocked(api)
          .mock.calls.some((c) => String(c[0]).includes("obra=obra-completa")),
      ).toBe(true);
    });
    expect(
      screen.getByRole("cell", { name: "La Casa de las Dos Palmas" }),
    ).toBeTruthy();
    expect(screen.queryByRole("cell", { name: "El Segundo Guion" })).toBeNull();

    fireEvent.change(screen.getByLabelText("Filtrar por obra"), {
      target: { value: "" },
    });
    fireEvent.change(screen.getByLabelText("Filtrar por fuente"), {
      target: { value: "caracol" },
    });
    expect(
      await screen.findByRole("cell", { name: "El Segundo Guion" }),
    ).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Filtrar por periodo"), {
      target: { value: "2026-01" },
    });
    expect(screen.getAllByText("2026-01").length).toBeGreaterThan(0);
  });

  it("al explicar, pinta el linaje exacto de ExplicarCifra", async () => {
    render(<PanelIngresos />);
    await screen.findByRole("cell", { name: "La Casa de las Dos Palmas" });

    fireEvent.click(
      screen.getAllByRole("button", { name: "Explicar esta cifra" })[0],
    );

    await waitFor(() => {
      expect(
        screen.getByRole("region", { name: "Explicacion de la cifra" }),
      ).toBeTruthy();
    });
    const panel = screen.getByRole("region", {
      name: "Explicacion de la cifra",
    });
    expect(panel.textContent).toContain("snap-2026-01");
    expect(panel.textContent).toContain("escalon alias");
    expect(panel.textContent).toContain("4800.00");
  });

  it("pedir la ref de otro titular muestra 403, no sus datos", async () => {
    render(<PanelIngresos />);
    await screen.findByRole("cell", { name: "La Casa de las Dos Palmas" });

    vi.mocked(api).mockRejectedValueOnce(
      new Error('{"error":"no autorizado"}'),
    );
    fireEvent.click(
      screen.getAllByRole("button", { name: "Explicar esta cifra" })[0],
    );

    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(screen.queryByText("Solo de Beto")).toBeNull();
    expect(screen.queryByText("tit-beto")).toBeNull();
  });
});

describe("TablaIngresos", () => {
  const linajeB: Explicacion = {
    ...linaje,
    ref: anaSegundo.ref,
    neto: "750.00",
    bruto: "900.00",
    obra: {
      ...linaje.obra,
      id: anaSegundo.obra_id,
      titulo: "El Segundo Guion",
    },
  };

  it("una respuesta tardia no sustituye el linaje de la cifra abierta", async () => {
    let resolverA: (v: Explicacion) => void = () => {};
    let resolverB: (v: Explicacion) => void = () => {};
    vi.mocked(api).mockImplementation((path: string) => {
      if (String(path).includes(encodeURIComponent(anaCasa.ref))) {
        return new Promise((resolver) => {
          resolverA = resolver as (v: Explicacion) => void;
        });
      }
      if (String(path).includes(encodeURIComponent(anaSegundo.ref))) {
        return new Promise((resolver) => {
          resolverB = resolver as (v: Explicacion) => void;
        });
      }
      return Promise.reject(new Error("ruta no mockeada: " + path));
    });

    render(<TablaIngresos filas={[anaCasa, anaSegundo]} />);
    const botones = screen.getAllByRole("button", {
      name: "Explicar esta cifra",
    });
    fireEvent.click(botones[0]);
    fireEvent.click(botones[1]);
    resolverB(linajeB);
    resolverA(linaje);

    const panel = await screen.findByRole("region", {
      name: "Explicacion de la cifra",
    });
    expect(panel.textContent).toContain("El Segundo Guion");
    expect(panel.textContent).toContain("750.00");
    expect(panel.textContent).not.toContain("La Casa de las Dos Palmas");
    expect(panel.textContent).not.toContain("4800.00");
  });

  it("cerrar el panel invalida la peticion pendiente", async () => {
    let resolverA: (v: Explicacion) => void = () => {};
    vi.mocked(api).mockImplementation((path: string) => {
      if (String(path).includes(encodeURIComponent(anaCasa.ref))) {
        return new Promise((resolver) => {
          resolverA = resolver as (v: Explicacion) => void;
        });
      }
      return Promise.reject(new Error("ruta no mockeada: " + path));
    });

    render(<TablaIngresos filas={[anaCasa]} />);
    const boton = screen.getByRole("button", { name: "Explicar esta cifra" });
    fireEvent.click(boton);
    fireEvent.click(boton);
    resolverA(linaje);

    await waitFor(() => {
      expect(
        screen.queryByRole("region", { name: "Explicacion de la cifra" }),
      ).toBeNull();
    });
  });
});
