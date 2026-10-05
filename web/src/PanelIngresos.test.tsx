import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
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
import { useSesion } from "./sesion";

vi.mock("./api", () => ({
  api: vi.fn(),
}));

vi.mock("./sesion", () => ({
  useSesion: vi.fn(),
}));

function comoRol(rol: string) {
  vi.mocked(useSesion).mockReturnValue({
    usuario: { rol },
  } as unknown as ReturnType<typeof useSesion>);
}

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
  fuente: "rcn",
  periodo: "2026-01",
  neto: "750.00",
};

function usoTV(id: string, puntos = "5616"): ValorizacionDeUso {
  return {
    uso_id: id,
    formula: "RD 9.1.1",
    puntos,
    terminos: [
      {
        producto: puntos,
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

const linaje: Explicacion = {
  ref: anaCasa.ref,
  neto: "3600.00",
  bruto: "4800.00",
  retenida: false,
  corrida: {
    proceso_id: "proc-2026-01",
    periodo: "2026-01",
    circuito: "nacional",
  },
  bolsa: { id: "bolsa-caracol", usuario_id: "caracol", bruto: "1000000.00" },
  reporte: {
    id: "rpt-caracol-2026-01",
    fuente: "caracol",
    sha256: "aaaa1111",
  },
  obra: {
    id: "obra-completa",
    titulo: "La Casa de las Dos Palmas",
    escalon: "alias",
    puntaje: "1.00000",
    puntos: "5616",
  },
  valorizacion: [usoTV("u-1")],
  regla: {
    snapshot_id: "snap-2026-01",
    reglamento: "RD 9.1.1+RD-IX-seed-sintetico",
  },
  split: {
    titular_id: "tit-ana",
    ipi: "IPI-00000001",
    porcentaje: "60.0000",
    version: 1,
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
  firmas: [
    {
      rol: "distribucion",
      actor_id: "user-distribucion-1",
      sobre_revision: 0,
      etapa: "verificacion",
      cuando: "2026-02-01T10:00:00Z",
    },
  ],
};

const JERGA = [
  "proc-2026-01",
  "rpt-caracol-2026-01",
  "aaaa1111",
  "snap-2026-01",
  "escal",
  "alias",
  "IPI-00000001",
  "user-distribucion-1",
  "Reporte de origen",
  "Obra y match",
  "RD-IX-seed-sintetico",
  "Corrida",
];

/** "$ 1.234,5" -> centavos, para sumar lo que la pantalla anuncia. */
function centavosDe(etiqueta: string): number {
  const m = /\$ ([\d.]+)(?:,(\d{1,2}))?/.exec(etiqueta);
  if (!m) throw new Error(`sin importe en "${etiqueta}"`);
  return (
    Number(m[1].replace(/\./g, "")) * 100 + Number((m[2] ?? "0").padEnd(2, "0"))
  );
}

beforeEach(() => comoRol("titular"));

afterEach(() => {
  cleanup();
  vi.mocked(api).mockReset();
});

describe("PanelExplicacion: la historia de la cifra", () => {
  it("cuenta la cifra de arriba abajo: recaudo, tu parte, descuentos y lo que recibes", () => {
    render(<PanelExplicacion cifra={linaje} />);
    const panel = screen.getByRole("region", {
      name: "Cómo se calculó tu pago",
    });
    const pasos = within(panel).getAllByRole("heading", { level: 4 });
    expect(pasos.map((h) => h.textContent)).toEqual([
      "Lo que se recaudó",
      "Tu parte",
      "Descuentos de ley",
      "Tú recibes",
      "Cómo se usó tu obra",
    ]);
    expect(panel.textContent).toContain("$ 1.000.000");
    expect(panel.textContent).toContain("Caracol Televisión");
    expect(screen.getByRole("img", { name: "Tu parte: 60 %" })).toBeTruthy();
    expect(panel.textContent).toContain("$ 4.800");
    expect(screen.getByLabelText("$ 3.600")).toBeTruthy();
  });

  it("los segmentos de los descuentos suman exactamente el bruto", () => {
    render(<PanelExplicacion cifra={linaje} />);
    const barra = screen.getByRole("group", { name: "Descuentos de ley" });
    const segmentos = within(barra).getAllByRole("button");
    expect(segmentos).toHaveLength(4);
    expect(segmentos[0].getAttribute("aria-label")).toMatch(
      /^Tú recibes: \$ 3\.600/,
    );
    const suma = segmentos
      .map((s) => centavosDe(s.getAttribute("aria-label") ?? ""))
      .reduce((a, b) => a + b, 0);
    expect(suma).toBe(centavosDe("$ 4.800"));
  });

  it("el detalle de un descuento abre con la razon llana y la cita del reglamento", () => {
    render(<PanelExplicacion cifra={linaje} />);
    fireEvent.click(
      screen.getByRole("button", { name: /^Gastos administrativos: \$ 480/ }),
    );
    const dialogo = screen.getByRole("dialog", {
      name: "Gastos administrativos",
    });
    expect(dialogo.textContent).toContain("operar");
    expect(dialogo.textContent).toContain("Ley 44 de 1993");
  });

  it("el detalle de tu parte explica que el porcentaje sale de la declaracion", () => {
    render(<PanelExplicacion cifra={linaje} />);
    fireEvent.click(
      screen.getByRole("button", { name: "¿De dónde sale tu parte?" }),
    );
    expect(screen.getByRole("dialog").textContent).toContain(
      "Declaración de Obra",
    );
  });

  it("al titular no le muestra jerga tecnica ni el detalle tecnico", () => {
    render(<PanelExplicacion cifra={linaje} />);
    const texto =
      screen.getByRole("region", { name: "Cómo se calculó tu pago" })
        .textContent ?? "";
    for (const j of JERGA) expect(texto, j).not.toContain(j);
    expect(
      screen.queryByRole("button", { name: /detalle técnico/ }),
    ).toBeNull();
  });

  it("las firmas se vuelven una sola linea amable", () => {
    render(<PanelExplicacion cifra={linaje} />);
    expect(
      screen.getByText(
        "Revisado y aprobado por REDES SGC el 1 de febrero de 2026",
      ),
    ).toBeTruthy();
  });

  it("sin firmas no promete una aprobacion", () => {
    render(<PanelExplicacion cifra={{ ...linaje, firmas: [] }} />);
    expect(screen.queryByText(/Revisado y aprobado/)).toBeNull();
  });

  it("el personal ve un detalle tecnico plegado con el linaje completo", () => {
    render(<PanelExplicacion cifra={linaje} tecnico />);
    const boton = screen.getByRole("button", { name: "Ver detalle técnico" });
    expect(boton.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByText(/snap-2026-01/)).toBeNull();

    fireEvent.click(boton);
    const tecnico = screen.getByRole("region", { name: "Detalle técnico" });
    for (const dato of [
      "proc-2026-01",
      "rpt-caracol-2026-01",
      "aaaa1111",
      "escalón alias",
      "snap-2026-01",
      "IPI-00000001",
      "declaración v1",
      "user-distribucion-1",
      "Cifra provisional de siembra",
    ]) {
      expect(tecnico.textContent, dato).toContain(dato);
    }
  });

  it("una obra retenida se muestra en reserva, con el motivo llano y que hacer", () => {
    render(
      <PanelExplicacion
        cifra={{
          ...linaje,
          neto: "0.00",
          deducciones: [],
          retenida: true,
          motivo: "declaracion_incompleta",
        }}
      />,
    );
    const aviso = screen.getByRole("status", {
      name: "Este pago está en reserva",
    });
    expect(aviso.textContent).toContain("100 %");
    expect(aviso.textContent).toContain("REDES SGC");
    expect(aviso.textContent).toContain("$ 4.800");
    expect(aviso.textContent).not.toContain("declaracion_incompleta");
    expect(
      screen.queryByRole("group", { name: "Descuentos de ley" }),
    ).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "¿Por qué está en reserva?" }),
    );
    expect(screen.getByRole("dialog").textContent).toContain(
      "declaracion discriminada del 100%",
    );
  });

  it("cada uso de la obra es una barra con su detalle en palabras", () => {
    render(
      <PanelExplicacion
        cifra={{
          ...linaje,
          valorizacion: [usoTV("u-1"), usoTV("u-2", "2808")],
        }}
      />,
    );
    const usos = screen.getByRole("list", { name: "Usos de tu obra" });
    const barras = within(usos).getAllByRole("button");
    expect(barras).toHaveLength(2);
    fireEvent.click(barras[1]);
    expect(screen.getByRole("dialog").textContent).toContain(
      "se emitió 10 veces",
    );
  });

  it("con mas de 5 usos muestra 5 y un desplegable con el resto", () => {
    const usos = ["u-1", "u-2", "u-3", "u-4", "u-5", "u-6", "u-7"].map((u) =>
      usoTV(u),
    );
    render(<PanelExplicacion cifra={{ ...linaje, valorizacion: usos }} />);
    const lista = () => screen.getByRole("list", { name: "Usos de tu obra" });
    expect(within(lista()).getAllByRole("button")).toHaveLength(5);
    fireEvent.click(screen.getByRole("button", { name: "Ver los 7 usos" }));
    expect(within(lista()).getAllByRole("button")).toHaveLength(7);
  });

  it("una cifra anterior al desglose dice que solo se conserva el total de puntos", () => {
    render(<PanelExplicacion cifra={{ ...linaje, valorizacion: [] }} />);
    expect(screen.getByText(/5\.616 puntos/)).toBeTruthy();
    expect(screen.queryByRole("list", { name: "Usos de tu obra" })).toBeNull();
  });
});

describe("FilaIngreso", () => {
  it("muestra obra, fuente llana, periodo en palabras y el neto; nunca el bruto", () => {
    render(
      <FilaIngreso
        fila={anaCasa}
        abierta={false}
        explicacion={null}
        error=""
        cargando={false}
        onExplicar={() => undefined}
      />,
    );
    const boton = screen.getByRole("button", {
      name: /La Casa de las Dos Palmas/,
    });
    expect(boton.textContent).toContain("Caracol Televisión");
    expect(boton.textContent).toContain("enero 2026");
    expect(boton.textContent).toContain("$ 3.600");
    expect(screen.queryByText(/4\.800/)).toBeNull();
  });

  it("pulsar la fila la despliega", () => {
    const onExplicar = vi.fn();
    render(
      <FilaIngreso
        fila={anaCasa}
        abierta={false}
        explicacion={null}
        error=""
        cargando={false}
        onExplicar={onExplicar}
      />,
    );
    const boton = screen.getByRole("button", {
      name: /La Casa de las Dos Palmas/,
    });
    expect(boton.getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(boton);
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
      if (path.includes(encodeURIComponent(anaCasa.ref))) return linaje;
      throw new Error("ruta no mockeada: " + path);
    });
  });

  it("carga solo los ingresos de la sesion, sin pasar el titular", async () => {
    render(<PanelIngresos />);
    expect(
      await screen.findByRole("button", { name: /La Casa de las Dos Palmas/ }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: /El Segundo Guion/ }),
    ).toBeTruthy();
    const llamada = vi.mocked(api).mock.calls[0][0];
    expect(llamada).toBe("/api/mis-ingresos");
    expect(llamada).not.toContain("titular");
  });

  it("los filtros recortan la lista y nombran fuente y periodo en llano", async () => {
    render(<PanelIngresos />);
    await screen.findByRole("button", { name: /El Segundo Guion/ });

    const fuente = screen.getByLabelText("Filtrar por fuente");
    expect(
      within(fuente).getByRole("option", { name: "RCN Televisión" }),
    ).toBeTruthy();
    const periodo = screen.getByLabelText("Filtrar por periodo");
    expect(
      within(periodo).getByRole("option", { name: "enero 2026" }),
    ).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Filtrar por obra"), {
      target: { value: "obra-completa" },
    });
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: /El Segundo Guion/ }),
      ).toBeNull(),
    );
    expect(
      vi
        .mocked(api)
        .mock.calls.some((c) => String(c[0]).includes("obra=obra-completa")),
    ).toBe(true);
  });

  it("desplegar una fila pinta la historia de su cifra", async () => {
    render(<PanelIngresos />);
    fireEvent.click(
      await screen.findByRole("button", { name: /La Casa de las Dos Palmas/ }),
    );
    expect(
      await screen.findByRole("region", { name: "Cómo se calculó tu pago" }),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("button", { name: /La Casa de las Dos Palmas/ })
        .getAttribute("aria-expanded"),
    ).toBe("true");
  });

  it("el personal ve el detalle tecnico y el titular no", async () => {
    comoRol("auditor");
    render(<PanelIngresos />);
    fireEvent.click(
      await screen.findByRole("button", { name: /La Casa de las Dos Palmas/ }),
    );
    expect(
      await screen.findByRole("button", { name: "Ver detalle técnico" }),
    ).toBeTruthy();
  });

  it("pedir una cifra ajena muestra el error, no sus datos", async () => {
    render(<PanelIngresos />);
    await screen.findByRole("button", { name: /La Casa de las Dos Palmas/ });
    vi.mocked(api).mockRejectedValueOnce(new Error("no autorizado"));
    fireEvent.click(
      screen.getByRole("button", { name: /La Casa de las Dos Palmas/ }),
    );
    expect((await screen.findByRole("alert")).textContent).toContain(
      "no autorizado",
    );
  });

  it("sin ingresos muestra un vacio amable", async () => {
    vi.mocked(api).mockResolvedValue({ ingresos: [] });
    render(<PanelIngresos />);
    expect(
      await screen.findByText("Aún no tienes ingresos liquidados"),
    ).toBeTruthy();
  });
});

