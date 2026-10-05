import {
  act,
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
import PanelCorridas from "./PanelCorridas";
import { resumenDePrueba } from "./resumenDePrueba";
import { INTERVALO_SONDEO_MS, Proceso, RUTAS_REPARTO } from "./tipos";

function json(cuerpo: unknown, status = 200) {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function usuario(rol: Rol) {
  return {
    id: "usr-1",
    email: "x@redes.co",
    nombre: "Persona de Prueba",
    rol,
    titular_id: "",
  };
}

const nacional: Proceso = {
  id: "proc-nac",
  circuito: "nacional",
  etapa: "verificacion",
  periodo: "2025",
  bolsa_id: "bolsa-nac",
  snapshot_id: "snap-1",
  revision: 1,
  firmas: [],
};

const internacional: Proceso = {
  id: "proc-int",
  circuito: "internacional",
  etapa: "recaudo",
  periodo: "2025-06",
  bolsa_id: "bolsa-int",
  snapshot_id: "snap-1",
  revision: 1,
  firmas: [],
};

function montar(ruta = "/distribucion") {
  return render(
    <MemoryRouter initialEntries={[ruta]}>
      <ProveedorDeSesion>
        <Routes>
          <Route path="/distribucion" element={<PanelCorridas />} />
          <Route path="/distribucion/:id" element={<PanelCorridas />} />
        </Routes>
      </ProveedorDeSesion>
    </MemoryRouter>,
  );
}

describe("PanelCorridas", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    setToken("tok");
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("agrupa las corridas por periodo y enlaza cada periodo", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") {
        return json(usuario("administrador"));
      }
      if (path === RUTAS_REPARTO.procesos) {
        return json([internacional, nacional]);
      }
      if (path.startsWith("/api/alertas")) {
        return json(resumenDePrueba());
      }
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    const periodos = await screen.findByRole("navigation", {
      name: "Periodos",
    });
    const enlaces = within(periodos).getAllByRole("link");
    // El mas reciente primero, y es el que se abre sin id.
    expect(enlaces.map((e) => e.textContent)).toEqual(["2025-06", "2025"]);
    expect(enlaces[0].getAttribute("aria-current")).toBe("true");
    const lista = screen.getByRole("list", { name: "Corridas" });
    expect(within(lista).getAllByRole("listitem")).toHaveLength(1);
    expect(within(lista).getByText(/Internacional/)).toBeTruthy();
    expect(screen.getAllByText("Recaudo").length).toBeGreaterThan(0);
  });

  it("una compuerta muestra firmados y pendientes", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") {
        return json(usuario("administrador"));
      }
      if (path === RUTAS_REPARTO.procesos) {
        return json([
          {
            ...nacional,
            firmas: [{ rol: "distribucion", actor_id: "usr-d", revision: 1 }],
          },
        ]);
      }
      if (path.startsWith("/api/alertas")) return json(resumenDePrueba());
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await waitFor(() => expect(screen.getByText("Firmado")).toBeTruthy());
    expect(screen.getByText("Pendiente")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Firmar" })).toBeNull();
  });

  it("firmar como distribucion recarga y la etapa avanza", async () => {
    let etapa: Proceso["etapa"] = "verificacion";
    vi.mocked(fetch).mockImplementation(async (input, init) => {
      const path = String(input);
      if (path === "/api/auth/session") {
        return json(usuario("distribucion"));
      }
      if (
        path === RUTAS_REPARTO.firmar("proc-nac") &&
        init?.method === "POST"
      ) {
        etapa = "liquidacion_final";
        return new Response(null, { status: 204 });
      }
      if (path === RUTAS_REPARTO.procesos) {
        return json([{ ...nacional, etapa }]);
      }
      if (path.startsWith("/api/alertas")) return json(resumenDePrueba());
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Firmar" })).toBeTruthy(),
    );
    fireEvent.click(screen.getByRole("button", { name: "Firmar" }));

    await waitFor(() =>
      expect(screen.getAllByText("Liquidación final").length).toBeGreaterThan(
        0,
      ),
    );
    const firma = vi
      .mocked(fetch)
      .mock.calls.find(
        ([url, init]) =>
          String(url) === RUTAS_REPARTO.firmar("proc-nac") &&
          init?.method === "POST",
      );
    expect(firma?.[1]?.body).toBe(JSON.stringify({ accion: "firmar" }));
  });

  it("un 403 al firmar se muestra y no oculta el pipeline", async () => {
    vi.mocked(fetch).mockImplementation(async (input, init) => {
      const path = String(input);
      if (path === "/api/auth/session") {
        return json(usuario("distribucion"));
      }
      if (
        path === RUTAS_REPARTO.firmar("proc-nac") &&
        init?.method === "POST"
      ) {
        return json({ error: "no autorizado" }, 403);
      }
      if (path === RUTAS_REPARTO.procesos) {
        return json([nacional]);
      }
      if (path.startsWith("/api/alertas")) return json(resumenDePrueba());
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Firmar" })).toBeTruthy(),
    );
    fireEvent.click(screen.getByRole("button", { name: "Firmar" }));

    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toBe("no autorizado"),
    );
    expect(screen.getByText("Importe de la obra")).toBeTruthy();
  });

  it("un id inexistente nunca ofrece firmar otra corrida", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") return json(usuario("distribucion"));
      if (path === RUTAS_REPARTO.procesos) return json([nacional]);
      return json(resumenDePrueba());
    });
    montar("/distribucion/no-existe");
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toContain("no encontrado"),
    );
    expect(screen.queryByRole("button", { name: "Firmar" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Rechazar" })).toBeNull();
  });

  it("navegar a otra corrida descarta el motivo y actúa sobre el id visible", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") return json(usuario("distribucion"));
      if (path === RUTAS_REPARTO.procesos)
        return json([nacional, { ...internacional, etapa: "verificacion" }]);
      return json(resumenDePrueba());
    });
    montar("/distribucion/proc-nac");
    await screen.findByRole("button", { name: "Rechazar" });
    fireEvent.click(screen.getByRole("button", { name: "Rechazar" }));
    fireEvent.change(screen.getByLabelText("Motivo del rechazo"), {
      target: { value: "Problema nacional" },
    });
    fireEvent.click(screen.getByRole("link", { name: "2025-06" }));
    expect(screen.queryByLabelText("Motivo del rechazo")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Rechazar" }));
    expect(
      (screen.getByLabelText("Motivo del rechazo") as HTMLTextAreaElement)
        .value,
    ).toBe("");
    fireEvent.change(screen.getByLabelText("Motivo del rechazo"), {
      target: { value: "Problema internacional" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Confirmar rechazo" }));
    await waitFor(() =>
      expect(
        vi.mocked(fetch).mock.calls.some(
          ([url, init]) =>
            String(url) === RUTAS_REPARTO.firmar("proc-int") &&
            init?.body ===
              JSON.stringify({
                accion: "rechazar",
                motivo: "Problema internacional",
              }),
        ),
      ).toBe(true),
    );
    expect(
      vi
        .mocked(fetch)
        .mock.calls.some(
          ([url]) => String(url) === RUTAS_REPARTO.firmar("proc-nac"),
        ),
    ).toBe(false);
  });

  it("una firma tardía no muestra su error en otra corrida", async () => {
    let responder!: (response: Response) => void;
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") return json(usuario("distribucion"));
      if (path === RUTAS_REPARTO.procesos)
        return json([nacional, { ...internacional, etapa: "verificacion" }]);
      if (path === RUTAS_REPARTO.firmar("proc-nac"))
        return new Promise<Response>((resolve) => {
          responder = resolve;
        });
      return json(resumenDePrueba());
    });
    montar("/distribucion/proc-nac");
    fireEvent.click(await screen.findByRole("button", { name: "Firmar" }));
    fireEvent.click(screen.getByRole("link", { name: "2025-06" }));
    await act(async () => {
      responder(json({ error: "Error nacional" }, 409));
    });
    expect(screen.queryByText("Error nacional")).toBeNull();
    expect(
      (screen.getByRole("button", { name: "Firmar" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false);
  });

  it("contabilidad pide las alertas y ve el aviso antes de firmar", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") return json(usuario("contabilidad"));
      if (path === RUTAS_REPARTO.procesos) return json([nacional]);
      if (path.startsWith("/api/alertas"))
        return json(
          resumenDePrueba({
            abiertas: 250,
            criticas_abiertas: 150,
            porTipo: { oni: 100, duplicado_registro: 150 },
          }),
        );
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    const aviso = await screen.findByText(/150 críticas bloquean/);
    expect(aviso.textContent).toContain("250 alertas abiertas");
    expect(
      vi
        .mocked(fetch)
        .mock.calls.some(([url]) =>
          String(url).startsWith("/api/alertas/resumen"),
        ),
    ).toBe(true);
    // Una tarjeta por tipo de alerta (seis), sin descripción repetida.
    const rejilla = screen.getByRole("region", { name: "Alertas del periodo" });
    expect(within(rejilla).getAllByRole("article")).toHaveLength(6);
    expect(screen.queryByText("Alertas abiertas del periodo")).toBeNull();
    // Una sola acción para resolverlas, como botón.
    const resolver = within(rejilla).getByRole("link", {
      name: "Resolver alertas",
    });
    expect(resolver.classList.contains("boton-secundario")).toBe(true);
    expect(resolver.getAttribute("href")).toBe("/anomalias?periodo=2025");
  });

  it("si el resumen falla avisa que no se pudo leer el estado de anomalias", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") return json(usuario("distribucion"));
      if (path === RUTAS_REPARTO.procesos) return json([nacional]);
      if (path.startsWith("/api/alertas"))
        return json({ error: "se cayo" }, 500);
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await screen.findByText(/No se pudo leer el estado de anomalías/);
  });

  it("firmar con alertas abiertas avisa, pero no bloquea la compuerta", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") return json(usuario("distribucion"));
      if (path === RUTAS_REPARTO.procesos) return json([nacional]);
      if (path.startsWith("/api/alertas"))
        return json(
          resumenDePrueba({
            abiertas: 2,
            porTipo: { oni: 1, reserva_declaracion_incompleta: 1 },
          }),
        );
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await screen.findByText("2 alertas abiertas; ninguna bloquea la corrida.");
    expect(
      (screen.getByRole("button", { name: "Firmar" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false);
  });

  it("volver a una corrida no revive el error de firma que quedo atras", async () => {
    vi.mocked(fetch).mockImplementation(async (input, init) => {
      const path = String(input);
      if (path === "/api/auth/session") return json(usuario("distribucion"));
      if (path === RUTAS_REPARTO.firmar("proc-nac") && init?.method === "POST")
        return json({ error: "Error nacional" }, 409);
      if (path === RUTAS_REPARTO.procesos)
        return json([nacional, { ...internacional, etapa: "verificacion" }]);
      return json(resumenDePrueba());
    });

    montar("/distribucion/proc-nac");

    fireEvent.click(await screen.findByRole("button", { name: "Firmar" }));
    await waitFor(() =>
      expect(screen.getByText("Error nacional")).toBeTruthy(),
    );

    fireEvent.click(screen.getByRole("link", { name: "2025-06" }));
    expect(screen.queryByText("Error nacional")).toBeNull();

    fireEvent.click(screen.getByRole("link", { name: "2025" }));
    expect(screen.queryByText("Error nacional")).toBeNull();
  });

  it("sin backend de procesos muestra el vacio, no un crash", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      if (String(input) === "/api/auth/session") {
        return json(usuario("administrador"));
      }
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    await waitFor(() =>
      expect(screen.getByText(/Sin datos todavía/)).toBeTruthy(),
    );
    expect(screen.queryByRole("alert")).toBeNull();
  });

  describe("con bolsas por pagador", () => {
    const caracol: Proceso = {
      ...nacional,
      id: "proc-caracol",
      periodo: "2025-01",
      etapa: "verificacion",
      bolsa_id: "bolsa-caracol",
    };
    const rcn: Proceso = {
      ...nacional,
      id: "proc-rcn",
      periodo: "2025-01",
      etapa: "deducciones",
      bolsa_id: "bolsa-rcn",
    };
    const viejo: Proceso = {
      ...nacional,
      id: "proc-viejo",
      periodo: "2024-12",
      etapa: "auditoria",
      bolsa_id: "bolsa-vieja",
    };
    const bolsas = [
      {
        id: "bolsa-caracol",
        usuario_id: "caracol",
        periodo: "2025-01",
        circuito: "nacional",
        bruto: "600000000.00",
      },
      {
        id: "bolsa-rcn",
        usuario_id: "rcn",
        periodo: "2025-01",
        circuito: "nacional",
        bruto: "400000000.00",
      },
      {
        id: "bolsa-vieja",
        usuario_id: "netflix",
        periodo: "2024-12",
        circuito: "nacional",
        bruto: "100000000.00",
      },
    ];

    function servir(
      rol: Rol,
      extra?: (path: string, init?: RequestInit) => Response | undefined,
    ) {
      vi.mocked(fetch).mockImplementation(async (input, init) => {
        const path = String(input);
        const respuesta = extra?.(path, init);
        if (respuesta) return respuesta;
        if (path === "/api/auth/session") return json(usuario(rol));
        // RCN antes que Caracol: el orden lo pone la bolsa, no la API.
        if (path === RUTAS_REPARTO.procesos) return json([viejo, rcn, caracol]);
        if (path === "/api/bolsas") return json(bolsas);
        if (path.startsWith("/api/alertas")) return json(resumenDePrueba());
        return json({ error: "ruta no encontrada" }, 404);
      });
    }

    async function detalle() {
      return screen.findByRole("region", { name: "Corrida seleccionada" });
    }

    it("cada corrida se nombra por su pagador, con monto, etapa y avance", async () => {
      servir("administrador");
      montar();

      const lista = await screen.findByRole("list", { name: "Corridas" });
      await within(lista).findByText("Caracol");
      const items = within(lista).getAllByRole("listitem");
      expect(items).toHaveLength(2);
      // Mayor bolsa primero; la de 2024-12 vive en su propio periodo.
      expect(within(items[0]).getByText("Caracol")).toBeTruthy();
      expect(within(items[0]).getByText("$ 600 M")).toBeTruthy();
      expect(within(items[0]).getByText(/Verificación/)).toBeTruthy();
      expect(within(items[0]).getByLabelText("Etapa 6 de 9")).toBeTruthy();
      expect(within(items[1]).getByText("RCN")).toBeTruthy();
      expect(within(items[1]).getByText("$ 400 M")).toBeTruthy();
      expect(within(lista).queryByText("Netflix")).toBeNull();
      expect(
        within(items[0]).getByRole("link").getAttribute("aria-current"),
      ).toBe("true");
    });

    it("el resumen del periodo suma las bolsas y las reparte por pagador", async () => {
      servir("administrador");
      montar();

      const resumen = await screen.findByRole("region", {
        name: "Resumen del periodo",
      });
      await within(resumen).findByLabelText("$ 1.000.000.000");
      const barra = within(resumen).getByRole("group", {
        name: "Bolsas por pagador",
      });
      expect(within(barra).getAllByRole("button")).toHaveLength(2);
    });

    it("la corrida elegida vive en la URL", async () => {
      servir("administrador");
      montar("/distribucion/proc-rcn");

      const region = await detalle();
      await within(region).findByRole("heading", { name: "RCN" });
      const lista = screen.getByRole("list", { name: "Corridas" });
      fireEvent.click(within(lista).getByRole("link", { name: /Caracol/ }));
      await within(await detalle()).findByRole("heading", { name: "Caracol" });
      expect(
        within(lista)
          .getByRole("link", { name: /Caracol/ })
          .getAttribute("aria-current"),
      ).toBe("true");
    });

    it("un tramo de la barra lleva a su corrida", async () => {
      servir("administrador");
      montar();

      const barra = await screen.findByRole("group", {
        name: "Bolsas por pagador",
      });
      await waitFor(() =>
        expect(within(barra).getAllByRole("button")).toHaveLength(2),
      );
      fireEvent.click(within(barra).getByRole("button", { name: /^RCN/ }));
      fireEvent.click(screen.getByRole("link", { name: "Ver corrida de RCN" }));
      await within(await detalle()).findByRole("heading", { name: "RCN" });
    });

    it("las cifras de la corrida salen de su bolsa, sin inventar deducciones", async () => {
      servir("administrador");
      montar("/distribucion/proc-caracol");

      const region = await detalle();
      const cifras = await within(region).findByRole("list", {
        name: "Cifras de la corrida",
      });
      await within(cifras).findByText("$ 600.000.000");
      expect(within(cifras).getByText("60 %")).toBeTruthy();
      expect(within(cifras).getByText("6 de 9")).toBeTruthy();
      expect(within(region).queryByText(/Deducciones aplicadas/)).toBeNull();
    });

    it("cambiar de periodo abre su primera corrida", async () => {
      servir("administrador");
      montar();

      await screen.findByRole("list", { name: "Corridas" });
      fireEvent.click(screen.getByRole("link", { name: "2024-12" }));
      await within(await detalle()).findByRole("heading", { name: "Netflix" });
    });

    it("administración avanza la corrida tras confirmar", async () => {
      servir("administrador", (path, init) => {
        if (
          path === `${RUTAS_REPARTO.proceso("proc-rcn")}/avanzar` &&
          init?.method === "POST"
        ) {
          return json({ ...rcn, etapa: "importe_obra" });
        }
        return undefined;
      });
      montar("/distribucion/proc-rcn");

      const avanzar = await screen.findByRole("button", {
        name: "Avanzar a Importe de la obra",
      });
      fireEvent.click(avanzar);
      fireEvent.click(screen.getByRole("button", { name: "Confirmar avance" }));
      await waitFor(() =>
        expect(
          vi
            .mocked(fetch)
            .mock.calls.some(
              ([url, init]) =>
                String(url) ===
                  `${RUTAS_REPARTO.proceso("proc-rcn")}/avanzar` &&
                init?.method === "POST",
            ),
        ).toBe(true),
      );
    });

    it("un 409 al avanzar se muestra junto al botón", async () => {
      servir("administrador", (path, init) => {
        if (
          path === `${RUTAS_REPARTO.proceso("proc-rcn")}/avanzar` &&
          init?.method === "POST"
        ) {
          return json({ error: "hay anomalias criticas" }, 409);
        }
        return undefined;
      });
      montar("/distribucion/proc-rcn");

      fireEvent.click(
        await screen.findByRole("button", {
          name: "Avanzar a Importe de la obra",
        }),
      );
      fireEvent.click(screen.getByRole("button", { name: "Confirmar avance" }));
      await waitFor(() =>
        expect(screen.getByRole("alert").textContent).toBe(
          "hay anomalias criticas",
        ),
      );
    });

    it("no ofrece avanzar a quien no administra ni una compuerta sin firmas", async () => {
      servir("distribucion");
      montar("/distribucion/proc-rcn");
      await within(await detalle()).findByRole("heading", { name: "RCN" });
      expect(screen.queryByRole("button", { name: /^Avanzar/ })).toBeNull();
      cleanup();

      servir("administrador");
      montar("/distribucion/proc-caracol");
      await within(await detalle()).findByRole("heading", { name: "Caracol" });
      expect(screen.queryByRole("button", { name: /^Avanzar/ })).toBeNull();
    });
  });

  it("sin bolsa, la corrida se nombra por su circuito", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") return json(usuario("administrador"));
      if (path === RUTAS_REPARTO.procesos) return json([nacional]);
      if (path.startsWith("/api/alertas")) return json(resumenDePrueba());
      return json({ error: "ruta no encontrada" }, 404);
    });

    montar();

    const lista = await screen.findByRole("list", { name: "Corridas" });
    expect(within(lista).getByText("Corrida nacional")).toBeTruthy();
    expect(within(lista).queryByText(/\$/)).toBeNull();
  });

  it("vuelve a pedir los procesos cada 15 segundos", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      vi.mocked(fetch).mockImplementation(async (input) => {
        const path = String(input);
        if (path === "/api/auth/session") return json(usuario("auditor"));
        if (path === RUTAS_REPARTO.procesos) return json([nacional]);
        return json({ error: "ruta no encontrada" }, 404);
      });
      montar();
      await screen.findByRole("list", { name: "Corridas" });
      const pedidos = () =>
        vi
          .mocked(fetch)
          .mock.calls.filter(([url]) => String(url) === RUTAS_REPARTO.procesos)
          .length;
      const antes = pedidos();
      await act(async () => {
        vi.advanceTimersByTime(INTERVALO_SONDEO_MS);
      });
      await waitFor(() => expect(pedidos()).toBe(antes + 1));
    } finally {
      vi.useRealTimers();
    }
  });

  it("no hay detalle técnico: la trazabilidad vive en Auditoría", async () => {
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/auth/session") return json(usuario("administrador"));
      if (path === RUTAS_REPARTO.procesos) return json([internacional]);
      if (path.startsWith("/api/alertas")) return json(resumenDePrueba());
      return json({ error: "ruta no encontrada" }, 404);
    });

    const { container } = montar();

    await screen.findByRole("list", { name: "Corridas" });
    expect(screen.queryByText(/Detalle técnico/)).toBeNull();
    expect(container.querySelector("details")).toBeNull();
    expect(container.textContent).not.toContain("proc-int");
    expect(screen.queryByText(/Cada reparto avanza por etapas/)).toBeNull();
  });

  describe("revisión 2 de Killgreck", () => {
    function proceso(parcial: Partial<Proceso>): Proceso {
      return {
        ...nacional,
        periodo: "2026-01",
        etapa: "deducciones",
        ...parcial,
      };
    }

    function servirCon(
      procesos: () => Proceso[],
      bolsas: () => Response,
      rol: Rol = "administrador",
    ) {
      vi.mocked(fetch).mockImplementation(async (input) => {
        const path = String(input);
        if (path === "/api/auth/session") return json(usuario(rol));
        if (path === RUTAS_REPARTO.procesos) return json(procesos());
        if (path === "/api/bolsas") return bolsas();
        if (path.startsWith("/api/alertas")) return json(resumenDePrueba());
        return json({ error: "ruta no encontrada" }, 404);
      });
    }

    const bolsaCaracol = {
      id: "bolsa-caracol-2026-01-nacional",
      usuario_id: "caracol",
      periodo: "2026-01",
      circuito: "nacional",
      bruto: "1890000000.00",
    };

    it("B2: dos corridas sobre la misma bolsa no la cuentan dos veces", async () => {
      // La API las lista por id: la -10 llega antes que la -2.
      const decima = proceso({
        id: "proc-bolsa-caracol-2026-01-nacional-10",
        bolsa_id: bolsaCaracol.id,
      });
      const segunda = proceso({
        id: "proc-bolsa-caracol-2026-01-nacional-2",
        bolsa_id: bolsaCaracol.id,
      });
      servirCon(
        () => [decima, segunda],
        () => json([bolsaCaracol]),
      );
      montar("/distribucion/proc-bolsa-caracol-2026-01-nacional-10");

      const resumen = await screen.findByRole("region", {
        name: "Resumen del periodo",
      });
      await within(resumen).findByLabelText("$ 1.890.000.000");
      expect(within(resumen).queryByLabelText("$ 3.780.000.000")).toBeNull();
      expect(
        within(resumen).getByText("2 corridas sobre 1 bolsa"),
      ).toBeTruthy();
      expect(within(resumen).queryByText(/una por pagador/)).toBeNull();

      // Se distinguen por orden de apertura, no por el orden de la API.
      const lista = within(resumen).getByRole("list", { name: "Corridas" });
      const items = within(lista).getAllByRole("listitem");
      expect(items).toHaveLength(2);
      expect(within(items[0]).getByText("Caracol")).toBeTruthy();
      expect(within(items[1]).getByText("Caracol · corrida 2")).toBeTruthy();

      const barra = within(resumen).getByRole("group", {
        name: "Bolsas por pagador",
      });
      expect(within(barra).getAllByRole("button")).toHaveLength(1);

      const region = await screen.findByRole("region", {
        name: "Corrida seleccionada",
      });
      expect(
        within(region).getByRole("heading", { name: "Caracol · corrida 2" }),
      ).toBeTruthy();
      const cifras = within(region).getByRole("list", {
        name: "Cifras de la corrida",
      });
      expect(within(cifras).getByText("100 %")).toBeTruthy();
    });

    it("B3: un monto compacto lleva el exacto en title y en el nombre accesible", async () => {
      servirCon(
        () => [proceso({ id: "proc-grande", bolsa_id: "bolsa-grande" })],
        () =>
          json([
            {
              ...bolsaCaracol,
              id: "bolsa-grande",
              bruto: "123456789012.34",
            },
          ]),
      );
      montar();

      const lista = await screen.findByRole("list", { name: "Corridas" });
      const exacto = "$ 123.456.789.012,34";
      const monto = await within(lista).findByTitle(exacto);
      expect(monto.textContent).toContain(exacto);
      expect(
        within(lista).getByRole("link", {
          name: new RegExp(exacto.replace(/[$.]/g, "\\$&")),
        }),
      ).toBeTruthy();

      const cifras = screen.getByRole("list", { name: "Cifras de la corrida" });
      expect(within(cifras).getByText(exacto)).toBeTruthy();
      expect(screen.getByLabelText(exacto)).toBeTruthy();
    });

    it("I2: si /api/bolsas falla avisa y no muestra total ni peso", async () => {
      servirCon(
        () => [proceso({ id: "proc-a", bolsa_id: "bolsa-a" })],
        () => json({ error: "fallo interno" }, 500),
      );
      montar();

      const alerta = await screen.findByRole("alert");
      expect(alerta.textContent).toMatch(/no se pudieron cargar los montos/i);
      const resumen = screen.getByRole("region", {
        name: "Resumen del periodo",
      });
      expect(within(resumen).queryByText(/Bolsa del periodo/)).toBeNull();
      expect(screen.queryByText("Peso en el periodo")).toBeNull();
    });

    it("I2: una corrida sin su bolsa en el listado no deja un total parcial", async () => {
      servirCon(
        () => [
          proceso({ id: "proc-a", bolsa_id: "bolsa-a" }),
          proceso({ id: "proc-b", bolsa_id: "bolsa-b" }),
        ],
        () => json([{ ...bolsaCaracol, id: "bolsa-a", bruto: "100.00" }]),
      );
      montar("/distribucion/proc-a");

      const resumen = await screen.findByRole("region", {
        name: "Resumen del periodo",
      });
      await within(resumen).findByText(/Falta el monto/);
      expect(within(resumen).queryByText(/Bolsa del periodo/)).toBeNull();
      expect(screen.queryByText("Peso en el periodo")).toBeNull();
    });

    it("I2: el sondeo vuelve a pedir las bolsas y el total se actualiza", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      try {
        const a = proceso({ id: "proc-a", bolsa_id: "bolsa-a" });
        const b = proceso({ id: "proc-b", bolsa_id: "bolsa-b" });
        const bolsaA = { ...bolsaCaracol, id: "bolsa-a", bruto: "100.00" };
        const bolsaB = {
          ...bolsaCaracol,
          id: "bolsa-b",
          usuario_id: "rcn",
          bruto: "900.00",
        };
        let llegoLaNueva = false;
        servirCon(
          () => (llegoLaNueva ? [a, b] : [a]),
          () => json(llegoLaNueva ? [bolsaA, bolsaB] : [bolsaA]),
        );
        montar("/distribucion/proc-a");

        const resumen = await screen.findByRole("region", {
          name: "Resumen del periodo",
        });
        await within(resumen).findByLabelText("$ 100");

        llegoLaNueva = true;
        await act(async () => {
          vi.advanceTimersByTime(INTERVALO_SONDEO_MS);
        });

        await within(resumen).findByLabelText("$ 1.000");
        const cifras = screen.getByRole("list", {
          name: "Cifras de la corrida",
        });
        await within(cifras).findByText("10 %");
      } finally {
        vi.useRealTimers();
      }
    });

    it("I7: el motivo del último rechazo se muestra", async () => {
      servirCon(
        () => [
          proceso({
            id: "proc-a",
            bolsa_id: "bolsa-a",
            etapa: "verificacion",
            rechazo_motivo: "faltan soportes de la liquidacion parcial",
          }),
        ],
        () => json([]),
      );
      montar();

      const estado = await screen.findByText(/Último rechazo/);
      expect(estado.textContent).toBe(
        "Último rechazo: faltan soportes de la liquidacion parcial",
      );
    });
  });
});
