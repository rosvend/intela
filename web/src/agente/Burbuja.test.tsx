import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setToken } from "../api";
import Burbuja from "./Burbuja";

function respuestaSSE(cuerpo: string, status = 200): Response {
  return new Response(cuerpo, {
    status,
    headers: { "content-type": "text/event-stream" },
  });
}

function montar() {
  return render(
    <MemoryRouter>
      <Burbuja />
    </MemoryRouter>,
  );
}

function preguntar(texto: string) {
  fireEvent.change(screen.getByLabelText("Tu pregunta"), {
    target: { value: texto },
  });
  fireEvent.click(screen.getByRole("button", { name: "Enviar" }));
}

describe("Burbuja", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    setToken("tok");
  });
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("empieza cerrada y abre el panel al pulsar la burbuja", () => {
    montar();
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Abrir asistente" }));
    expect(
      screen.getByRole("dialog", { name: "Asistente de Intela" }),
    ).toBeTruthy();
  });

  it("al abrir enfoca la pregunta y al cerrar devuelve el foco a la burbuja", () => {
    montar();
    fireEvent.click(screen.getByRole("button", { name: "Abrir asistente" }));
    expect(document.activeElement).toBe(screen.getByLabelText("Tu pregunta"));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Abrir asistente" }),
    );
  });

  it("envia la pregunta y pinta la respuesta que llega por el flujo", async () => {
    vi.mocked(fetch).mockResolvedValue(
      respuestaSSE(
        'event: answer\ndata: {"texto":"Una ONI es una obra no identificada.","parcial":false,"restringida":false}\n\n',
      ),
    );
    montar();
    fireEvent.click(screen.getByRole("button", { name: "Abrir asistente" }));
    preguntar("que es una ONI?");

    expect(
      await screen.findByText("Una ONI es una obra no identificada."),
    ).toBeTruthy();
    expect(screen.getByText("que es una ONI?")).toBeTruthy();
    const [ruta, init] = vi.mocked(fetch).mock.calls[0];
    expect(ruta).toBe("/api/agente/consulta");
    expect(JSON.parse(String(init?.body))).toEqual({
      mensaje: "que es una ONI?",
      historial: [],
    });
  });

  it("reenvia la conversacion previa como historial", async () => {
    vi.mocked(fetch).mockImplementation(() =>
      Promise.resolve(
        respuestaSSE(
          'event: answer\ndata: {"texto":"r","parcial":false,"restringida":false}\n\n',
        ),
      ),
    );
    montar();
    fireEvent.click(screen.getByRole("button", { name: "Abrir asistente" }));
    preguntar("uno");
    await screen.findByText("r");
    preguntar("dos");
    await waitFor(() => expect(vi.mocked(fetch).mock.calls).toHaveLength(2));
    const cuerpo = JSON.parse(String(vi.mocked(fetch).mock.calls[1][1]?.body));
    expect(cuerpo.historial).toEqual([
      { rol: "usuario", texto: "uno" },
      { rol: "asistente", texto: "r" },
    ]);
  });

  it("un fallo de la peticion se muestra en linea, sin colgar el panel", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response(
        JSON.stringify({
          error: "demasiadas solicitudes, reintente en un momento",
        }),
        {
          status: 429,
          headers: { "content-type": "application/json" },
        },
      ),
    );
    montar();
    fireEvent.click(screen.getByRole("button", { name: "Abrir asistente" }));
    preguntar("hola");
    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain("demasiadas solicitudes");
    expect(
      (screen.getByLabelText("Tu pregunta") as HTMLTextAreaElement).disabled,
    ).toBe(false);
  });

  it("conserva la conversacion al cerrar y reabrir", async () => {
    vi.mocked(fetch).mockResolvedValue(
      respuestaSSE(
        'event: answer\ndata: {"texto":"guardado en memoria","parcial":false,"restringida":false}\n\n',
      ),
    );
    montar();
    fireEvent.click(screen.getByRole("button", { name: "Abrir asistente" }));
    preguntar("hola");
    await screen.findByText("guardado en memoria");
    fireEvent.click(screen.getByRole("button", { name: "Cerrar asistente" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Abrir asistente" }));
    expect(screen.getByText("guardado en memoria")).toBeTruthy();
  });
});
