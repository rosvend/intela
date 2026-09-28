import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, ErrorDeCuerpoIlegible, ErrorDeRed } from "./api";
import { useApi } from "./useApi";

describe("useApi", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("empieza cargando y termina con los datos de la respuesta", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ estado: "listo" }), {
        headers: { "content-type": "application/json" },
      }),
    );

    const { result } = renderHook(() =>
      useApi<{ estado: string }>("/api/ready"),
    );

    expect(result.current.cargando).toBe(true);

    await waitFor(() => expect(result.current.cargando).toBe(false));
    expect(result.current.datos).toEqual({ estado: "listo" });
    expect(result.current.error).toBeNull();
  });

  it("termina con un error tipado si la API responde con un estado de error", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ error: "no autorizado" }), {
        status: 403,
      }),
    );

    const { result } = renderHook(() => useApi("/api/obras"));

    await waitFor(() => expect(result.current.cargando).toBe(false));
    expect(result.current.datos).toBeNull();
    expect(result.current.error?.message).toBe("no autorizado");
  });

  it("un 2xx que no trae JSON queda como error, nunca como datos", async () => {
    // `api()` devuelve el `Response` crudo cuando el content-type no es JSON.
    // Entregarlo como `datos` es lo que tumbaba al listado de ingesta: quien
    // pidio `Carga[]` recibia un `Response` y reventaba al pedirle `.length` o
    // `.map`. El hook es la unica frontera que puede notar la diferencia.
    vi.mocked(fetch).mockResolvedValue(
      new Response("<html><body>sin API</body></html>", {
        status: 200,
        headers: { "content-type": "text/html" },
      }),
    );

    const { result } = renderHook(() =>
      useApi<{ estado: string }[]>("/api/reportes"),
    );

    await waitFor(() => expect(result.current.cargando).toBe(false));
    expect(result.current.datos).toBeNull();
    expect(result.current.error?.message).toBe("la respuesta no vino en JSON");
  });

  it("un 2xx con el cuerpo ilegible llega con el mensaje del error, no con el generico", async () => {
    // Con `content-type` de JSON y un cuerpo que no parsea, `api()` lanza
    // `ErrorDeCuerpoIlegible`: el servidor **contesto**, y contesto sin error
    // -es un 2xx-; lo que no llego fue el cuerpo. Eso es algo que el texto
    // generico ("error desconocido") no puede decir, asi que la union de
    // errores tipados tiene que incluir la clase.
    const cuerpoIlegible = '{"total": 12';
    const respuesta = new Response(cuerpoIlegible, {
      status: 200,
      headers: { "content-type": "application/json" },
    });
    vi.mocked(fetch).mockResolvedValue(respuesta);

    const { result } = renderHook(() => useApi("/api/tablero/oni"));

    await waitFor(() => expect(result.current.cargando).toBe(false));

    // Precondicion: el doble devolvio un 2xx con un cuerpo que no parsea. Un
    // doble que contestara un 500 mediria el camino del `ApiError` y la prueba
    // pasaria afirmando lo mismo por el motivo equivocado.
    expect(respuesta.ok).toBe(true);
    expect(respuesta.status).toBe(200);
    expect(respuesta.headers.get("content-type")).toContain("json");
    await expect(
      new Response(cuerpoIlegible, {
        headers: { "content-type": "application/json" },
      }).json(),
    ).rejects.toThrow();

    const error = result.current.error;
    expect(error).toBeInstanceOf(ErrorDeCuerpoIlegible);
    expect(error?.message).toBe("la respuesta llegó sin un cuerpo legible");
    expect((error as ErrorDeCuerpoIlegible).status).toBe(200);
  });

  it("control negativo: un ApiError sigue llegando con su mensaje, no con el generico", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ error: "la base esta caida" }), {
        status: 500,
        headers: { "content-type": "application/json" },
      }),
    );

    const { result } = renderHook(() => useApi("/api/tablero/oni"));

    await waitFor(() => expect(result.current.cargando).toBe(false));

    const error = result.current.error;
    expect(error).toBeInstanceOf(ApiError);
    expect(error?.message).toBe("la base esta caida");
    expect((error as ApiError).status).toBe(500);
  });

  it("control negativo: un fallo de red sigue llegando con su mensaje, no con el generico", async () => {
    vi.mocked(fetch).mockRejectedValue(new TypeError("Failed to fetch"));

    const { result } = renderHook(() => useApi("/api/tablero/oni"));

    await waitFor(() => expect(result.current.cargando).toBe(false));

    const error = result.current.error;
    expect(error).toBeInstanceOf(ErrorDeRed);
    expect(error?.message).toBe("no se pudo contactar al servidor");
  });

  it("vuelve a pedir los datos cuando cambia el path", async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ id: "1" }), {
          headers: { "content-type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ id: "2" }), {
          headers: { "content-type": "application/json" },
        }),
      );

    const { result, rerender } = renderHook(
      ({ path }) => useApi<{ id: string }>(path),
      { initialProps: { path: "/api/obras/1" } },
    );

    await waitFor(() => expect(result.current.datos).toEqual({ id: "1" }));

    act(() => rerender({ path: "/api/obras/2" }));

    await waitFor(() => expect(result.current.datos).toEqual({ id: "2" }));
    expect(fetch).toHaveBeenCalledTimes(2);
  });
});

