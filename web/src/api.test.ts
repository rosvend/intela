import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ApiError,
  ErrorDeRed,
  api,
  nombreDeContentDisposition,
  setToken,
  setUnauthorizedHandler,
} from "./api";

// La pagina de error de nginx: la contestan el 502, el 504 y el 408 de
// `client_body_timeout` (deploy/nginx.conf). No es un mensaje de la API.
const PAGINA_DEL_PROXY =
  "<html><head><title>409 Conflict</title></head><body><h1>nginx</h1></body></html>";

describe("api", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    localStorage.clear();
    setUnauthorizedHandler(null);
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

  it("no adjunta el token en una llamada anonima", async () => {
    setToken("token-de-prueba");
    vi.mocked(fetch).mockResolvedValue(
      new Response("{}", { headers: { "content-type": "application/json" } }),
    );

    await api("/api/auth/session", { method: "POST", anonima: true });

    const [, init] = vi.mocked(fetch).mock.calls[0];
    expect(new Headers(init?.headers).get("Authorization")).toBeNull();
  });

  it("declara el cuerpo como JSON cuando se envia uno", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response("{}", {
        headers: { "content-type": "application/json" },
      }),
    );

    await api("/api/obras", {
      method: "POST",
      body: JSON.stringify({ a: 1 }),
    });

    const [, init] = vi.mocked(fetch).mock.calls[0];
    expect(new Headers(init?.headers).get("Content-Type")).toBe(
      "application/json",
    );
  });

  it("CONSERVA el token ante un 401 anonimo: un login fallido no cierra la sesion vigente", async () => {
    // Una llamada anonima no envia token, asi que su 401 no dice nada sobre la
    // sesion guardada. Quien tenga sesion abierta, entre a /login y escriba mal
    // la clave, no debe perderla.
    setToken("token-vigente");
    const handler = vi.fn();
    setUnauthorizedHandler(handler);
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ error: "credenciales invalidas" }), {
        status: 401,
      }),
    );

    await expect(
      api("/api/auth/session", { method: "POST", anonima: true }),
    ).rejects.toThrow("credenciales invalidas");
    expect(localStorage.getItem("intela.token")).toBe("token-vigente");
    expect(handler).not.toHaveBeenCalled();
  });

  it("limpia el token Y llama al handler ante un 401 con sesion (token vencido)", async () => {
    setToken("token-vencido");
    const handler = vi.fn();
    setUnauthorizedHandler(handler);
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ error: "sesion invalida o expirada" }), {
        status: 401,
      }),
    );

    await expect(api("/api/auth/session")).rejects.toThrow();
    expect(localStorage.getItem("intela.token")).toBeNull();
    expect(handler).toHaveBeenCalledOnce();
  });

  it("sin handler registrado, un 401 con sesion redirige por window.location", async () => {
    setToken("token-vencido");
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ error: "sesion invalida o expirada" }), {
        status: 401,
      }),
    );
    const ubicacion = { href: "" };
    vi.stubGlobal("location", ubicacion);

    await expect(api("/api/auth/session")).rejects.toThrow();
    expect(ubicacion.href).toBe("/login");
  });

  it("lanza ApiError con el status para distinguir 403 de 500", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ error: "no autorizado" }), {
        status: 403,
      }),
    );

    const error = await api("/api/obras").catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).status).toBe(403);
    expect((error as ApiError).message).toBe("no autorizado");
  });

  it("el mensaje de error sale de {error: ...}, no del JSON crudo", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ error: "base de datos no disponible" }), {
        status: 500,
      }),
    );

    await expect(api("/api/ready")).rejects.toThrow(
      "base de datos no disponible",
    );
  });

  it("un cuerpo de error vacio (como el 204 de DELETE seguido de reintento) usa el statusText", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response("", { status: 500, statusText: "Internal Server Error" }),
    );

    await expect(api("/api/obras")).rejects.toThrow("Internal Server Error");
  });

  it("un cuerpo de error que parsea pero no trae `error` no se pinta crudo", async () => {
    // El contrato promete `{error: "..."}`. Un cuerpo que si parsea como JSON
    // pero nombra el mensaje de otra manera -`{"detalle": ...}`, la forma de
    // tantos proxies- no es un mensaje de esta API: devolverlo tal cual pintaba
    // el JSON entero, con sus llaves y comillas, como explicacion del sistema.
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ detalle: "algo del proxy" }), {
        status: 500,
        headers: { "content-type": "application/json" },
      }),
    );

    const error = await api("/api/reportes").catch((e: unknown) => e);

    expect((error as ApiError).message).toBe(
      "el servidor respondió un error ilegible",
    );
    expect((error as ApiError).message).not.toContain("detalle");
    expect((error as ApiError).message).not.toContain("{");
  });

  it("un `error` vacio tambien es ilegible: una cadena vacia no es un mensaje", async () => {
    // `cuerpo.error || texto` caia al `||` con `""` -que es falsy- y acababa
    // pintando el JSON crudo.
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ error: "" }), {
        status: 500,
        headers: { "content-type": "application/json" },
      }),
    );

    const error = await api("/api/reportes").catch((e: unknown) => e);

    expect((error as ApiError).message).toBe(
      "el servidor respondió un error ilegible",
    );
    // Ni el JSON crudo: el mensaje propio no lleva llaves ni comillas.
    expect((error as ApiError).message).not.toContain('"error"');
  });

  it("un cuerpo de error que no es JSON se sustituye: la pagina del proxy no es un mensaje del backend", async () => {
    // El contrato promete que un error de la API viene como `{error: "..."}`,
    // asi que un cuerpo que no parsea como JSON no lo puso la API. Devuelto
    // crudo, el HTML de nginx acababa pintado como explicacion del sistema
    // (escapado, pero entero) en cuanto el status no fuera 502/504: el 408 de
    // `client_body_timeout` es el caso realista.
    vi.mocked(fetch).mockResolvedValue(
      new Response(PAGINA_DEL_PROXY, {
        status: 409,
        headers: { "content-type": "text/html" },
      }),
    );

    const error = await api("/api/reportes").catch((e: unknown) => e);

    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).status).toBe(409);
    expect((error as ApiError).message).toBe(
      "el servidor respondió un error ilegible",
    );
    // Ni una etiqueta ni el titulo de la pagina del proxy.
    expect((error as ApiError).message).not.toContain("nginx");
    expect((error as ApiError).message).not.toContain("<");
  });

  it("un fetch que rechaza produce ErrorDeRed, no una excepcion sin tipar", async () => {
    vi.mocked(fetch).mockRejectedValue(new TypeError("Failed to fetch"));

    const error = await api("/api/ready").catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ErrorDeRed);
  });

  it("no llama a res.json() en una respuesta 204 sin content-type (DELETE de sesion)", async () => {
    vi.mocked(fetch).mockResolvedValue(new Response(null, { status: 204 }));

    await expect(
      api("/api/auth/session", { method: "DELETE" }),
    ).resolves.not.toThrow();
  });

  it("extrae el filename de Content-Disposition", () => {
    expect(
      nombreDeContentDisposition(
        'attachment; filename="liquidacion-2026-01.pdf"',
      ),
    ).toBe("liquidacion-2026-01.pdf");
  });
});
