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
const SHA_CINE =
  "3c8d1e5f7a9b0c2d4e6f8a0b1c3d5e7f9a1b3c5d7e9f0a2b4c6d8e0f1a3b5c7d";

// El id de una carga es `rep-` mas 64 hex, y el de un rechazo es ese id mas
// `-<n>` (internal/aplicacion/ingesta.go: `idReporte` compone el de la carga a
// partir del par (fuente, huella) y `u.ID = rep.ID + "-" + n` el del rechazo).
// El hex de estos fixtures es la huella del archivo y no el derivado del par
// -da igual para lo que se prueba- pero la FORMA y el largo son los de
// produccion a proposito: con ids de 12 caracteres la tabla nunca se pinta al
// ancho con que se vera, y el corte de linea del CSS no se ejerce.
const ID_CARACOL = `rep-${SHA_CARACOL}`;
const ID_NETFLIX = `rep-${SHA_NETFLIX}`;
const ID_CINE = `rep-${SHA_CINE}`;

const cargaSinRechazos = {
  id: ID_CARACOL,
  fuente: "caracol",
  periodo: "2026-01",
  sha256: SHA_CARACOL,
  // `claveObjeto(sha)` en Go: "reportes/" mas la huella.
  clave_objeto: `reportes/${SHA_CARACOL}`,
  nbytes: 21032,
  recibido: "2026-02-03T14:05:00Z",
  aceptados: 1234,
  rechazados: 0,
} satisfies Carga;

const cargaConRechazos = {
  id: ID_NETFLIX,
  fuente: "netflix",
  periodo: "2026-01",
  sha256: SHA_NETFLIX,
  clave_objeto: `reportes/${SHA_NETFLIX}`,
  nbytes: 4096,
  recibido: "2026-02-04T09:30:00Z",
  aceptados: 58,
  rechazados: 3,
} satisfies Carga;

