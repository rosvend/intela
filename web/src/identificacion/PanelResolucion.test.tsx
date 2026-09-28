import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DEBOUNCE_TECLEO_MS } from "../useValorDiferido";
import Dialogo from "./Dialogo";
import PanelResolucion, { type ModoResolucion } from "./PanelResolucion";
import { MAX_NOTA } from "./resolucion";
import type { CasoIdentificacion } from "./tipos";

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
  ids_fuente: "ID_Ficha=48213\nID_Emision=991204",
  evidencia: "Coincidencia parcial por título y número de episodio.",
  estado: "pendiente",
  candidatos: [],
  obra_asignada: null,
  resuelto_por: null,
  resuelto_en: null,
  ultima_actualizacion: "2024-11-05T10:00:00Z",
  nota: null,
} satisfies CasoIdentificacion;

/**
 * Monta `PanelResolucion` DENTRO de un `Dialogo`, tal como lo usa
 * `BandejaIdentificacion`: el panel no gestiona su propio foco ni Escape, asi
 * que probar eso en aislamiento (sin el `Dialogo`) probaria un mecanismo que
 * la pantalla real nunca ejercita.
 */
function Arnes({
  modo,
  onCancelar,
  onEnviarInicio = vi.fn(),
  onEnviandoCambia = vi.fn(),
  onFalla = vi.fn(),
  onExito = vi.fn(),
  onRecargarTodo = vi.fn(),
}: {
  modo: ModoResolucion;
  onCancelar?: () => void;
  onEnviarInicio?: () => void;
  onEnviandoCambia?: (enviando: boolean) => void;
  onFalla?: (volvioALaLista: boolean) => void;
  onExito?: (info: { obraTitulo: string | null; esDescarte: boolean }) => void;
  onRecargarTodo?: () => void;
}) {
  const [abierto, setAbierto] = useState(true);
  const [bloqueado, setBloqueado] = useState(false);
  const cerrar = () => {
    setAbierto(false);
    onCancelar?.();
  };
  return (
    <Dialogo
      abierto={abierto}
      variante="lateral"
      idTitulo="titulo-panel"
      bloqueado={bloqueado}
      onCerrar={cerrar}
    >
      <PanelResolucion
        idTitulo="titulo-panel"
        caso={casoUno}
        modo={modo}
        onCancelar={cerrar}
        onEnviarInicio={onEnviarInicio}
        onEnviandoCambia={(v) => {
          setBloqueado(v);
          onEnviandoCambia(v);
        }}
        onFalla={onFalla}
        onExito={onExito}
        onRecargarTodo={onRecargarTodo}
      />
    </Dialogo>
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("PanelResolucion", () => {
  it("el boton principal empieza deshabilitado sin nota, y se habilita al escribir una nota no vacia", () => {
    render(<Arnes modo={{ tipo: "descarte" }} />);

    const boton = screen.getByRole("button", { name: "Descartar registro" });
    expect(boton.hasAttribute("disabled")).toBe(true);

    fireEvent.change(screen.getByLabelText("Nota *"), {
      target: { value: "  " },
    });
    // Solo espacios no cuenta como nota (se recorta antes de enviar).
    expect(boton.hasAttribute("disabled")).toBe(true);

    fireEvent.change(screen.getByLabelText("Nota *"), {
      target: { value: "motivo del descarte" },
    });
    expect(boton.hasAttribute("disabled")).toBe(false);
  });

  it("modo busqueda: el boton principal exige ademas una obra elegida", () => {
    render(<Arnes modo={{ tipo: "busqueda" }} />);

    expect(
      screen.getByRole("button", { name: "Selecciona una obra" }),
    ).not.toBeNull();

    fireEvent.change(screen.getByLabelText("Nota *"), {
      target: { value: "nota valida" },
    });

    // Con nota pero sin obra elegida, sigue pidiendo elegir una.
    const boton = screen.getByRole("button", { name: "Selecciona una obra" });
    expect(boton.hasAttribute("disabled")).toBe(true);
  });

  it("el textarea de la nota tiene el tope del contrato y el contador lo refleja", () => {
    render(<Arnes modo={{ tipo: "descarte" }} />);

    const textarea = screen.getByLabelText("Nota *") as HTMLTextAreaElement;
    expect(textarea.maxLength).toBe(MAX_NOTA);
    expect(screen.getByText(`0/${MAX_NOTA}`)).not.toBeNull();

    fireEvent.change(textarea, { target: { value: "hola" } });
    expect(screen.getByText(`4/${MAX_NOTA}`)).not.toBeNull();
  });

  it("modo candidata: llega con la obra ya elegida y el boton dice a cual", () => {
    render(
      <Arnes
        modo={{
          tipo: "candidata",
          obraId: "obra-1",
          obraTitulo: "La Niña",
          obraAnio: 2016,
          obraGenero: "Drama",
          puntaje: 0.71,
        }}
      />,
    );

    expect(screen.getByText("Obra elegida")).not.toBeNull();
    expect(screen.getByText("La Niña")).not.toBeNull();
    expect(screen.getByText("0,71")).not.toBeNull();
    expect(
      screen.getByRole("button", { name: "Asignar a La Niña" }),
    ).not.toBeNull();
  });

  it("modo descarte: muestra el aviso de que el registro no se asigna a ninguna obra", () => {
    render(<Arnes modo={{ tipo: "descarte" }} />);

    expect(
      screen.getByText("Este registro no se asignará a ninguna obra"),
    ).not.toBeNull();
    expect(
      screen.getByText(
        "Queda como descartado: no pondera en el reparto ni sale en el listado público de ONI. La decisión y tu nota quedan en la bitácora.",
      ),
    ).not.toBeNull();
  });

  it("busqueda: no pide nada con el campo vacio, y pide con debounce al escribir (D7)", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => json([])),
    );
    render(<Arnes modo={{ tipo: "busqueda" }} />);

    // Campo vacio: ninguna peticion (D7, "no listar obras al azar").
    expect(fetch).not.toHaveBeenCalled();

    const campo = screen.getByLabelText("Título de la obra");
    vi.useFakeTimers();
    fireEvent.change(campo, { target: { value: "Nov" } });
    expect(fetch).not.toHaveBeenCalled();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(DEBOUNCE_TECLEO_MS);
    });
    vi.useRealTimers();

    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
    expect(vi.mocked(fetch).mock.calls[0][0]).toBe(
      "/api/obras?titulo=Nov&limite=5",
    );
  });

  it("Escape (delegado al Dialogo) cierra el panel llamando a onCancelar", () => {
    const onCancelar = vi.fn();
    render(<Arnes modo={{ tipo: "descarte" }} onCancelar={onCancelar} />);

    fireEvent.keyDown(document, { key: "Escape" });

    expect(onCancelar).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("al abrir, el foco (gestionado por el Dialogo) cae dentro del panel", () => {
    render(<Arnes modo={{ tipo: "descarte" }} />);

    const dialogo = screen.getByRole("dialog");
    expect(dialogo.contains(document.activeElement)).toBe(true);
  });

  it("mientras se envia, el panel avisa a onEnviandoCambia y Escape ya no cierra (bloqueado)", async () => {
    let liberar: (r: Response) => void = () => {};
    const enVuelo = new Promise<Response>((resolver) => {
      liberar = resolver;
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => enVuelo),
    );
    const onEnviandoCambia = vi.fn();
    const onEnviarInicio = vi.fn();

    render(
      <Arnes
        modo={{ tipo: "descarte" }}
        onEnviandoCambia={onEnviandoCambia}
        onEnviarInicio={onEnviarInicio}
      />,
    );

    fireEvent.change(screen.getByLabelText("Nota *"), {
      target: { value: "nota" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Descartar registro" }));

    expect(onEnviarInicio).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(onEnviandoCambia).toHaveBeenCalledWith(true));
    expect(screen.getByRole("button", { name: "Guardando…" })).not.toBeNull();

    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("dialog")).not.toBeNull();

    liberar(json({}));
    await waitFor(() => expect(onEnviandoCambia).toHaveBeenCalledWith(false));
  });

  it("cuerpo exacto para descarte: {decision: descartar, nota} recortada, sin obra_id", async () => {
    let cuerpoEnviado: unknown = null;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_entrada: RequestInfo | URL, init?: RequestInit) => {
        cuerpoEnviado = JSON.parse(String(init?.body));
        return json({});
      }),
    );
    const onExito = vi.fn();
    render(<Arnes modo={{ tipo: "descarte" }} onExito={onExito} />);

    fireEvent.change(screen.getByLabelText("Nota *"), {
      target: { value: "  no es del repertorio  " },
    });
    fireEvent.click(screen.getByRole("button", { name: "Descartar registro" }));

    await waitFor(() => expect(onExito).toHaveBeenCalledTimes(1));
    expect(cuerpoEnviado).toEqual({
      decision: "descartar",
      nota: "no es del repertorio",
    });
    expect(onExito).toHaveBeenCalledWith({
      obraTitulo: null,
      esDescarte: true,
    });
  });

  it("cuerpo exacto para candidata: {decision: asignar, obra_id, nota} recortada", async () => {
    let cuerpoEnviado: unknown = null;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_entrada: RequestInfo | URL, init?: RequestInit) => {
        cuerpoEnviado = JSON.parse(String(init?.body));
        return json({});
      }),
    );
    const onExito = vi.fn();
    render(
      <Arnes
        modo={{
          tipo: "candidata",
          obraId: "obra-1",
          obraTitulo: "La Niña",
          obraAnio: 2016,
          obraGenero: "Drama",
          puntaje: 0.71,
        }}
        onExito={onExito}
      />,
    );

    fireEvent.change(screen.getByLabelText("Nota *"), {
      target: { value: " coincide " },
    });
    fireEvent.click(screen.getByRole("button", { name: "Asignar a La Niña" }));

    await waitFor(() => expect(onExito).toHaveBeenCalledTimes(1));
    expect(cuerpoEnviado).toEqual({
      decision: "asignar",
      obra_id: "obra-1",
      nota: "coincide",
    });
    expect(onExito).toHaveBeenCalledWith({
      obraTitulo: "La Niña",
      esDescarte: false,
    });
  });

  it("un error que no es 409 de la API muestra el mensaje del servidor, con la nota intacta", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => json({ error: "la nota es obligatoria" }, 400)),
    );
    const onFalla = vi.fn();
    render(<Arnes modo={{ tipo: "descarte" }} onFalla={onFalla} />);

    fireEvent.change(screen.getByLabelText("Nota *"), {
      target: { value: "una nota" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Descartar registro" }));

    expect(
      await screen.findByText(
        "No pudimos guardar la resolución: la nota es obligatoria. Tu nota sigue aquí.",
      ),
    ).not.toBeNull();
    expect(onFalla).toHaveBeenCalledWith(true);
    expect((screen.getByLabelText("Nota *") as HTMLTextAreaElement).value).toBe(
      "una nota",
    );
  });
});