describe("useApi: tecleo y cancelacion (item 11)", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    // Los temporizadores falsos son de estas dos pruebas: dejarlos puestos
    // rompe cualquier prueba posterior del fichero.
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("al cambiar de ruta aborta la peticion anterior, sin dejar la pantalla cargando ni un error inventado", async () => {
    const senales: AbortSignal[] = [];
    const enVuelo: ((respuesta: Response) => void)[] = [];
    vi.mocked(fetch).mockImplementation((_entrada, init) => {
      const signal = init?.signal ?? undefined;
      if (signal) senales.push(signal);
      return new Promise<Response>((resolver, rechazar) => {
        // Un doble que RESPETA la señal: cancelada la peticion, rechaza como
        // rechazaria `fetch`. Uno que la ignorara dejaria esta prueba pasando
        // sin haber probado la cancelacion -la respuesta llegaria tarde y el
        // hook la descartaria con la bandera, que es justo lo que no basta-.
        signal?.addEventListener("abort", () =>
          rechazar(new DOMException("la peticion se cancelo", "AbortError")),
        );
        enVuelo.push(resolver);
      });
    });

    const { result, rerender } = renderHook(
      ({ path }) => useApi<{ id: string }>(path),
      { initialProps: { path: "/api/obras/1" } },
    );

    // Precondicion: la peticion salio, con una señal, y sigue en vuelo. Sin
    // esto, el `aborted` de abajo podria ser el de una peticion que nunca se
    // hizo.
    expect(senales).toHaveLength(1);
    expect(senales[0].aborted).toBe(false);

    act(() => rerender({ path: "/api/obras/2" }));

    // Cada peticion trae su propio control: la vieja se cancela, la nueva no.
    expect(senales[0].aborted).toBe(true);
    expect(senales).toHaveLength(2);
    expect(senales[1].aborted).toBe(false);
    // Y el aborto no se convierte en nada: ni error, ni un `cargando` que se
    // quede puesto -el modo de fallo que la adenda del paso avisa-.
    expect(result.current.error).toBeNull();
    expect(result.current.cargando).toBe(true);

    await act(async () => {
      enVuelo[1](
        new Response(JSON.stringify({ id: "2" }), {
          headers: { "content-type": "application/json" },
        }),
      );
    });

    expect(result.current.datos).toEqual({ id: "2" });
    expect(result.current.cargando).toBe(false);
    expect(result.current.error).toBeNull();
  });
});