const rechazos = [
  {
    id: `${ID_NETFLIX}-2`,
    titulo: "Obra sintetica A",
    ids_fuente: "id_netflix=80000001",
    motivo: 'fila 2, titulo (columna "Title"): vacio',
  },
  {
    id: `${ID_NETFLIX}-5`,
    titulo: "Obra sintetica B",
    ids_fuente: "id_netflix=80000002",
    motivo:
      'fila 5, horas_vistas (columna "Hours Viewed"): "muchas" no es un numero',
  },
  {
    id: `${ID_NETFLIX}-11`,
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

  it("un 2xx sin JSON deja el listado en error en vez de tumbarlo", async () => {
    // `api()` devuelve el `Response` crudo cuando el content-type no es JSON:
    // antes llegaba hasta `cargas.length` / `cargas.map` y reventaba la
    // pantalla. `useApi` lo convierte en error.
    vi.mocked(fetch).mockResolvedValue(
      new Response("<html><body>sin API</body></html>", {
        status: 200,
        headers: { "content-type": "text/html" },
      }),
    );

    render(<ListaCargas periodo="2026-01" />);

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain(
      "No se pudo consultar el listado de cargas",
    );
    expect(alerta.textContent).toContain("la respuesta no vino en JSON");
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("un 2xx con un JSON que no es una lista no revienta el listado", async () => {
    // `useApi<Carga[]>` no comprueba la forma: un objeto con 200 (el `{error}`
    // de un proxy, por ejemplo) llegaria a `.length` y a `.map`.
    vi.mocked(fetch).mockResolvedValue(json({ error: "algo salio mal" }));

    render(<ListaCargas periodo="2026-01" />);

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain(
      "El listado no llegó como una lista de cargas legibles.",
    );
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("un 2xx con una lista de elementos vacios no revienta el listado", async () => {
    // Reproduccion del crash de la revision pasada 3: `[{}]` es una lista de
    // verdad con un elemento sin ningun campo, asi que pasaba el `Array.isArray`
    // y `huellaCorta(carga.sha256)` lanzaba "Cannot read properties of
    // undefined (reading 'slice')". Sin ErrorBoundary en `web/src`, la pantalla
    // se quedaba en blanco.
    vi.mocked(fetch).mockResolvedValue(json([{}]));

    render(<ListaCargas periodo="2026-01" />);

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain(
      "El listado no llegó como una lista de cargas legibles.",
    );
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("una carga con un campo mal tipado deja el listado en error, no la fila escondida", async () => {
    // `sha256` es el campo que revienta; `aceptados`/`rechazados` son las cifras
    // que el listado muestra, y una cifra que no es un numero se pintaria como
    // `NaN`. Los dos casos dejan el listado entero en error: saltarse la fila
    // mala escondería una carga sin decirlo.
    vi.mocked(fetch).mockResolvedValue(
      json([{ ...cargaSinRechazos, sha256: null }, cargaConRechazos]),
    );

    render(<ListaCargas periodo="2026-01" />);

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain(
      "El listado no llegó como una lista de cargas legibles.",
    );
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("un log de rechazos que no es una lista no revienta la fila", async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(json([cargaConRechazos]))
      .mockResolvedValueOnce(json({ error: "algo salio mal" }));

    render(<ListaCargas periodo="2026-01" />);

    await screen.findByRole("table", { name: "Cargas hechas" });
    fireEvent.click(screen.getByRole("button", { name: "Ver rechazos (3)" }));

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain(
      "El log no llegó como una lista de rechazos legibles.",
    );
    // El resto del listado sigue en pie.
    expect(filaCon("netflix")).toBeTruthy();
  });

  it("un log con un rechazo de campo mal tipado deja la fila en error, sin tumbarla", async () => {
    // `ids_fuente` objeto: el contrato lo declara `string` y React no pinta un
    // objeto como hijo ("Objects are not valid as a React child"), asi que la
    // tabla reventaba con el panel entero.
    vi.mocked(fetch)
      .mockResolvedValueOnce(json([cargaConRechazos]))
      .mockResolvedValueOnce(
        json([{ ...rechazos[0], ids_fuente: { id_netflix: "80000001" } }]),
      );

    render(<ListaCargas periodo="2026-01" />);

    await screen.findByRole("table", { name: "Cargas hechas" });
    fireEvent.click(screen.getByRole("button", { name: "Ver rechazos (3)" }));

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain(
      "El log no llegó como una lista de rechazos legibles.",
    );
    // El resto del listado sigue en pie.
    expect(filaCon("netflix")).toBeTruthy();
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

  it("al abrir pide la primera pagina del log de esa carga y la pinta; al cerrar lo oculta", async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(json([cargaSinRechazos, cargaConRechazos]))
      .mockResolvedValueOnce(json(rechazos));

    render(<ListaCargas periodo="2026-01" />);

    await screen.findByRole("table", { name: "Cargas hechas" });
    fireEvent.click(screen.getByRole("button", { name: "Ver rechazos (3)" }));

    const log = await screen.findByRole("table", { name: "Filas rechazadas" });
    // La primera pagina, con el limite explicito: el servidor aplica 100 si no
    // se manda, pero la pantalla lo dice para que el rango de abajo cuadre.
    expect(pedido(1)).toBe(
      `/api/reportes/${ID_NETFLIX}/rechazos?limite=100&desplazamiento=0`,
    );
    expect(fetch).toHaveBeenCalledTimes(2);
    // Las filas que el servidor mando, ninguna recortada por la pantalla.
    expect(within(log).getAllByRole("row").slice(1)).toHaveLength(3);
    for (const rechazo of rechazos) {
      expect(within(log).getByText(rechazo.motivo)).toBeTruthy();
    }
    // Y la cifra entera, del recuento de la carga: es "de M", no "de 3 filas
    // que casualmente llegaron".
    expect(screen.getByText("Rechazos 1 a 3 de 3")).toBeTruthy();
    // En la ultima pagina no hay a donde seguir, y hacia atras no hay nada.
    expect(screen.getByRole("button", { name: "Siguiente" })).toHaveProperty(
      "disabled",
      true,
    );
    expect(screen.getByRole("button", { name: "Anterior" })).toHaveProperty(
      "disabled",
      true,
    );
    // La columna "Id" pinta el id entero, con sus 64 hex de huella: es la
    // unica vista que lo muestra a su ancho real (~70 caracteres).
    expect(within(log).getByText(rechazos[0].id)).toBeTruthy();

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

  it("el log se pide por paginas y cada una dice su tramo de la cifra total", async () => {
    // Una carga con 250 rechazos: el listado dice 250, y el log se pide de 100
    // en 100 con el desplazamiento de cada pagina.
    const carga = { ...cargaConRechazos, rechazados: 250 } satisfies Carga;
    const pagina = (n: number, desde: number) =>
      Array.from({ length: n }, (_, i) => ({
        id: `${ID_NETFLIX}-${desde + i}`,
        titulo: `Obra sintetica ${desde + i}`,
        ids_fuente: `id_netflix=${desde + i}`,
        motivo: `fila ${desde + i}, titulo (columna "Title"): vacio`,
      })) satisfies Rechazo[];
    vi.mocked(fetch)
      .mockResolvedValueOnce(json([carga]))
      .mockResolvedValueOnce(json(pagina(100, 0)))
      .mockResolvedValueOnce(json(pagina(100, 100)))
      .mockResolvedValueOnce(json(pagina(50, 200)));

    render(<ListaCargas periodo="2026-01" />);

    await screen.findByRole("table", { name: "Cargas hechas" });
    fireEvent.click(screen.getByRole("button", { name: "Ver rechazos (250)" }));
    await screen.findByRole("table", { name: "Filas rechazadas" });

    expect(screen.getByText("Rechazos 1 a 100 de 250")).toBeTruthy();
    expect(pedido(1)).toBe(
      `/api/reportes/${ID_NETFLIX}/rechazos?limite=100&desplazamiento=0`,
    );
    // Hay mas, asi que se puede seguir.
    expect(screen.getByRole("button", { name: "Siguiente" })).toHaveProperty(
      "disabled",
      false,
    );

    fireEvent.click(screen.getByRole("button", { name: "Siguiente" }));
    await screen.findByText("Rechazos 101 a 200 de 250");
    expect(pedido(2)).toBe(
      `/api/reportes/${ID_NETFLIX}/rechazos?limite=100&desplazamiento=100`,
    );

    fireEvent.click(screen.getByRole("button", { name: "Siguiente" }));
    await screen.findByText("Rechazos 201 a 250 de 250");
    expect(pedido(3)).toBe(
      `/api/reportes/${ID_NETFLIX}/rechazos?limite=100&desplazamiento=200`,
    );
    // Ultima pagina: se apaga el siguiente y se puede volver.
    expect(screen.getByRole("button", { name: "Siguiente" })).toHaveProperty(
      "disabled",
      true,
    );
    // Volver atras pide la pagina anterior otra vez: no hay cache.
    vi.mocked(fetch).mockResolvedValueOnce(json(pagina(100, 100)));
    fireEvent.click(screen.getByRole("button", { name: "Anterior" }));
    await screen.findByText("Rechazos 101 a 200 de 250");
    expect(pedido(4)).toBe(
      `/api/reportes/${ID_NETFLIX}/rechazos?limite=100&desplazamiento=100`,
    );
  });

  it("varias cargas pueden tener su log abierto a la vez", async () => {
    const otra = {
      ...cargaConRechazos,
      id: ID_CINE,
      fuente: "cine",
      sha256: SHA_CINE,
      clave_objeto: `reportes/${SHA_CINE}`,
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
    expect(pedido(2)).toBe(
      `/api/reportes/${ID_CINE}/rechazos?limite=100&desplazamiento=0`,
    );
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
  listado,
}: {
  rol: Rol;
  cargas?: (url: string) => Carga[];
  subida?: () => Promise<Response>;
  /** Respuesta cruda del listado, para probar un 400 del backend. */
  listado?: (url: string) => Response;
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
      if (listado) return Promise.resolve(listado(url));
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

/** Los GET del log de rechazos de alguna carga: uno por cada fila abierta sin cache, y uno por cada cambio de pagina. */
function logsDeRechazos() {
  return llamadas().filter((l) => l.url.includes("/rechazos"));
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

function elegirMes(valor: string) {
  fireEvent.change(screen.getByLabelText("Mes"), {
    target: { value: valor },
  });
}

function elegirAnno(valor: string) {
  fireEvent.click(screen.getByLabelText("Anual"));
  fireEvent.change(screen.getByLabelText("Año"), {
    target: { value: valor },
  });
}

// El texto del boton cambia: "Subir a AAAA-MM" cuando nombra el periodo
// destino, "Subir reporte" cuando no hay ningun periodo utilizable y
// "Subiendo…" mientras la subida esta en vuelo. El helper los abarca todos;
// los tests que juzgan el texto lo afirman literal.
const botonSubir = () => screen.getByRole("button", { name: /^Subir/ });

const VACIO_2026_01 = "No hay cargas registradas para el periodo 2026-01.";
// Errores de validacion al intentar subir con algo sin elegir. Literales para
// que un cambio de copy se note.
const ERROR_SIN_PERIODO =
  "Falta el periodo: elige un mes o un año para subir el reporte.";
const ERROR_SIN_FUENTE = "Falta la fuente: elige de dónde viene el reporte.";
const ERROR_SIN_ARCHIVO = "Falta el archivo: suelta o elige el reporte a subir.";

// El fallo no clasificable del panel: lo que se ve cuando un 2xx no trae una
// `Entrega` legible. Textos propios de `PanelResultado`, repetidos aqui como
// literales para que un cambio de copy se note. El titulo es neutro a proposito:
// justo debajo va el aviso de que la entrega pudo haber llegado.
const TITULO_FALLO_DESCONOCIDO = "No se sabe si la entrega se registró";
const MENSAJE_DESCONOCIDO = "error desconocido al subir el archivo";
const PUDO_LLEGAR =
  "La entrega pudo haber llegado al servidor: revisa el listado de cargas antes de volver a subirla.";
// Lo que `mensajeDeError` (api.ts) pone en lugar de un cuerpo que no es JSON.
const MENSAJE_ERROR_ILEGIBLE = "el servidor respondió un error ilegible";

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
      id: `${ID_CARACOL}-6`,
      titulo: "Obra sintetica D",
      ids_fuente: "id_ficha=F-004",
      motivo: 'fila 6, titulo (columna "Titulo"): vacio',
    },
  ],
} satisfies Entrega;

// La misma entrega, como la devuelve el listado despues de subirla.
const cargaSubida = { ...cargaSinRechazos, rechazados: 1 } satisfies Carga;

// Un 201 que no trae una `Entrega`. El primero es el cuerpo que devuelve un
// proxy; el segundo un JSON al que le falta todo salvo el id; el tercero un JSON
// con `rechazados` lista y nada mas, que es el que pasaba la guarda anterior
// -solo exigia "objeto con `rechazados` lista"- y reventaba al leer
// `entrega.sha256` al pintar la huella; el cuarto es una entrega completa con un
// `null` dentro de `rechazados`, que revienta al leer `.id` de cada fila; el
// quinto una entrega completa cuyo rechazo trae `ids_fuente` como objeto, que el
// contrato declara `string` y que React no pinta como hijo. Los cinco llegan a
// `api()` como un resultado valido.
const CUERPOS_QUE_NO_SON_ENTREGA: [string, () => Response][] = [
  [
    "HTML",
    () =>
      new Response("<html><body>Bad Gateway</body></html>", {
        status: 201,
        headers: { "content-type": "text/html" },
      }),
  ],
  ["JSON sin rechazados", () => json({ id: ID_CARACOL }, 201)],
  [
    "JSON con rechazados y sin el resto de la entrega",
    () => json({ rechazados: [] }, 201),
  ],
  [
    "JSON con un rechazo nulo en la lista",
    () => json({ ...entregaCaracol, rechazados: [null] }, 201),
  ],
  [
    "JSON con un rechazo cuyo ids_fuente es un objeto",
    () =>
      json(
        {
          ...entregaCaracol,
          rechazados: [
            { ...entregaCaracol.rechazados[0], ids_fuente: { a: 1 } },
          ],
        },
        201,
      ),
  ],
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
    expect(screen.getByLabelText("Mes")).toHaveProperty("value", "2026-01");
    await screen.findByText(VACIO_2026_01);
    expect(getsDelListado()).toEqual(["/api/reportes?periodo=2026-01"]);

    // Si la URL cambia por fuera del campo (aqui, el enlace de la nav), el
    // campo la sigue en vez de ensenar un periodo que ya no se aplica.
    fireEvent.click(screen.getByRole("link", { name: "Ingesta" }));
    expect(ubicacion()).toBe("/ingesta");
    expect(screen.getByLabelText("Mes")).toHaveProperty("value", "");
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

  it("un 4xx con un cuerpo que no es JSON no pinta ese cuerpo como mensaje del backend", async () => {
    // El 409 llega con la pagina de error de nginx en vez del JSON del
    // contrato: el status manda el titulo, pero el mensaje no es el cuerpo. Un
    // 408 de `client_body_timeout` es el caso realista de la misma via.
    simularServidor({
      rol: "administrador",
      subida: () =>
        Promise.resolve(
          new Response("<html><body><h1>409 Conflict</h1>nginx</body></html>", {
            status: 409,
            headers: { "content-type": "text/html" },
          }),
        ),
    });

    montarApp("/ingesta?periodo=2026-01");
    await screen.findByText(VACIO_2026_01);

    elegirFuente("caracol");
    elegirArchivo(archivoCaracol());
    fireEvent.click(botonSubir());

    const alerta = await screen.findByRole("alert");
    expect(
      within(alerta).getByRole("heading", {
        name: "La entrega choca con lo que ya está guardado",
      }),
    ).toBeTruthy();
    // El 409 tiene DOS ramas -el duplicado, donde la entrega ya estaba, y la
    // evidencia corrupta, donde esta peticion tampoco dejo entrega- y el TITULO
    // solo ve el status, asi que no puede elegir rama. Aqui ademas el cuerpo
    // que llega es la pagina del proxy, no el mensaje del backend. Por eso el
    // titulo no puede afirmar ni el registro ni el no-registro, y solo nombra
    // el conflicto; el mensaje de debajo dice cual de las dos es cuando llega
    // legible.
    expect(alerta.textContent).not.toMatch(/no se registró/i);
    expect(alerta.textContent).toContain(MENSAJE_ERROR_ILEGIBLE);
    // Ni el marcado ni el titulo de la pagina del proxy.
    expect(document.body.textContent).not.toContain("409 Conflict");
    expect(document.body.textContent).not.toContain("nginx");
    // Un 409 si es una respuesta del servidor: no lleva el aviso de que pudo
    // haber llegado.
    expect(within(alerta).queryByText(PUDO_LLEGAR)).toBeNull();
  });

  it("un 500 no afirma el no-registro, avisa de que pudo llegar y no pinta el JSON crudo de su cuerpo", async () => {
    // Un 500 no garantiza que no quedo entrega: `Store.EnTransaccion` devuelve
    // el fallo del COMMIT como un error mas (internal/infraestructura/postgres/
    // store.go) y si la respuesta del COMMIT se pierde el cliente no puede
    // saber si entro (`08007`/`40003` del motor), asi que el panel no promete
    // el no-registro y devuelve el aviso de mirar el listado antes de resubir.
    // El cuerpo, ademas, no trae `error`, asi que no es un mensaje del backend
    // y no se pinta.
    simularServidor({
      rol: "administrador",
      subida: () => Promise.resolve(json({ detalle: "algo" }, 500)),
    });

    montarApp("/ingesta?periodo=2026-01");
    await screen.findByText(VACIO_2026_01);
    const pedidosAntes = getsDelListado().length;

    elegirFuente("caracol");
    elegirArchivo(archivoCaracol());
    fireEvent.click(botonSubir());

    const alerta = await screen.findByRole("alert");
    expect(
      within(alerta).getByRole("heading", {
        name: "No se sabe si la entrega se registró",
      }),
    ).toBeTruthy();
    expect(alerta.textContent).toContain(MENSAJE_ERROR_ILEGIBLE);
    expect(within(alerta).getByText(PUDO_LLEGAR)).toBeTruthy();
    // Ni el JSON crudo ni su clave llegan a la pantalla.
    expect(alerta.textContent).not.toContain("detalle");
    expect(document.body.textContent).not.toContain('{"detalle"');
    expect(subidas()).toHaveLength(1);

    // Y el listado que el aviso manda a mirar se vuelve a pedir. Es la mitad que
    // faltaba: afirmar el texto de PUDO_LLEGAR sin comprobar el GET dejaba en
    // verde una pantalla que mandaba al operador a la foto de ANTES de la
    // subida -si el COMMIT entro, no ve la fila nueva, concluye que no llego y
    // reenvia, y el 409 es irreversible-.
    await vi.waitFor(() =>
      expect(getsDelListado()).toHaveLength(pedidosAntes + 1),
    );
  });

  it("un listado con un elemento que no es una carga deja el error en su sitio sin tumbar la pantalla", async () => {
    simularServidor({
      rol: "administrador",
      // Una lista de verdad con un elemento sin campos: es la forma que
      // reventaba en `huellaCorta(carga.sha256)`.
      cargas: () => [{} as Carga],
    });

    montarApp("/ingesta?periodo=2026-01");

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain(
      "El listado no llegó como una lista de cargas legibles.",
    );
    // La pantalla entera sigue montada: antes el TypeError la dejaba en blanco.
    expect(
      screen.getByRole("heading", { name: "Ingesta de reportes" }),
    ).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Cargas hechas" })).toBeTruthy();
    expect(screen.queryByRole("table", { name: "Cargas hechas" })).toBeNull();
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

  it("sin periodo, fuente o archivo la subida no sale: avisa que falta sin llamar al servidor", async () => {
    simularServidor({ rol: "administrador" });

    montarApp("/ingesta?periodo=2026-01");
    await screen.findByText(VACIO_2026_01);

    // El boton siempre se puede pulsar; lo que falta se dice con un error.
    fireEvent.click(botonSubir());
    expect(screen.getByText(ERROR_SIN_FUENTE)).toBeTruthy();
    expect(subidas()).toHaveLength(0);

    // Al corregirse el error se apaga solo, sin reintentar.
    elegirFuente("caracol");
    expect(screen.queryByText(ERROR_SIN_FUENTE)).toBeNull();

    fireEvent.click(botonSubir());
    expect(screen.getByText(ERROR_SIN_ARCHIVO)).toBeTruthy();
    expect(subidas()).toHaveLength(0);

    elegirArchivo(archivoCaracol());
    expect(screen.queryByText(ERROR_SIN_ARCHIVO)).toBeNull();

    fireEvent.click(botonSubir());
    expect(subidas()).toHaveLength(1);
  });

  it("el periodo se elige con el selector: pasa a la URL, filtra el listado y habilita la subida", async () => {
    simularServidor({ rol: "administrador" });

    montarApp("/ingesta");
    await screen.findByText("Aún no hay cargas registradas.");
    expect(getsDelListado()).toEqual(["/api/reportes"]);

    elegirFuente("caracol");
    elegirArchivo(archivoCaracol());

    // Sin periodo el boton se puede pulsar, pero la subida no sale: dice que
    // falta y no llama al servidor.
    fireEvent.click(botonSubir());
    expect(screen.getByText(ERROR_SIN_PERIODO)).toBeTruthy();
    expect(subidas()).toHaveLength(0);
    expect(getsDelListado()).toHaveLength(1);

    // El calendario no deja estados a medias: elegir aplica de una vez y el
    // error se apaga solo.
    elegirMes("2026-01");
    expect(ubicacion()).toBe("/ingesta?periodo=2026-01");
    expect(screen.queryByText(ERROR_SIN_PERIODO)).toBeNull();
    await screen.findByText(VACIO_2026_01);
    expect(getsDelListado()).toEqual([
      "/api/reportes",
      "/api/reportes?periodo=2026-01",
    ]);

    // Vaciarlo lo quita de la URL y el listado vuelve a traer todo.
    elegirMes("");
    expect(ubicacion()).toBe("/ingesta");
    await screen.findByText("Aún no hay cargas registradas.");
    expect(getsDelListado()).toHaveLength(3);
  });

  it("en modo anual el periodo es un año: pasa a la URL y habilita la subida", async () => {
    simularServidor({ rol: "administrador" });

    montarApp("/ingesta");
    await screen.findByText("Aún no hay cargas registradas.");

    elegirFuente("caracol");
    elegirArchivo(archivoCaracol());
    elegirAnno("2026");

    expect(ubicacion()).toBe("/ingesta?periodo=2026");
    await screen.findByText("No hay cargas registradas para el periodo 2026.");
    expect(getsDelListado()).toEqual([
      "/api/reportes",
      "/api/reportes?periodo=2026",
    ]);
    expect(botonSubir().textContent).toBe("Subir a 2026");
    expect(botonSubir()).toHaveProperty("disabled", false);
  });

  it("el boton nombra el periodo destino cuando hay uno utilizable, y no cuando no lo hay", async () => {
    simularServidor({ rol: "administrador" });

    montarApp("/ingesta?periodo=2026-01");
    await screen.findByText(VACIO_2026_01);

    elegirFuente("caracol");
    elegirArchivo(archivoCaracol());
    // El boton dice a donde va el archivo, no solo que sube.
    expect(botonSubir().textContent).toBe("Subir a 2026-01");

    // Sin periodo vuelve al texto neutro, y al pulsarlo dice que falta.
    elegirMes("");
    await screen.findByText("Aún no hay cargas registradas.");
    expect(botonSubir().textContent).toBe("Subir reporte");
    fireEvent.click(botonSubir());
    expect(screen.getByText(ERROR_SIN_PERIODO)).toBeTruthy();
    expect(subidas()).toHaveLength(0);
  });

  it.each(["2026-13", "2026-00"])(
    "con el periodo %s el rechazo lo da el servidor, que es quien conoce la regla",
    async (periodo) => {
      // El calendario no deja elegir un mes imposible, asi que el caso solo
      // llega por la URL. El cliente NO lleva una copia del patron del
      // dominio: lo unico que decide es que haya periodo. Que el mes exista lo
      // contesta el backend, y su 400 es el que se ve. Con la copia en el
      // navegador, un mes imposible entraba por `curl`, por el scheduler o por
      // cualquier pantalla futura, y la unicidad (sha256, fuente) lo dejaba
      // quemado para siempre.
      const mensaje = `reporte invalido: periodo "${periodo}", se esperaba AAAA o AAAA-MM con un mes entre 01 y 12`;
      simularServidor({
        rol: "administrador",
        // El backend rechaza el mes imposible en las DOS rutas: el listado
        // contesta 400 en vez de una lista vacia, que se leeria como "ese mes no
        // tuvo recaudo" cuando lo que pasa es que ese mes no existe. Sin periodo
        // el listado sigue siendo el de siempre.
        listado: (url) =>
          url.includes(`periodo=${periodo}`)
            ? json({ error: mensaje }, 400)
            : json([]),
        subida: () => Promise.resolve(json({ error: mensaje }, 400)),
      });

      montarApp(`/ingesta?periodo=${periodo}`);

      // El periodo de la URL se consulta y el listado trae el 400.
      expect(ubicacion()).toBe(`/ingesta?periodo=${periodo}`);
      await vi.waitFor(() =>
        expect(getsDelListado()).toContain(`/api/reportes?periodo=${periodo}`),
      );
      const alertas = await screen.findAllByRole("alert");
      expect(alertas.some((a) => a.textContent?.includes(mensaje))).toBe(true);

      elegirFuente("caracol");
      elegirArchivo(archivoCaracol());

      // La subida no queda bloqueada en el navegador: hay periodo elegido, y
      // quien decide sobre el mes es el servidor.
      expect(botonSubir()).toHaveProperty("disabled", false);
      expect(botonSubir().textContent).toBe(`Subir a ${periodo}`);

      fireEvent.click(botonSubir());

      await vi.waitFor(() => expect(subidas()).toHaveLength(1));
      // Y el mensaje del backend se pinta tal cual, sin reescribirlo.
      await vi.waitFor(() => {
        expect(
          screen
            .getAllByRole("alert")
            .some((a) => a.textContent?.includes(mensaje)),
        ).toBe(true);
      });
      // Un 400 cierra la duda: no lleva el aviso de que pudo haber llegado.
      expect(screen.queryByText(PUDO_LLEGAR)).toBeNull();
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
    elegirMes("2026-02");
    await screen.findByText(
      "No hay cargas registradas para el periodo 2026-02.",
    );

    // Y al volver, la fila esta cerrada de nuevo y su log no se pide sin clic.
    elegirMes("2026-01");
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
