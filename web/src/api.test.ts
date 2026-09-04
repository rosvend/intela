import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, apiPublica, setToken } from "./api";

describe("api", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("adjunta el token como cabecera Bearer", async () => {
    setToken("token-de-prueba");
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ estado: "ok" }), {
        headers: { "content-type": "application/json" },
      }),
    );

    await api("/api/ready");

    const [, init] = vi.mocked(fetch).mock.calls[0];
    const headers = new Headers(init?.headers);
    expect(headers.get("Authorization")).toBe("Bearer token-de-prueba");
  });

  it("declara el cuerpo como JSON cuando se envia uno", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response("{}", {
        headers: { "content-type": "application/json" },
      }),
    );

    await api("/api/obras", { method: "POST", body: JSON.stringify({ a: 1 }) });

    const [, init] = vi.mocked(fetch).mock.calls[0];
    expect(new Headers(init?.headers).get("Content-Type")).toBe(
      "application/json",
    );
  });

  it("limpia el token guardado ante un 401 del propio login", async () => {
    setToken("token-vencido");
    vi.mocked(fetch).mockResolvedValue(
      new Response("credenciales invalidas", { status: 401 }),
    );

    await expect(api("/api/sesiones", { method: "POST" })).rejects.toThrow();
    expect(localStorage.getItem("intela.token")).toBeNull();
  });

  it("apiPublica no adjunta token ni redirige ante 404", async () => {
    setToken("token-de-prueba");
    vi.mocked(fetch).mockResolvedValue(
      new Response("no hay listado", { status: 404 }),
    );

    await expect(apiPublica("/api/publico/oni")).resolves.toBeNull();

    const [, init] = vi.mocked(fetch).mock.calls[0];
    expect(init?.headers).toBeUndefined();
    expect(localStorage.getItem("intela.token")).toBe("token-de-prueba");
  });
});
