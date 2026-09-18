import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "../App";
import { setToken } from "../api";
import { ProveedorDeSesion, type Rol } from "../sesion";
import ListaCargas, { type Carga } from "./ListaCargas";
import type { Entrega } from "./PanelResultado";
import type { Rechazo } from "./TablaRechazos";

function json(cuerpo: unknown, status = 200): Response {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

/** El path de la llamada `n` a fetch (0 = la primera). */
function pedido(n: number): unknown {
  return vi.mocked(fetch).mock.calls[n]?.[0];
}

/** La fila del listado que contiene `texto`. */
function filaCon(texto: string): HTMLElement {
  const fila = screen.getByText(texto).closest("tr");
  if (!fila) throw new Error(`"${texto}" no esta dentro de una fila`);
  return fila;
}

// Fixtures tipados con el contrato generado: si `Carga` o `Rechazo` cambian
// en api/openapi.yaml, `tsc` rompe aqui antes que en pantalla. Datos
// sinteticos, no de ningun canal real.
const SHA_CARACOL =
  "9f2c4e1a7b3d5f60a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718";
const SHA_NETFLIX =
  "7a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f607189f2c4e1a7b3d5f6";

const cargaSinRechazos = {
  id: "rep-9f2c4e1a",
  fuente: "caracol",
  periodo: "2026-01",
  sha256: SHA_CARACOL,
  clave_objeto: "reportes/9f2c4e1a",
  nbytes: 21032,
  recibido: "2026-02-03T14:05:00Z",
  aceptados: 1234,
  rechazados: 0,
} satisfies Carga;

const cargaConRechazos = {
  id: "rep-7a1b2c3d",
  fuente: "netflix",
  periodo: "2026-01",
  sha256: SHA_NETFLIX,
  clave_objeto: "reportes/7a1b2c3d",
  nbytes: 4096,
  recibido: "2026-02-04T09:30:00Z",
  aceptados: 58,
  rechazados: 3,
} satisfies Carga;

const rechazos = [
  {
    id: "rep-7a1b2c3d-2",
    titulo: "Obra sintetica A",
    ids_fuente: "id_netflix=80000001",
    motivo: 'fila 2, titulo (columna "Title"): vacio',
  },
  {
    id: "rep-7a1b2c3d-5",
    titulo: "Obra sintetica B",
    ids_fuente: "id_netflix=80000002",
    motivo:
      'fila 5, horas_vistas (columna "Hours Viewed"): "muchas" no es un numero',
  },
  {
    id: "rep-7a1b2c3d-11",
    titulo: "Obra sintetica C",
    ids_fuente: "id_netflix=80000003",
    motivo: 'fila 11, titulo (columna "Title"): vacio',
  },
] satisfies Rechazo[];

describe("ListaCargas", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("pide las cargas del periodo en la query", async () => {
    vi.mocked(fetch).mockResolvedValue(json([]));

    render(<ListaCargas periodo="2026-01" />);

    await screen.findByText(
      "No hay cargas registradas para el periodo 2026-01.",
    );
    expect(pedido(0)).toBe("/api/reportes?periodo=2026-01");
  });

  it("codifica el periodo para que no parta la query", async () => {
    vi.mocked(fetch).mockResolvedValue(json([]));

    render(<ListaCargas periodo="2026-01&fuente=x" />);

    await screen.findByText(/No hay cargas registradas/);
    expect(pedido(0)).toBe("/api/reportes?periodo=2026-01%26fuente%3Dx");
  });

  it("sin periodo pide todas las cargas y lo dice si no hay ninguna", async () => {
    vi.mocked(fetch).mockResolvedValue(json([]));

    render(<ListaCargas periodo="" />);

    await screen.findByText("Aún no hay cargas registradas.");
    expect(pedido(0)).toBe("/api/reportes");
  });

  it("mientras llega la respuesta muestra que esta cargando", () => {
    vi.mocked(fetch).mockReturnValue(new Promise<Response>(() => {}));

    render(<ListaCargas periodo="2026-01" />);

    expect(screen.getByRole("status")).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("un error del backend se muestra con su mensaje", async () => {
    const mensaje =
      'reporte invalido: periodo "2026-1", se esperaba AAAA o AAAA-MM';
    vi.mocked(fetch).mockResolvedValue(json({ error: mensaje }, 400));

    render(<ListaCargas periodo="2026-1" />);

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain(mensaje);
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("cada carga muestra fuente, periodo, huella corta, recuentos y fecha", async () => {
    vi.mocked(fetch).mockResolvedValue(
      json([cargaSinRechazos, cargaConRechazos]),
    );

    render(<ListaCargas periodo="2026-01" />);

    const tabla = await screen.findByRole("table", { name: "Cargas hechas" });
    const encabezados = within(tabla)
      .getAllByRole("columnheader")
      .map((th) => th.textContent);
    expect(encabezados).toEqual([
      "Recibido",
      "Fuente",
      "Periodo",
      "Huella",
      "Aceptadas",
      "Rechazadas",
      "",
    ]);

    const fila = filaCon("caracol");
    const celdas = within(fila)
      .getAllByRole("cell")
      .map((td) => td.textContent);
    // La fecha depende de la zona horaria del runner: el esperado sale del
    // mismo formato, no de un string fijo.
    const recibido = new Intl.DateTimeFormat("es-CO", {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(cargaSinRechazos.recibido));
    expect(celdas).toEqual([
      recibido,
      "caracol",
      "2026-01",
      SHA_CARACOL.slice(0, 12),
      "1.234",
      "0",
      "",
    ]);
    // La huella completa queda a mano para volver a la evidencia.
    expect(within(fila).getByTitle(SHA_CARACOL).textContent).toBe(
      SHA_CARACOL.slice(0, 12),
    );
    // El instante exacto viaja en el atributo, sin depender de la zona.
    // (`getByText(recibido)` no sirve: es-CO mete espacios finos U+202F que
    // el normalizador colapsa en el DOM pero no en el string buscado.)
    expect(fila.querySelector("time")?.getAttribute("datetime")).toBe(
      cargaSinRechazos.recibido,
    );
  });

  it("una fecha que no se puede leer se muestra tal cual, sin romper el listado", async () => {
    vi.mocked(fetch).mockResolvedValue(
      json([{ ...cargaSinRechazos, recibido: "ayer" }]),
    );

    render(<ListaCargas periodo="2026-01" />);

    await screen.findByRole("table", { name: "Cargas hechas" });
    expect(within(filaCon("caracol")).getByText("ayer")).toBeTruthy();
  });

  it("solo las cargas con rechazos ofrecen verlos, y nada se pide antes de abrir", async () => {
    vi.mocked(fetch).mockResolvedValue(
      json([cargaSinRechazos, cargaConRechazos]),
    );

    render(<ListaCargas periodo="2026-01" />);

    await screen.findByRole("table", { name: "Cargas hechas" });
    expect(within(filaCon("caracol")).queryByRole("button")).toBeNull();

    const boton = within(filaCon("netflix")).getByRole("button", {
      name: "Ver rechazos (3)",
    });
    expect(boton.getAttribute("aria-expanded")).toBe("false");
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("al abrir pide el log de esa carga y lo pinta entero; al cerrar lo oculta", async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(json([cargaSinRechazos, cargaConRechazos]))
      .mockResolvedValueOnce(json(rechazos));

    render(<ListaCargas periodo="2026-01" />);

    await screen.findByRole("table", { name: "Cargas hechas" });
    fireEvent.click(screen.getByRole("button", { name: "Ver rechazos (3)" }));

    const log = await screen.findByRole("table", { name: "Filas rechazadas" });
    expect(pedido(1)).toBe("/api/reportes/rep-7a1b2c3d/rechazos");
    expect(fetch).toHaveBeenCalledTimes(2);
    // Invariante 3: todas las filas del log, ninguna recortada.
    expect(within(log).getAllByRole("row").slice(1)).toHaveLength(3);
    for (const rechazo of rechazos) {
      expect(within(log).getByText(rechazo.motivo)).toBeTruthy();
    }

    const boton = screen.getByRole("button", { name: "Ocultar rechazos" });
    expect(boton.getAttribute("aria-expanded")).toBe("true");
    // aria-controls apunta a la fila que contiene el log.
    const controlada = document.getElementById(
      boton.getAttribute("aria-controls") ?? "",
    );
    expect(controlada?.contains(log)).toBe(true);

    fireEvent.click(boton);

    expect(
      screen.queryByRole("table", { name: "Filas rechazadas" }),
    ).toBeNull();
    expect(
      screen
        .getByRole("button", { name: "Ver rechazos (3)" })
        .getAttribute("aria-expanded"),
    ).toBe("false");
  });

  it("varias cargas pueden tener su log abierto a la vez", async () => {
    const otra = {
      ...cargaConRechazos,
      id: "rep-5e6f7a8b",
      fuente: "cine",
      rechazados: 1,
    } satisfies Carga;
    vi.mocked(fetch)
      .mockResolvedValueOnce(json([cargaConRechazos, otra]))
      .mockResolvedValueOnce(json(rechazos))
      .mockResolvedValueOnce(json([rechazos[0]]));

    render(<ListaCargas periodo="2026-01" />);

    await screen.findByRole("table", { name: "Cargas hechas" });
    fireEvent.click(screen.getByRole("button", { name: "Ver rechazos (3)" }));
    await screen.findByRole("table", { name: "Filas rechazadas" });
    fireEvent.click(screen.getByRole("button", { name: "Ver rechazos (1)" }));

    await vi.waitFor(() =>
      expect(
        screen.getAllByRole("table", { name: "Filas rechazadas" }),
      ).toHaveLength(2),
    );
    expect(pedido(2)).toBe("/api/reportes/rep-5e6f7a8b/rechazos");
  });

  it("si la carga ya no existe, el error queda dentro de su fila", async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(json([cargaSinRechazos, cargaConRechazos]))
      .mockResolvedValueOnce(json({ error: "esa carga no existe" }, 404));

    render(<ListaCargas periodo="2026-01" />);

    await screen.findByRole("table", { name: "Cargas hechas" });
    const boton = screen.getByRole("button", { name: "Ver rechazos (3)" });
    fireEvent.click(boton);

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain("esa carga no existe");
    const fila = document.getElementById(
      boton.getAttribute("aria-controls") ?? "",
    );
    expect(fila?.contains(alerta)).toBe(true);
    // El resto del listado sigue en pie.
    expect(filaCon("caracol")).toBeTruthy();
  });
});

// ---------- pantalla completa, montada dentro de App ----------

/** Sonda de ubicacion: ruta y query, para ver si hubo navegacion y donde vive el periodo. */
function Ubicacion() {
  const { pathname, search } = useLocation();
  return <span data-testid="ubicacion">{pathname + search}</span>;
}

function montarApp(entrada: string) {
  return render(
    <MemoryRouter initialEntries={[entrada]}>
      <ProveedorDeSesion>
        <Ubicacion />
        <App />
      </ProveedorDeSesion>
    </MemoryRouter>,
  );
}

function ubicacion(): string | null {
  return screen.getByTestId("ubicacion").textContent;
}

const esListado = (url: string) =>
  url === "/api/reportes" || url.startsWith("/api/reportes?");

/**
 * Un backend falso que responde por URL y metodo: la sesion con el rol del
 * test, el listado con `cargas(url)` (leidas en cada GET) y la subida con
 * `subida()`. Todo lo demas, 404.
 */
function simularServidor({
  rol,
  cargas = () => [],
  subida,
}: {
  rol: Rol;
  cargas?: (url: string) => Carga[];
  subida?: () => Promise<Response>;
}) {
  vi.mocked(fetch).mockImplementation((entrada, init) => {
    const url = String(entrada);
    const metodo = init?.method ?? "GET";
    if (url === "/api/auth/session") {
      return Promise.resolve(
        json({
          id: "usr-1",
          email: "x@redes.co",
          nombre: "Persona de Prueba",
          rol,
          titular_id: "",
        }),
      );
    }
    if (metodo === "GET" && esListado(url)) {
      return Promise.resolve(json(cargas(url)));
    }
    if (metodo === "POST" && url === "/api/reportes" && subida) {
      return subida();
    }
    return Promise.resolve(json({ error: "ruta no encontrada" }, 404));
  });
}

function llamadas() {
  return vi.mocked(fetch).mock.calls.map(([entrada, init]) => ({
    url: String(entrada),
    metodo: init?.method ?? "GET",
    init,
  }));
}

function getsDelListado(): string[] {
  return llamadas()
    .filter((l) => l.metodo === "GET" && esListado(l.url))
    .map((l) => l.url);
}

function subidas() {
  return llamadas().filter(
    (l) => l.metodo === "POST" && l.url === "/api/reportes",
  );
}

/** Los GET del log de rechazos de alguna carga: uno por cada fila abierta sin cache. */
function logsDeRechazos() {
  return llamadas().filter((l) => l.url.endsWith("/rechazos"));
}

/** El valor que acompana a una etiqueta del resumen de la entrega. */
function dato(etiqueta: string): string | null | undefined {
  return screen.getByText(etiqueta).nextElementSibling?.textContent;
}

const archivoCaracol = () =>
  new File(["a,b"], "caracol.csv", { type: "text/csv" });

function elegirFuente(valor: string) {
  fireEvent.change(screen.getByLabelText("Fuente"), {
    target: { value: valor },
  });
}

function elegirArchivo(archivo: File) {
  fireEvent.change(screen.getByLabelText("Archivo"), {
    target: { files: [archivo] },
  });
}

function escribirPeriodo(valor: string) {
  fireEvent.change(screen.getByLabelText("Periodo de recaudo"), {
    target: { value: valor },
  });
}

// El texto del boton cambia: "Subir a AAAA-MM" cuando nombra el periodo
// destino, "Subir reporte" cuando no hay ningun periodo utilizable y
// "Subiendo…" mientras la subida esta en vuelo. El helper los abarca todos;
// los tests que juzgan el texto lo afirman literal.
const botonSubir = () => screen.getByRole("button", { name: /^Subir/ });

const VACIO_2026_01 = "No hay cargas registradas para el periodo 2026-01.";
const FALTA_PERIODO =
  "Escribe un periodo válido para poder subir el reporte: AAAA, o AAAA-MM con el mes entre 01 y 12.";

// El fallo no clasificable del panel: lo que se ve cuando un 2xx no trae una
// `Entrega` legible. Textos propios de `PanelResultado`, repetidos aqui como
// literales para que un cambio de copy se note.
const TITULO_FALLO_DESCONOCIDO = "No se pudo registrar la entrega";
const MENSAJE_DESCONOCIDO = "error desconocido al subir el archivo";
const PUDO_LLEGAR =
  "La entrega pudo haber llegado al servidor: revisa el listado de cargas antes de volver a subirla.";

// Ejemplo del 400 en api/openapi.yaml: un unico string que nombra columnas.
const MENSAJE_400 =
  'reporte invalido: a la entrega de "caracol" le faltan columnas requeridas: Duracion_total. El archivo trae: Canal, Titulo, ID_Ficha';

const entregaCaracol = {
  id: cargaSinRechazos.id,
  fuente: "caracol",
  periodo: "2026-01",
  sha256: SHA_CARACOL,
  clave_objeto: cargaSinRechazos.clave_objeto,
  nbytes: cargaSinRechazos.nbytes,
  aceptados: 1234,
  rechazados: [
    {
      id: "rep-9f2c4e1a-6",
      titulo: "Obra sintetica D",
      ids_fuente: "id_ficha=F-004",
      motivo: 'fila 6, titulo (columna "Titulo"): vacio',
    },
  ],
} satisfies Entrega;

// La misma entrega, como la devuelve el listado despues de subirla.
const cargaSubida = { ...cargaSinRechazos, rechazados: 1 } satisfies Carga;

// Un 201 que no trae una `Entrega`. El primero es el cuerpo que devuelve un
// proxy, el segundo un JSON con otra forma: los dos llegan a `api()` como un
// resultado valido, y el panel reventaria al leer `rechazados.length`.
const CUERPOS_QUE_NO_SON_ENTREGA: [string, () => Response][] = [
  [
    "HTML",
    () =>
      new Response("<html><body>Bad Gateway</body></html>", {
        status: 201,
        headers: { "content-type": "text/html" },
      }),
  ],
  ["JSON sin rechazados", () => json({ id: "rep-9f2c4e1a" }, 201)],
];

describe("pantalla de ingesta (integracion con App)", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    setToken("tok");
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("el administrador ve el enlace y la pantalla, y el listado pide el periodo de la URL", async () => {
    simularServidor({ rol: "administrador" });

    montarApp("/ingesta?periodo=2026-01");

    await screen.findByRole("heading", { name: "Ingesta de reportes" });
    expect(screen.getByRole("link", { name: "Ingesta" })).toBeTruthy();
    expect(
      screen.queryByText("Esta pantalla llega en un PR posterior."),
    ).toBeNull();
    expect(screen.getByLabelText("Periodo de recaudo")).toHaveProperty(
      "value",
      "2026-01",
    );
    await screen.findByText(VACIO_2026_01);
    expect(getsDelListado()).toEqual(["/api/reportes?periodo=2026-01"]);

    // Si la URL cambia por fuera del campo (aqui, el enlace de la nav), el
    // campo la sigue en vez de ensenar un periodo que ya no se aplica.
    fireEvent.click(screen.getByRole("link", { name: "Ingesta" }));
    expect(ubicacion()).toBe("/ingesta");
    expect(screen.getByLabelText("Periodo de recaudo")).toHaveProperty(
      "value",
      "",
    );
    await screen.findByText("Aún no hay cargas registradas.");
  });

  it("una subida OK manda fuente, periodo y archivo en un FormData, muestra los recuentos y vuelve a pedir el listado", async () => {
    let cargas: Carga[] = [];
    simularServidor({
      rol: "administrador",
      cargas: () => cargas,
      subida: () => {
        cargas = [cargaSubida];
        return Promise.resolve(json(entregaCaracol, 201));
      },
    });

    montarApp("/ingesta?periodo=2026-01");
    await screen.findByText(VACIO_2026_01);
    const pedidosAntes = getsDelListado().length;

    elegirFuente("caracol");
    elegirArchivo(archivoCaracol());
    fireEvent.click(botonSubir());

    await screen.findByRole("heading", { name: "Entrega registrada" });
    expect(subidas()).toHaveLength(1);
    const cuerpo = subidas()[0].init?.body;
    if (!(cuerpo instanceof FormData)) {
      throw new Error("el POST no lleva un FormData");
    }
    expect(cuerpo.get("fuente")).toBe("caracol");
    expect(cuerpo.get("periodo")).toBe("2026-01");
    const archivo = cuerpo.get("archivo");
    expect(archivo).toBeInstanceOf(File);
    expect((archivo as File).name).toBe("caracol.csv");
    // El formato lo deduce el backend de la extension.
    expect(cuerpo.has("formato")).toBe(false);

    expect(dato("Filas aceptadas")).toBe("1.234");
    expect(dato("Filas rechazadas")).toBe("1");

    // El listado se remonta al subir, vuelve a pedirse y trae la carga nueva.
    await screen.findByRole("table", { name: "Cargas hechas" });
    expect(getsDelListado()).toHaveLength(pedidosAntes + 1);
    expect(getsDelListado().at(-1)).toBe("/api/reportes?periodo=2026-01");
  });

  it("un archivo mal formado muestra el 400 tal cual, sin recargar ni navegar", async () => {
    simularServidor({
      rol: "administrador",
      subida: () => Promise.resolve(json({ error: MENSAJE_400 }, 400)),
    });

    montarApp("/ingesta?periodo=2026-01");
    await screen.findByText(VACIO_2026_01);

    elegirFuente("caracol");
    elegirArchivo(archivoCaracol());
    fireEvent.click(botonSubir());

    const alerta = await screen.findByRole("alert");
    expect(within(alerta).getByText(MENSAJE_400).textContent).toBe(MENSAJE_400);
    // El formulario sigue montado, con lo elegido, y la ruta no cambio.
    expect(screen.getByLabelText("Fuente")).toHaveProperty("value", "caracol");
    expect(botonSubir()).toHaveProperty("disabled", false);
    expect(ubicacion()).toBe("/ingesta?periodo=2026-01");
    // No entro nada, asi que el listado no se vuelve a pedir.
    expect(getsDelListado()).toHaveLength(1);
  });

  it("con la subida en vuelo el boton queda deshabilitado y dos submits seguidos mandan un solo POST", async () => {
    let responder: (respuesta: Response) => void = () => {};
    simularServidor({
      rol: "administrador",
      subida: () =>
        new Promise<Response>((resolver) => {
          responder = resolver;
        }),
    });

    montarApp("/ingesta?periodo=2026-01");
    await screen.findByText(VACIO_2026_01);

    elegirFuente("caracol");
    elegirArchivo(archivoCaracol());
    const boton = botonSubir();
    const formulario = boton.closest("form");
    if (!formulario) throw new Error("no se encontro el formulario");

    // Los dos submits van dentro del MISMO `act`: el segundo entra antes de que
    // React vuelva a pintar, asi que el `disabled` del render todavia no existe
    // y el unico que puede cortarlo es el ref `enVuelo`. Con dos
    // `fireEvent.click` sueltos cada uno cerraria su propio `act`, el boton ya
    // estaria deshabilitado y el test pasaria tambien sin el ref.
    act(() => {
      fireEvent.submit(formulario);
      fireEvent.submit(formulario);
    });

    expect(subidas()).toHaveLength(1);
    expect(boton).toHaveProperty("disabled", true);
    expect(boton.textContent).toBe("Subiendo…");

    responder(json(entregaCaracol, 201));
    await screen.findByRole("heading", { name: "Entrega registrada" });
    expect(botonSubir()).toHaveProperty("disabled", false);
    expect(subidas()).toHaveLength(1);
    // El listado se remonta tras el 201: se espera a que termine.
    await screen.findByText(VACIO_2026_01);
  });

  it.each(CUERPOS_QUE_NO_SON_ENTREGA)(
    "un 201 con cuerpo de %s no tumba la pantalla: cae en el fallo desconocido",
    async (_caso, respuesta) => {
      simularServidor({
        rol: "administrador",
        subida: () => Promise.resolve(respuesta()),
      });

      montarApp("/ingesta?periodo=2026-01");
      await screen.findByText(VACIO_2026_01);

      elegirFuente("caracol");
      elegirArchivo(archivoCaracol());
      fireEvent.click(botonSubir());

      // Ni el cuerpo ni su forma llegan al panel: el 201 se lee como un fallo
      // no clasificable, con el aviso de que la entrega pudo haber llegado.
      const alerta = await screen.findByRole("alert");
      expect(
        within(alerta).getByRole("heading", {
          name: TITULO_FALLO_DESCONOCIDO,
        }),
      ).toBeTruthy();
      expect(alerta.textContent).toContain(MENSAJE_DESCONOCIDO);
      expect(alerta.textContent).toContain(PUDO_LLEGAR);

      // La pantalla sigue montada, el listado en pie y el formulario se puede
      // reintentar.
      expect(
        screen.getByRole("heading", { name: "Ingesta de reportes" }),
      ).toBeTruthy();
      expect(
        screen.getByRole("heading", { name: "Cargas hechas" }),
      ).toBeTruthy();
      expect(screen.getByText(VACIO_2026_01)).toBeTruthy();
      expect(botonSubir()).toHaveProperty("disabled", false);
      expect(subidas()).toHaveLength(1);
    },
  );

  it("sin fuente o sin archivo no se puede subir", async () => {
    simularServidor({ rol: "administrador" });

    montarApp("/ingesta?periodo=2026-01");
    await screen.findByText(VACIO_2026_01);

    expect(botonSubir()).toHaveProperty("disabled", true);
    elegirFuente("caracol");
    expect(botonSubir()).toHaveProperty("disabled", true);
    elegirArchivo(archivoCaracol());
    expect(botonSubir()).toHaveProperty("disabled", false);
    elegirFuente("");
    expect(botonSubir()).toHaveProperty("disabled", true);
  });

  it("el periodo solo se aplica completo: entonces pasa a la URL, filtra el listado y habilita la subida", async () => {
    simularServidor({ rol: "administrador" });

    montarApp("/ingesta");
    await screen.findByText("Aún no hay cargas registradas.");
    expect(getsDelListado()).toEqual(["/api/reportes"]);

    elegirFuente("caracol");
    elegirArchivo(archivoCaracol());
    expect(botonSubir()).toHaveProperty("disabled", true);
    expect(screen.getByText(FALTA_PERIODO)).toBeTruthy();

    // A medias no toca la URL ni consulta nada.
    escribirPeriodo("2026-0");
    expect(ubicacion()).toBe("/ingesta");
    expect(botonSubir()).toHaveProperty("disabled", true);

    escribirPeriodo("2026-01");
    expect(ubicacion()).toBe("/ingesta?periodo=2026-01");
    await screen.findByText(VACIO_2026_01);
    expect(getsDelListado()).toEqual([
      "/api/reportes",
      "/api/reportes?periodo=2026-01",
    ]);
    expect(botonSubir()).toHaveProperty("disabled", false);
    expect(screen.queryByText(FALTA_PERIODO)).toBeNull();

    // Volver a dejarlo a medias bloquea la subida: el campo ya no dice el
    // periodo que se mandaria.
    escribirPeriodo("2026-0");
    expect(ubicacion()).toBe("/ingesta?periodo=2026-01");
    expect(botonSubir()).toHaveProperty("disabled", true);

    // Vaciarlo lo quita de la URL y el listado vuelve a traer todo.
    escribirPeriodo("");
    expect(ubicacion()).toBe("/ingesta");
    await screen.findByText("Aún no hay cargas registradas.");
    expect(getsDelListado()).toHaveLength(3);
  });

  it("el boton nombra el periodo destino cuando hay uno utilizable, y no cuando no lo hay", async () => {
    simularServidor({ rol: "administrador" });

    montarApp("/ingesta?periodo=2026-01");
    await screen.findByText(VACIO_2026_01);

    elegirFuente("caracol");
    elegirArchivo(archivoCaracol());
    // El boton dice a donde va el archivo, no solo que sube.
    expect(botonSubir().textContent).toBe("Subir a 2026-01");
    expect(botonSubir()).toHaveProperty("disabled", false);

    // Sin periodo utilizable vuelve al texto neutro.
    escribirPeriodo("");
    await screen.findByText("Aún no hay cargas registradas.");
    expect(botonSubir().textContent).toBe("Subir reporte");
    expect(botonSubir()).toHaveProperty("disabled", true);
  });

  it.each(["2026-13", "2026-00"])(
    "con el periodo %s la subida queda bloqueada, aunque el listado si se consulte",
    async (periodo) => {
      simularServidor({ rol: "administrador" });

      montarApp("/ingesta");
      await screen.findByText("Aún no hay cargas registradas.");

      elegirFuente("caracol");
      elegirArchivo(archivoCaracol());
      escribirPeriodo(periodo);

      // El backend no lo frena: su validador del periodo es mas flojo que el
      // del dominio, asi que el listado lo consulta y contesta que en ese
      // periodo no hay cargas.
      expect(ubicacion()).toBe(`/ingesta?periodo=${periodo}`);
      await screen.findByText(
        `No hay cargas registradas para el periodo ${periodo}.`,
      );

      // Pero subir a un mes que no existe no se deshace: la subida queda
      // bloqueada y se dice por que.
      expect(botonSubir()).toHaveProperty("disabled", true);
      expect(botonSubir().textContent).toBe("Subir reporte");
      expect(screen.getByText(FALTA_PERIODO)).toBeTruthy();

      // Y no se cuela ni con un submit directo al formulario.
      const formulario = botonSubir().closest("form");
      if (!formulario) throw new Error("no se encontro el formulario");
      fireEvent.submit(formulario);
      expect(subidas()).toHaveLength(0);
    },
  );

  it("al cambiar de periodo el listado se remonta: la fila abierta queda cerrada y no repite su log", async () => {
    simularServidor({
      rol: "administrador",
      // Con una carga en rechazos solo en 2026-01, para que la ida y vuelta se
      // note.
      cargas: (url) =>
        url.includes("periodo=2026-01") ? [cargaConRechazos] : [],
    });

    montarApp("/ingesta?periodo=2026-01");
    await screen.findByRole("table", { name: "Cargas hechas" });

    fireEvent.click(screen.getByRole("button", { name: "Ver rechazos (3)" }));
    // La carga no existe para este backend falso, pero el error queda dentro de
    // la fila abierta: lo que importa aqui es que el log se pidio una vez.
    await screen.findByRole("alert");
    expect(logsDeRechazos()).toHaveLength(1);
    expect(
      screen
        .getByRole("button", { name: "Ocultar rechazos" })
        .getAttribute("aria-expanded"),
    ).toBe("true");

    // Otro periodo: el listado se remonta y con el se va el estado de las filas.
    escribirPeriodo("2026-02");
    await screen.findByText(
      "No hay cargas registradas para el periodo 2026-02.",
    );

    // Y al volver, la fila esta cerrada de nuevo y su log no se pide sin clic.
    escribirPeriodo("2026-01");
    await screen.findByRole("table", { name: "Cargas hechas" });
    expect(
      screen
        .getByRole("button", { name: "Ver rechazos (3)" })
        .getAttribute("aria-expanded"),
    ).toBe("false");
    expect(logsDeRechazos()).toHaveLength(1);
  });

  it("soltar un archivo en la zona lo elige, igual que el selector", async () => {
    simularServidor({ rol: "administrador" });

    montarApp("/ingesta?periodo=2026-01");
    await screen.findByText(VACIO_2026_01);

    elegirFuente("caracol");
    fireEvent.drop(screen.getByText(/Arrastra el archivo hasta aquí/), {
      dataTransfer: { files: [archivoCaracol()] },
    });

    expect(screen.getByText("caracol.csv")).toBeTruthy();
    expect(botonSubir()).toHaveProperty("disabled", false);
  });

  it("un auditor en /ingesta ve 'No autorizado', sin enlace y sin pedir nada a /api/reportes", async () => {
    simularServidor({ rol: "auditor" });

    montarApp("/ingesta");

    await screen.findByRole("heading", { name: "No autorizado" });
    expect(screen.queryByRole("link", { name: "Ingesta" })).toBeNull();
    expect(
      screen.queryByRole("heading", { name: "Ingesta de reportes" }),
    ).toBeNull();
    expect(llamadas().filter((l) => l.url.startsWith("/api/reportes"))).toEqual(
      [],
    );
  });
});
