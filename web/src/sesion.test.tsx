import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import AUTENTICACION from "../../internal/aplicacion/autenticacion.go?raw";
import VARIABLES_API from "../../infra/modules/api/variables.tf?raw";
import { ProveedorDeSesion, SESION_INACTIVIDAD_MS, useSesion } from "./sesion";
import { setToken } from "./api";

function Sonda() {
  const { usuario, cargando } = useSesion();
  if (cargando) return <p>cargando</p>;
  return <p>{usuario ? `hola ${usuario.nombre}` : "sin sesion"}</p>;
}

function montar() {
  return render(
    <MemoryRouter>
      <ProveedorDeSesion>
        <Sonda />
      </ProveedorDeSesion>
    </MemoryRouter>,
  );
}

function respuestaUsuario(nombre: string) {
  return new Response(
    JSON.stringify({
      id: "usr-1",
      email: "x@redes.co",
      nombre,
      rol: "administrador",
      titular_id: "",
    }),
    { headers: { "content-type": "application/json" } },
  );
}

/** Monta, espera a que resuelva la sesion, pulsa salir y espera a que termine. */
async function montarYSalir() {
  let termino = false;

  function ConBotonDeSalida() {
    const { salir, cargando } = useSesion();
    if (cargando) return <p>cargando</p>;
    return (
      <button
        onClick={() => {
          void salir().then(() => (termino = true));
        }}
      >
        salir
      </button>
    );
  }

  render(
    <MemoryRouter>
      <ProveedorDeSesion>
        <ConBotonDeSalida />
      </ProveedorDeSesion>
    </MemoryRouter>,
  );

  await waitFor(() => expect(screen.getByText("salir")).toBeTruthy());
  fireEvent.click(screen.getByText("salir"));
  await waitFor(() => expect(termino).toBe(true));
}

describe("ProveedorDeSesion", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("sin token guardado, termina de cargar sin sesion y sin llamar a la API", async () => {
    montar();

    await waitFor(() => expect(screen.getByText("sin sesion")).toBeTruthy());
    expect(fetch).not.toHaveBeenCalled();
  });

  it("con token guardado, resuelve la sesion contra GET /auth/session", async () => {
    setToken("token-valido");
    vi.mocked(fetch).mockResolvedValue(
      new Response(
        JSON.stringify({
          id: "usr-1",
          email: "ana@redes.co",
          nombre: "Ana Escritora",
          rol: "titular",
          titular_id: "tit-ana",
        }),
        { headers: { "content-type": "application/json" } },
      ),
    );

    montar();

    await waitFor(() =>
      expect(screen.getByText("hola Ana Escritora")).toBeTruthy(),
    );
  });

  it("con un token que la API ya no reconoce, termina sin sesion en vez de romper", async () => {
    setToken("token-vencido");
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ error: "sesion invalida o expirada" }), {
        status: 401,
      }),
    );

    montar();

    await waitFor(() => expect(screen.getByText("sin sesion")).toBeTruthy());
  });

  it("si otra pestana cierra sesion, esta se entera y deja de mostrar al usuario", async () => {
    setToken("token-valido");
    vi.mocked(fetch).mockResolvedValue(respuestaUsuario("Ana Escritora"));

    montar();
    await waitFor(() =>
      expect(screen.getByText("hola Ana Escritora")).toBeTruthy(),
    );

    // El evento `storage` es lo que el navegador dispara en las OTRAS pestanas.
    act(() => {
      localStorage.removeItem("intela.token");
      window.dispatchEvent(
        new StorageEvent("storage", { key: "intela.token", newValue: null }),
      );
    });

    await waitFor(() => expect(screen.getByText("sin sesion")).toBeTruthy());
  });

  it("si otra pestana entra con otra cuenta, esta reresuelve y no muestra al usuario viejo", async () => {
    setToken("token-de-ana");
    vi.mocked(fetch).mockResolvedValueOnce(respuestaUsuario("Ana Escritora"));

    montar();
    await waitFor(() =>
      expect(screen.getByText("hola Ana Escritora")).toBeTruthy(),
    );

    // Otra pestana inicia sesion como otro usuario: el token cambia.
    vi.mocked(fetch).mockResolvedValueOnce(respuestaUsuario("Admin Intela"));
    act(() => {
      localStorage.setItem("intela.token", "token-de-admin");
      window.dispatchEvent(
        new StorageEvent("storage", {
          key: "intela.token",
          newValue: "token-de-admin",
        }),
      );
    });

    await waitFor(() =>
      expect(screen.getByText("hola Admin Intela")).toBeTruthy(),
    );
  });

  it("ignora cambios de otras claves del almacen", async () => {
    setToken("token-valido");
    vi.mocked(fetch).mockResolvedValue(respuestaUsuario("Ana Escritora"));

    montar();
    await waitFor(() =>
      expect(screen.getByText("hola Ana Escritora")).toBeTruthy(),
    );

    act(() => {
      window.dispatchEvent(
        new StorageEvent("storage", { key: "otra.cosa", newValue: "x" }),
      );
    });

    expect(screen.getByText("hola Ana Escritora")).toBeTruthy();
  });
});

describe("salir()", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("pide la revocacion al servidor y limpia el token", async () => {
    setToken("tok");
    vi.mocked(fetch).mockResolvedValueOnce(respuestaUsuario("Admin"));
    vi.mocked(fetch).mockResolvedValueOnce(new Response(null, { status: 204 }));

    await montarYSalir();

    const [ruta, init] = vi.mocked(fetch).mock.calls[1];
    expect(String(ruta)).toContain("/api/auth/session");
    expect(init?.method).toBe("DELETE");
    expect(localStorage.getItem("intela.token")).toBeNull();
  });
});