describe("TablaIngresos", () => {
  const linajeB: Explicacion = {
    ...linaje,
    ref: anaSegundo.ref,
    neto: "750.00",
    bruto: "900.00",
    deducciones: [],
    obra: {
      ...linaje.obra,
      id: anaSegundo.obra_id,
      titulo: "El Segundo Guion",
    },
  };

  it("una respuesta tardia no sustituye la cifra abierta", async () => {
    let resolverA: (v: Explicacion) => void = () => {};
    let resolverB: (v: Explicacion) => void = () => {};
    vi.mocked(api).mockImplementation((path: string) => {
      if (String(path).includes(encodeURIComponent(anaCasa.ref))) {
        return new Promise((r) => {
          resolverA = r as (v: Explicacion) => void;
        });
      }
      return new Promise((r) => {
        resolverB = r as (v: Explicacion) => void;
      });
    });

    render(<TablaIngresos filas={[anaCasa, anaSegundo]} />);
    fireEvent.click(
      screen.getByRole("button", { name: /La Casa de las Dos Palmas/ }),
    );
    fireEvent.click(screen.getByRole("button", { name: /El Segundo Guion/ }));
    resolverB(linajeB);
    resolverA(linaje);

    const panel = await screen.findByRole("region", {
      name: "Cómo se calculó tu pago",
    });
    expect(panel.textContent).toContain("$ 900");
    expect(panel.textContent).not.toContain("$ 4.800");
  });

  it("cerrar la fila invalida la peticion pendiente", async () => {
    let resolverA: (v: Explicacion) => void = () => {};
    vi.mocked(api).mockImplementation(
      () =>
        new Promise((r) => {
          resolverA = r as (v: Explicacion) => void;
        }),
    );
    render(<TablaIngresos filas={[anaCasa]} />);
    const boton = screen.getByRole("button", {
      name: /La Casa de las Dos Palmas/,
    });
    fireEvent.click(boton);
    fireEvent.click(boton);
    resolverA(linaje);
    await waitFor(() =>
      expect(
        screen.queryByRole("region", { name: "Cómo se calculó tu pago" }),
      ).toBeNull(),
    );
  });
});
