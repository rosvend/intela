import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, apiStream, setToken, setUnauthorizedHandler } from "./api";

/** Un cuerpo SSE partido en trozos arbitrarios, como llega por la red. */
function respuestaSSE(trozos: string[], status = 200): Response {
  const cod = new TextEncoder();
  const cuerpo = new ReadableStream<Uint8Array>({
    start(c) {
      for (const t of trozos) c.enqueue(cod.encode(t));
      c.close();
    },
  });
  return new Response(cuerpo, {
    status,
    headers: { "content-type": "text/event-stream; charset=utf-8" },
  });
}

describe("apiStream", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    localStorage.clear();
    setUnauthorizedHandler(null);
  });

  it("envia POST con Bearer y entrega cada evento en orden aunque llegue partido", async () => {
    setToken("tok");
    vi.mocked(fetch).mockResolvedValue(
      respuestaSSE([
        'event: tool_call\ndata: {"herramien',
        'ta":"eco"}\n\nevent: answer\n',
        'data: {"texto":"hola","parcial":false,"restringida":false}\n\n',
      ]),
    );
    const eventos: [string, unknown][] = [];

    await apiStream("/api/agente/consulta", { mensaje: "x" }, (n, d) =>
      eventos.push([n, d]),
    );

    const [ruta, init] = vi.mocked(fetch).mock.calls[0];
    expect(ruta).toBe("/api/agente/consulta");
    expect(init?.method).toBe("POST");
    expect(new Headers(init?.headers).get("Authorization")).toBe("Bearer tok");
    expect(JSON.parse(String(init?.body))).toEqual({ mensaje: "x" });
    expect(eventos).toEqual([
      ["tool_call", { herramienta: "eco" }],
      ["answer", { texto: "hola", parcial: false, restringida: false }],
    ]);
  });

  it("reconoce un CRLF partido entre dos trozos como fin de linea", async () => {
    vi.mocked(fetch).mockResolvedValue(
      respuestaSSE([
        'event: tool_call\r\ndata: {"herramienta":"eco"}\r\n\r',
        '\nevent: answer\r\ndata: {"texto":"hola"}\r',
        "\n\r\n",
      ]),
    );
    const eventos: [string, unknown][] = [];

    await apiStream("/api/agente/consulta", { mensaje: "x" }, (n, d) =>
      eventos.push([n, d]),
    );

    expect(eventos).toEqual([
      ["tool_call", { herramienta: "eco" }],
      ["answer", { texto: "hola" }],
    ]);
  });

  it("un error HTTP antes del flujo sube como ApiError con el mensaje del servidor", async () => {
    setToken("tok");
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ error: "demasiadas solicitudes" }), {
        status: 429,
        headers: { "content-type": "application/json" },
      }),
    );
    await expect(
      apiStream("/api/agente/consulta", { mensaje: "x" }, () => {}),
    ).rejects.toEqual(new ApiError(429, "demasiadas solicitudes"));
  });

  it("ignora bloques con data que no es JSON en vez de romper el flujo", async () => {
    vi.mocked(fetch).mockResolvedValue(
      respuestaSSE([
        'event: answer\ndata: {roto\n\nevent: error\ndata: {"mensaje":"m"}\n\n',
      ]),
    );
    const eventos: string[] = [];
    await apiStream("/x", {}, (n) => eventos.push(n));
    expect(eventos).toEqual(["error"]);
  });
});