// ADR 0025: el DELETE se reintenta dos veces (300 y 800 ms) ANTES de borrar el token local.
describe("salir() con reintentos", () => {
  let salir: () => Promise<void> = () => Promise.resolve();

  function Captura() {
    salir = useSesion().salir;
    return null;
  }

  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    // Sin token al montar: no hay GET /auth/session y cada llamada a fetch es un DELETE.
    render(
      <MemoryRouter>
        <ProveedorDeSesion>
          <Captura />
        </ProveedorDeSesion>
      </MemoryRouter>,
    );
    setToken("tok");
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  async function salirYEsperar() {
    let termino = false;
    const enCurso = act(() => salir().then(() => void (termino = true)));
    await vi.advanceTimersByTimeAsync(2000);
    await enCurso;
    expect(termino).toBe(true);
  }

  it("un fallo de red seguido de exito: dos DELETE y el token fuera", async () => {
    vi.mocked(fetch)
      .mockRejectedValueOnce(new TypeError("Failed to fetch"))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));

    await salirYEsperar();

    expect(fetch).toHaveBeenCalledTimes(2);
    for (const [, init] of vi.mocked(fetch).mock.calls) {
      expect(init?.method).toBe("DELETE");
    }
    expect(localStorage.getItem("intela.token")).toBeNull();
  });

  it("reintenta con el token todavia guardado", async () => {
    const tokenEnCadaIntento: (string | null)[] = [];
    vi.mocked(fetch).mockImplementation(() => {
      tokenEnCadaIntento.push(localStorage.getItem("intela.token"));
      return Promise.reject(new TypeError("Failed to fetch"));
    });

    await salirYEsperar();

    expect(tokenEnCadaIntento).toEqual(["tok", "tok", "tok"]);
  });

  it("tres fallos: tres DELETE, sin aviso, y el token fuera igual", async () => {
    vi.mocked(fetch)
      .mockRejectedValueOnce(new TypeError("Failed to fetch"))
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ error: "x" }), { status: 503 }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ error: "x" }), { status: 500 }),
      );

    await salirYEsperar();

    expect(fetch).toHaveBeenCalledTimes(3);
    expect(localStorage.getItem("intela.token")).toBeNull();
  });

  it("espera entre intentos en vez de reintentar de golpe", async () => {
    vi.mocked(fetch).mockRejectedValue(new TypeError("Failed to fetch"));

    const enCurso = act(() => salir());
    await vi.advanceTimersByTimeAsync(0);
    expect(fetch).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(299);
    expect(fetch).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(fetch).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(800);
    expect(fetch).toHaveBeenCalledTimes(3);
    await enCurso;
  });

  it("un 401 ya esta revocado: un solo DELETE", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ error: "sesion invalida" }), {
        status: 401,
      }),
    );

    await salirYEsperar();

    expect(fetch).toHaveBeenCalledTimes(1);
    expect(localStorage.getItem("intela.token")).toBeNull();
  });
});

/**
 * La API no publica su inactividad maxima, asi que el cliente copia el valor por
 * defecto. Estas pruebas leen el Go y el Terraform (como `?raw`, igual que el
 * contrato en `identificacion/contrato.test.ts`) para que un cambio en un lado
 * sin el otro falle aqui y no deje el sondeo de Distribucion cortando a
 * destiempo (#223). En AWS manda el Terraform: pasa `SESION_INACTIVIDAD`
 * explicito y el valor por defecto del Go no llega a usarse.
 *
 * Del Go basta `aplicacion.InactividadPorDefecto`: que `cmd/api` y `cmd/lambda`
 * la usen lo vigila `cmd/lambda/cableado_test.go`.
 *
 * Se exige igualdad aunque solo un sentido es peligroso: con el cliente por
 * encima del servidor, el sondeo sostiene la sesion mas de lo configurado. Por
 * debajo solo corta antes, pero dos valores para lo mismo confunden.
 */
describe("SESION_INACTIVIDAD_MS", () => {
  const MS_POR_UNIDAD: Record<string, number> = {
    Second: 1000,
    Minute: 60_000,
    Hour: 3_600_000,
  };

  it("es aplicacion.InactividadPorDefecto", () => {
    const m =
      /const InactividadPorDefecto = (\d+) \* time\.(Second|Minute|Hour)\b/.exec(
        AUTENTICACION,
      );
    expect(m && Number(m[1]) * MS_POR_UNIDAD[m[2]]).toBe(SESION_INACTIVIDAD_MS);
  });

  /** `"30m"`, `"1h"` o `"1h30m"` (sintaxis de duracion de Go) en ms; NaN si no. */
  function duracionTexto(texto: string): number {
    const MS: Record<string, number> = { h: 3_600_000, m: 60_000, s: 1000 };
    const partes = [...texto.matchAll(/(\d+)([hms])/g)];
    if (partes.map(([p]) => p).join("") !== texto) return NaN;
    return partes.reduce((ms, [, n, u]) => ms + Number(n) * MS[u], 0);
  }

  it("es el valor por defecto de session_idle_timeout en infra/modules/api", () => {
    const m =
      /variable "session_idle_timeout" \{[^}]*?default\s*=\s*"([^"]+)"/.exec(
        VARIABLES_API,
      );
    expect(m && duracionTexto(m[1])).toBe(SESION_INACTIVIDAD_MS);
  });
});
