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
import { DEBOUNCE_TECLEO_MS } from "../useValorDiferido";
import type { Obra, VersionDeclaracion } from "./tipos";

function json(cuerpo: unknown, status = 200): Response {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

// Fixtures tipados con el contrato generado: si `Obra` cambia en
// api/openapi.yaml, `tsc` rompe aqui antes que en pantalla. Nombres y titulos
// sinteticos, no de ningun canal real.
//
// Las tres obras son el caso que el listado tiene que saber distinguir:
// `obraIncompleta` y `obraSinDeclaracion` llegan del backend con el MISMO
// estado -`incompleta`- y solo las separa `version_vigente`.
const obraCompleta = {
  id: "obra-1",
  titulo: "La Casa de las Dos Palmas",
  genero: "Drama",
  anio: 1991,
  tipo: "serie",
  ida: "IDA-1",
  coautores: [
    { nombre: "Ana Escritora", ipi: "IPI-00000001", rol: "guionista" },
  ],
  estado_declaracion: "completa",
  suma_porcentajes: 100,
  version_vigente: 3,
} satisfies Obra;

const obraIncompleta = {
  ...obraCompleta,
  id: "obra-2",
  titulo: "Noche de Bodas",
  genero: "Telenovela",
  anio: 2014,
  tipo: "telenovela",
  estado_declaracion: "incompleta",
  suma_porcentajes: 74.5,
  version_vigente: 2,
} satisfies Obra;

const obraSinDeclaracion = {
  ...obraCompleta,
  id: "obra-3",
  titulo: "Sin Declarar Todavia",
  genero: "Unitario",
  anio: 2020,
  tipo: "unitario",
  ida: undefined,
  estado_declaracion: "incompleta",
  suma_porcentajes: 0,
  version_vigente: null,
} satisfies Obra;

const LAS_TRES = [obraCompleta, obraIncompleta, obraSinDeclaracion];

/**
 * La version vigente de `obraIncompleta`, como la sirve el historial: la unica
 * abierta (`vigente_hasta: null`) y con el mismo numero de version que declara
 * la obra. `DetalleObra` no pinta partes si las dos lecturas no coinciden.
 */
const VERSION_VIGENTE_DE_LA_INCOMPLETA: VersionDeclaracion = {
  version: 2,
  vigente_desde: "2026-03-01T09:00:00Z",
  vigente_hasta: null,
  estado: "incompleta",
  partes: [{ titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: 74.5 }],
};

/**
 * Una obra cuyo identificador necesita ir codificado en la direccion. El
 * contrato lo declara opaco y asignado FUERA de este sistema, asi que no se
 * puede suponer que no traiga un caracter que la ruta reserva.
 */
const obraConEspacioEnElId = {
  ...obraCompleta,
  id: "obra 7",
  titulo: "Titulo Con Espacio",
} satisfies Obra;

/** La fila de la tabla que contiene `texto`. */
function filaCon(texto: string): HTMLElement {
  const fila = screen.getByText(texto).closest("tr");
  if (!fila) throw new Error(`"${texto}" no esta dentro de una fila`);
  return fila;
}

function tabla(): HTMLElement {
  return screen.getByRole("table", { name: "Catálogo de obras" });
}

const esConsultaDeObras = (url: string) => url.startsWith("/api/obras");

/**
 * El detalle de una obra y su historial. Se reconocen ANTES que el listado,
 * porque `GET /api/obras/{id}` tambien empieza por `/api/obras`: sin ese corte,
 * seguir el enlace de una fila recibiria la lista del catalogo y la ficha diria
 * que la obra no llego legible.
 */
const esLaObra = (url: string) => /^\/api\/obras\/[^/]+$/.test(url);
const esElHistorial = (url: string) =>
  /^\/api\/obras\/[^/]+\/declaracion\/historial$/.test(url);

/** Los GET de la busqueda, en orden: una por cada cambio de filtro o de pagina. */
function consultas(): string[] {
  return vi
    .mocked(fetch)
    .mock.calls.map(([entrada, init]) => ({
      url: String(entrada),
      metodo: init?.method ?? "GET",
    }))
    .filter((l) => l.metodo === "GET" && esConsultaDeObras(l.url))
    .map((l) => l.url);
}

const ultimaConsulta = () => consultas().at(-1);

/** Sonda de ubicacion: ruta y query, para ver donde viven los filtros. */
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

function respuestaDeSesion(rol: Rol): Response {
  return json({
    id: "usr-1",
    email: "x@redes.co",
    nombre: "Persona de Prueba",
    rol,
    titular_id: "",
  });
}

/**
 * Un backend falso que responde por URL y metodo: la sesion con el rol del
 * test, el catalogo con `obras(url)`, y el detalle de una obra con
 * `detalle(id)` mas su historial con `historial()`. `respuesta` sustituye al
 * catalogo cuando lo que se prueba es un fallo. Todo lo demas, 404.
 */
function simularServidor({
  rol,
  obras = () => [],
  respuesta,
  detalle,
  historial = () => [],
}: {
  rol: Rol;
  obras?: (url: string) => unknown;
  respuesta?: () => Response;
  detalle?: (id: string) => Response;
  historial?: () => VersionDeclaracion[];
}) {
  vi.mocked(fetch).mockImplementation((entrada, init) => {
    const url = String(entrada);
    const metodo = init?.method ?? "GET";
    if (url === "/api/auth/session") {
      return Promise.resolve(respuestaDeSesion(rol));
    }
    if (metodo === "GET" && esElHistorial(url)) {
      return Promise.resolve(json(historial()));
    }
    if (metodo === "GET" && esLaObra(url)) {
      // El segmento llega codificado en la direccion y el backend lo lee
      // decodificado: aqui se decodifica para reconocer la obra, igual que
      // `useParams` entrega el id ya decodificado a la ficha.
      const id = decodeURIComponent(url.slice("/api/obras/".length));
      return Promise.resolve(
        detalle ? detalle(id) : json({ error: "esa obra no existe" }, 404),
      );
    }
    if (metodo === "GET" && esConsultaDeObras(url)) {
      return Promise.resolve(respuesta ? respuesta() : json(obras(url)));
    }
    return Promise.resolve(json({ error: "ruta no encontrada" }, 404));
  });
}

/**
 * Un backend falso que RESPETA la señal: la sesion contesta, y cada consulta
 * del catalogo queda en vuelo -con su `AbortSignal` anotado- hasta que el test
 * la resuelve. Una peticion cancelada rechaza con el mismo aborto que daria
 * `fetch`; un doble que ignorara la señal dejaria la prueba del aborto pasando
 * sin haber probado nada, porque la respuesta vieja llegaria tarde y el hook la
 * descartaria igual con su bandera.
 */
function servidorQueRespetaLaSenal(rol: Rol) {
  const senales: AbortSignal[] = [];
  const enVuelo: ((respuesta: Response) => void)[] = [];
  vi.mocked(fetch).mockImplementation((entrada, init) => {
    const url = String(entrada);
    if (url === "/api/auth/session") {
      return Promise.resolve(respuestaDeSesion(rol));
    }
    if (!esConsultaDeObras(url)) {
      return Promise.resolve(json({ error: "ruta no encontrada" }, 404));
    }
    const signal = init?.signal ?? undefined;
    if (signal) senales.push(signal);
    return new Promise<Response>((resolver, rechazar) => {
      signal?.addEventListener("abort", () =>
        rechazar(new DOMException("la peticion se cancelo", "AbortError")),
      );
      enVuelo.push(resolver);
    });
  });
  return { senales, enVuelo };
}

/**
 * El detalle de una obra de `lista`, como lo sirve `GET /obras/{id}`.
 *
 * Un identificador que no esta en la lista es 404 y NO el detalle de otra obra:
 * asi, un enlace que apunte al id equivocado hace fallar el test en vez de
 * pintar la ficha de otra obra como si fuera la suya.
 */
function detalleDe(lista: readonly Obra[]): (id: string) => Response {
  return (id) => {
    const obra = lista.find((candidata) => candidata.id === id);
    return obra ? json(obra) : json({ error: "esa obra no existe" }, 404);
  };
}

describe("pantalla de catalogo (integracion con App)", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    setToken("tok");
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("el administrador ve la pantalla y la primera consulta pide la primera pagina", async () => {
    simularServidor({ rol: "administrador", obras: () => [obraCompleta] });

    montarApp("/catalogo");

    await screen.findByRole("table", { name: "Catálogo de obras" });
    expect(screen.getByRole("link", { name: "Catálogo" })).toBeTruthy();
    expect(
      screen.queryByText("Esta pantalla llega en un PR posterior."),
    ).toBeNull();
    expect(
      screen.getByRole("heading", { name: "Catálogo de obras" }),
    ).toBeTruthy();
    expect(consultas()).toEqual(["/api/obras?limite=20"]);

    const encabezados = within(tabla())
      .getAllByRole("columnheader")
      .map((th) => th.textContent);
    expect(encabezados).toEqual([
      "Título",
      "Género",
      "Año",
      "Tipo",
      "IDA",
      "Declaración",
    ]);

    const fila = filaCon("La Casa de las Dos Palmas");
    expect(
      within(fila)
        .getAllByRole("cell")
        .map((td) => td.textContent),
    ).toEqual([
      "La Casa de las Dos Palmas",
      "Drama",
      "1991",
      "serie",
      "IDA-1",
      "Completa100.0000%",
    ]);
  });

  it("la cabecera explica la consecuencia de una declaracion incompleta sin inventar un estado", async () => {
    simularServidor({ rol: "administrador", obras: () => LAS_TRES });

    montarApp("/catalogo");

    await screen.findByRole("table", { name: "Catálogo de obras" });

    // "En reserva" es un acierto de copy -nombra la consecuencia, no el
    // defecto- y se conserva como explicacion. Lo que no puede es ocupar el
    // lugar del enum: el estado que se pinta es el que manda el backend.
    const cabecera = screen
      .getByRole("heading", { name: "Catálogo de obras" })
      .closest("header");
    expect(cabecera?.textContent).toContain("R-04");
    expect(cabecera?.textContent).toContain("en reserva");
    expect(cabecera?.textContent).toContain("nunca se prorratea");

    expect(within(tabla()).getByText("Incompleta")).toBeTruthy();
    expect(within(tabla()).queryByText("En reserva")).toBeNull();
  });

  it("una obra sin declaracion NO se pinta como una declarada incompleta", async () => {
    simularServidor({ rol: "administrador", obras: () => LAS_TRES });

    montarApp("/catalogo");

    await screen.findByRole("table", { name: "Catálogo de obras" });

    // Las dos obras llegan del backend con el mismo estado: la unica diferencia
    // es que una tiene una version vigente y la otra no.
    expect(obraSinDeclaracion.estado_declaracion).toBe("incompleta");
    expect(obraIncompleta.estado_declaracion).toBe("incompleta");
    expect(obraSinDeclaracion.version_vigente).toBeNull();

    const sinDeclaracion = filaCon("Sin Declarar Todavia");
    const incompleta = filaCon("Noche de Bodas");

    // Sobre la obra que nadie declaro no se afirma que tenga una declaracion.
    expect(within(sinDeclaracion).getByText("Sin declaración")).toBeTruthy();
    expect(within(sinDeclaracion).queryByText("Incompleta")).toBeNull();

    expect(within(incompleta).getByText("Incompleta")).toBeTruthy();
    expect(within(incompleta).queryByText("Sin declaración")).toBeNull();

    // Y la suma va en las dos, la que da el backend (cero cuando no hay
    // declaracion, que es lo que el contrato define para ese caso).
    expect(within(sinDeclaracion).getByText("0.0000%")).toBeTruthy();
    expect(within(incompleta).getByText("74.5000%")).toBeTruthy();
  });

  it("la declaracion incompleta se pinta en ambar y nunca como un fallo", async () => {
    simularServidor({ rol: "administrador", obras: () => LAS_TRES });

    montarApp("/catalogo");

    await screen.findByRole("table", { name: "Catálogo de obras" });

    const etiqueta = within(filaCon("Noche de Bodas")).getByText("Incompleta");
    expect(etiqueta.className).toContain("badge-estado-incompleta");
    expect(etiqueta.className).not.toMatch(/error/);

    // Un total por debajo de 100 es un estado valido del negocio (R-04): no es
    // un error de quien declaro, y no hay ningun estado `invalida` en pantalla
    // porque el backend no puede persistirlo.
    expect(document.body.textContent).not.toMatch(/inv[aá]lida/i);
    expect(
      within(filaCon("Noche de Bodas")).getByText("Incompleta").closest("td")
        ?.className,
    ).not.toMatch(/error/);
  });

  it("pinta el estado y la suma tal como llegan, sin deducir uno del otro", async () => {
    // El contrato manda los dos campos y ninguno se deduce del otro: una parte
    // sin IPI deja la declaracion `incompleta` con la suma en 100. Si el cliente
    // derivara un campo del otro -o redondeara la suma-, corregiria al backend
    // y la pantalla dejaria de decir lo que el sistema sabe.
    const discordante = {
      ...obraCompleta,
      id: "obra-8",
      titulo: "Discordante",
      estado_declaracion: "completa",
      suma_porcentajes: 12.5,
      version_vigente: 1,
    } satisfies Obra;
    const casiCien = {
      ...obraCompleta,
      id: "obra-9",
      titulo: "Casi Cien",
      estado_declaracion: "incompleta",
      suma_porcentajes: 99.9999,
      version_vigente: 4,
    } satisfies Obra;
    simularServidor({
      rol: "administrador",
      obras: () => [discordante, casiCien],
    });

    montarApp("/catalogo");

    await screen.findByRole("table", { name: "Catálogo de obras" });

    const filaDiscordante = filaCon("Discordante");
    expect(within(filaDiscordante).getByText("Completa")).toBeTruthy();
    expect(within(filaDiscordante).getByText("12.5000%")).toBeTruthy();

    const filaCasi = filaCon("Casi Cien");
    expect(within(filaCasi).getByText("Incompleta")).toBeTruthy();
    // 99.9999 no se pinta como 100.0000: la celda no puede contradecir el
    // estado que acaba de mandar el backend.
    expect(within(filaCasi).getByText("99.9999%")).toBeTruthy();
    expect(within(filaCasi).queryByText("100.0000%")).toBeNull();
  });

  it("el resumen dice lo que la API sostiene: la pagina y los filtros, sin periodo ni alta de obras", async () => {
    simularServidor({ rol: "administrador", obras: () => LAS_TRES });

    montarApp("/catalogo");

    await screen.findByRole("table", { name: "Catálogo de obras" });

    // Ni el total de coincidencias -la respuesta no lo trae- ni el periodo del
    // mockup, que el contrato no puede acotar para una obra (D-010).
    expect(
      screen.getByText("3 obras en esta página · sin filtros activos"),
    ).toBeTruthy();
    expect(document.body.textContent).not.toContain("2024-S2");

    // El alta de obras tiene endpoint, pero no esta en el alcance de #30: un
    // boton que no hace nada es peor que su ausencia.
    expect(screen.queryByRole("button", { name: /nueva obra/i })).toBeNull();

    // El detalle SI existe ya -es el paso 6 y cada fila enlaza con su obra-,
    // asi que aqui no se afirma su ausencia: el enlace de cada fila y el
    // destino al que lleva se prueban en su propio test, mas abajo.
  });

  it("cada fila enlaza al detalle de SU obra, y el enlace abre esa obra y no otra", async () => {
    simularServidor({
      rol: "administrador",
      obras: () => LAS_TRES,
      detalle: detalleDe(LAS_TRES),
      historial: () => [VERSION_VIGENTE_DE_LA_INCOMPLETA],
    });

    montarApp("/catalogo");
    await screen.findByRole("table", { name: "Catálogo de obras" });

    // El destino se afirma fila por fila, con su identificador. Un enlace que
    // apuntara al detalle de OTRA obra -el error de copiar el `to` y olvidar la
    // variable- se ve igual en una captura, y una prueba que solo contara
    // enlaces tampoco lo veria.
    const destinos = LAS_TRES.map((obra) => [
      obra.titulo,
      within(filaCon(obra.titulo)).getByRole("link").getAttribute("href"),
    ]);
    expect(destinos).toEqual([
      ["La Casa de las Dos Palmas", "/catalogo/obra-1"],
      ["Noche de Bodas", "/catalogo/obra-2"],
      ["Sin Declarar Todavia", "/catalogo/obra-3"],
    ]);

    // El enlace es el titulo, y solo el titulo: el IDA es un identificador que
    // alguien puede necesitar copiar, y una fila entera que navegara se comeria
    // el clic y la seleccion de esa celda.
    const primera = filaCon("La Casa de las Dos Palmas");
    expect(within(primera).getAllByRole("link")).toHaveLength(1);
    const celdaDelIda = within(primera).getByText("IDA-1");
    expect(celdaDelIda.tagName).toBe("TD");
    expect(celdaDelIda.closest("a")).toBeNull();

    // Y sigue el enlace de la SEGUNDA fila, que no es la primera del catalogo:
    // la ficha que aparece es la de esa obra, con su identificador y su
    // declaracion vigente.
    fireEvent.click(within(filaCon("Noche de Bodas")).getByRole("link"));

    expect(ubicacion()).toBe("/catalogo/obra-2");
    expect(
      await screen.findByRole("heading", { name: "Noche de Bodas", level: 1 }),
    ).toBeTruthy();
    expect(screen.getByText("obra-2")).toBeTruthy();
    expect(screen.queryByText("La Casa de las Dos Palmas")).toBeNull();

    const partes = await screen.findByRole("table", {
      name: "Partes de la declaración vigente",
    });
    expect(within(partes).getByText("74.5000%")).toBeTruthy();
  });

  it("la vuelta desde la ficha devuelve al catalogo con la busqueda y la pagina que se dejaron", async () => {
    // Desde que cada fila enlaza con su ficha, buscar una obra y abrirla es el
    // ida y vuelta normal del administrador: si la vuelta cayera en el catalogo
    // entero, la busqueda que acaba de escribir -y la pagina en la que estaba-
    // se perderian y tendria que rehacerla para seguir donde iba.
    const veinte = Array.from({ length: 20 }, (_, i) => ({
      ...obraCompleta,
      id: `obra-${i}`,
      titulo: `Obra ${i}`,
    })) satisfies Obra[];
    simularServidor({
      rol: "administrador",
      obras: () => veinte,
      detalle: detalleDe(veinte),
    });

    // Una busqueda de verdad: un filtro puesto y la segunda pagina.
    montarApp("/catalogo?titulo=Obra&desplazamiento=20");
    await screen.findByRole("table", { name: "Catálogo de obras" });
    expect(consultas()).toEqual([
      "/api/obras?titulo=Obra&limite=20&desplazamiento=20",
    ]);
    await screen.findByText("Obras 21 a 40");

    fireEvent.click(within(filaCon("Obra 3")).getByRole("link"));

    await screen.findByRole("heading", { name: "Obra 3", level: 1 });
    // La ficha no escribe la busqueda en su propia direccion: los filtros y la
    // pagina son parametros del catalogo y esta pantalla no los lee.
    expect(ubicacion()).toBe("/catalogo/obra-3");

    fireEvent.click(screen.getByRole("link", { name: /Volver al catálogo/ }));

    // La vuelta cae en LA MISMA direccion de la que se salio, y la pantalla
    // vuelve a pedir lo mismo: filtro y pagina, no solo el filtro.
    expect(ubicacion()).toBe("/catalogo?titulo=Obra&desplazamiento=20");
    await vi.waitFor(() =>
      expect(ultimaConsulta()).toBe(
        "/api/obras?titulo=Obra&limite=20&desplazamiento=20",
      ),
    );
    expect(screen.getByLabelText("Título")).toHaveProperty("value", "Obra");
    await screen.findByText("Obras 21 a 40");
  });

  it("el identificador viaja codificado y vuelve entero a la consulta de la obra", async () => {
    simularServidor({
      rol: "administrador",
      obras: () => [obraConEspacioEnElId],
      detalle: detalleDe([obraConEspacioEnElId]),
    });

    montarApp("/catalogo");
    await screen.findByRole("table", { name: "Catálogo de obras" });

    const enlace = within(filaCon(obraConEspacioEnElId.titulo)).getByRole(
      "link",
    );
    // El segmento viaja codificado: un id opaco puede traer un caracter que la
    // direccion no admite tal cual -un espacio ya hace invalida la URL-, y sin
    // codificar el `href` sale con ese caracter crudo.
    expect(enlace.getAttribute("href")).toBe("/catalogo/obra%207");

    fireEvent.click(enlace);

    expect(
      await screen.findByRole("heading", {
        name: obraConEspacioEnElId.titulo,
        level: 1,
      }),
    ).toBeTruthy();
    // El id vuelve entero: el segmento decodificado es el que el backend
    // reconoce, asi que se consulta la obra que la fila nombraba.
    expect(consultas()).toContain("/api/obras/obra%207");
    expect(screen.getByText("obra 7")).toBeTruthy();
  });

  it("cada filtro va a la URL y a la consulta con el parametro que acepta el backend", async () => {
    simularServidor({ rol: "administrador", obras: () => [obraCompleta] });

    montarApp("/catalogo");
    await screen.findByRole("table", { name: "Catálogo de obras" });

    // `titulo` es parcial y viaja codificado: un `&` o un espacio no pueden
    // partir la query.
    fireEvent.change(screen.getByLabelText("Título"), {
      target: { value: "Casa & Dos" },
    });
    await vi.waitFor(() =>
      expect(ultimaConsulta()).toBe("/api/obras?titulo=Casa+%26+Dos&limite=20"),
    );

    fireEvent.change(screen.getByLabelText("Género"), {
      target: { value: "Drama" },
    });
    fireEvent.change(screen.getByLabelText("IPI de coautor"), {
      target: { value: "IPI-00000001" },
    });
    fireEvent.change(screen.getByLabelText("Año"), {
      target: { value: "1991" },
    });

    await vi.waitFor(() =>
      expect(ultimaConsulta()).toBe(
        "/api/obras?titulo=Casa+%26+Dos&genero=Drama&anio=1991&ipi=IPI-00000001&limite=20",
      ),
    );
    // Los filtros viven en la URL: recargar, volver atras o compartir el enlace
    // conserva la busqueda. Se comparan los valores decodificados y no el orden
    // de la query, que es el de inserccion y no dice nada.
    const [ruta, query] = (ubicacion() ?? "").split("?");
    expect(ruta).toBe("/catalogo");
    expect(Object.fromEntries(new URLSearchParams(query))).toEqual({
      titulo: "Casa & Dos",
      genero: "Drama",
      anio: "1991",
      ipi: "IPI-00000001",
    });
    await screen.findByText("1 obra en esta página · 4 filtros activos");
  });

  it("el año solo se manda cuando es un entero positivo, y lo dice cuando no filtra", async () => {
    simularServidor({ rol: "administrador", obras: () => [obraCompleta] });

    montarApp("/catalogo");
    await screen.findByRole("table", { name: "Catálogo de obras" });

    fireEvent.change(screen.getByLabelText("Año"), {
      target: { value: "1991" },
    });
    await vi.waitFor(() =>
      expect(ultimaConsulta()).toBe("/api/obras?anio=1991&limite=20"),
    );
    expect(screen.getByLabelText("Año")).toHaveProperty("value", "1991");
    expect(screen.getByText("Año de producción exacto.")).toBeTruthy();

    // Un valor que el backend rechazaria con 400 no se manda. El campo conserva
    // lo tecleado -un `type=number` lo habria descartado sin decirlo- y la
    // ayuda explica que no esta filtrando.
    fireEvent.change(screen.getByLabelText("Año"), {
      target: { value: "1991a" },
    });
    await vi.waitFor(() =>
      expect(ultimaConsulta()).toBe("/api/obras?limite=20"),
    );
    expect(screen.getByLabelText("Año")).toHaveProperty("value", "1991a");
    expect(
      screen.getByText(
        "No se filtra por año mientras el valor no sea un año entero positivo.",
      ),
    ).toBeTruthy();
    expect(consultas().some((url) => url.includes("anio=1991a"))).toBe(false);
    expect(consultas()).not.toContain("/api/obras?anio=1&limite=20");
    expect(consultas()).not.toContain("/api/obras?anio=19&limite=20");
  });

  it("cambiar de filtro vuelve a la primera pagina", async () => {
    const veinte = Array.from({ length: 20 }, (_, i) => ({
      ...obraCompleta,
      id: `obra-${i}`,
      titulo: `Obra ${i}`,
    })) satisfies Obra[];
    simularServidor({ rol: "administrador", obras: () => veinte });

    montarApp("/catalogo?desplazamiento=20");
    await screen.findByRole("table", { name: "Catálogo de obras" });
    expect(consultas()).toEqual(["/api/obras?limite=20&desplazamiento=20"]);

    fireEvent.change(screen.getByLabelText("Título"), {
      target: { value: "Obra 3" },
    });

    // Seguir en el desplazamiento 20 de un resultado que ya es otro dejaria la
    // pantalla vacia por una razon que nada explicaria.
    await vi.waitFor(() =>
      expect(ultimaConsulta()).toBe("/api/obras?titulo=Obra+3&limite=20"),
    );
    expect(ubicacion()).toBe("/catalogo?titulo=Obra+3");
  });

  it("limpiar los filtros los quita todos y vuelve a la primera pagina", async () => {
    simularServidor({ rol: "administrador", obras: () => LAS_TRES });

    montarApp("/catalogo?titulo=Casa&anio=1991&desplazamiento=20");
    await screen.findByRole("table", { name: "Catálogo de obras" });
    expect(screen.getByLabelText("Año")).toHaveProperty("value", "1991");

    fireEvent.click(screen.getByRole("button", { name: "Limpiar filtros" }));

    await vi.waitFor(() =>
      expect(ultimaConsulta()).toBe("/api/obras?limite=20"),
    );
    expect(ubicacion()).toBe("/catalogo");
    // El campo del anio sigue a la URL cuando cambia por fuera de el.
    expect(screen.getByLabelText("Año")).toHaveProperty("value", "");
    expect(screen.getByLabelText("Título")).toHaveProperty("value", "");
    await screen.findByText("3 obras en esta página · sin filtros activos");
  });

  it("la paginacion avanza con desplazamiento y ofrece volver a la anterior", async () => {
    const veinte = Array.from({ length: 20 }, (_, i) => ({
      ...obraCompleta,
      id: `obra-${i}`,
      titulo: `Obra ${i}`,
    })) satisfies Obra[];
    simularServidor({ rol: "administrador", obras: () => veinte });

    montarApp("/catalogo");
    await screen.findByRole("table", { name: "Catálogo de obras" });

    // No hay un total en la respuesta, asi que no se dice "pagina 2 de 7": el
    // boton se ofrece porque la pagina vino entera.
    expect(screen.getByText("Obras 1 a 20")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Anterior" })).toHaveProperty(
      "disabled",
      true,
    );
    const siguiente = screen.getByRole("button", { name: "Siguiente" });
    expect(siguiente).toHaveProperty("disabled", false);

    fireEvent.click(siguiente);

    await vi.waitFor(() =>
      expect(ultimaConsulta()).toBe("/api/obras?limite=20&desplazamiento=20"),
    );
    await screen.findByText("Obras 21 a 40");
    expect(screen.getByRole("button", { name: "Anterior" })).toHaveProperty(
      "disabled",
      false,
    );
  });

  it("un clic de paginacion pide de inmediato, sin esperar al debounce del tecleo", async () => {
    const veinte = Array.from({ length: 20 }, (_, i) => ({
      ...obraCompleta,
      id: `obra-${i}`,
      titulo: `Obra ${i}`,
    })) satisfies Obra[];
    simularServidor({ rol: "administrador", obras: () => veinte });
    montarApp("/catalogo");
    await screen.findByRole("table", { name: "Catálogo de obras" });

    // El reloj lo lleva el test y NUNCA avanza: si la peticion sale, sale sin
    // esperar. Con el debounce sobre todo el `path`, esperaba 250 ms para nada.
    vi.useFakeTimers();
    try {
      const antes = consultas().length;
      fireEvent.click(screen.getByRole("button", { name: "Siguiente" }));

      expect(consultas().length).toBe(antes + 1);
      expect(ultimaConsulta()).toBe("/api/obras?limite=20&desplazamiento=20");
    } finally {
      vi.useRealTimers();
    }
  });

  it("una pagina que vino incompleta no ofrece siguiente", async () => {
    simularServidor({ rol: "administrador", obras: () => LAS_TRES });

    montarApp("/catalogo");
    await screen.findByRole("table", { name: "Catálogo de obras" });

    expect(screen.getByRole("button", { name: "Siguiente" })).toHaveProperty(
      "disabled",
      true,
    );
  });

  it("una pagina avanzada sin obras no afirma que el catalogo este vacio", async () => {
    simularServidor({ rol: "administrador", obras: () => [] });

    montarApp("/catalogo?desplazamiento=40");

    await screen.findByText("Esta página del catálogo no trae obras.");
    // Las paginas anteriores no se consultaron: lo unico que se sabe es que
    // esta no trae nada.
    expect(
      screen.queryByText("El catálogo no tiene obras registradas."),
    ).toBeNull();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("sin filtros y sin obras lo dice, y con filtros distingue que no hubo coincidencias", async () => {
    simularServidor({ rol: "administrador", obras: () => [] });

    montarApp("/catalogo");
    await screen.findByText("El catálogo no tiene obras registradas.");

    fireEvent.change(screen.getByLabelText("Título"), {
      target: { value: "Zzz" },
    });

    await screen.findByText("Ninguna obra coincide con los filtros.");
    expect(
      screen.queryByText("El catálogo no tiene obras registradas."),
    ).toBeNull();

    // Limpiar desde el propio estado vacio devuelve el catalogo entero.
    fireEvent.click(screen.getByRole("button", { name: "limpia los filtros" }));
    await screen.findByText("El catálogo no tiene obras registradas.");
  });

  it("mientras llega la respuesta muestra que esta cargando, sin tabla", async () => {
    vi.mocked(fetch).mockImplementation((entrada) => {
      const url = String(entrada);
      if (url === "/api/auth/session") {
        return Promise.resolve(respuestaDeSesion("administrador"));
      }
      return new Promise<Response>(() => {});
    });

    montarApp("/catalogo");

    await screen.findByRole("heading", { name: "Catálogo de obras" });
    expect(screen.getByRole("status")).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("un 400 del backend se muestra con su mensaje tal cual", async () => {
    // El mensaje literal del contrato para un parametro mal formado. Aunque la
    // pantalla no mande un anio invalido, la URL la escribe cualquiera.
    const mensaje = "anio tiene que ser un entero positivo";
    simularServidor({
      rol: "administrador",
      respuesta: () => json({ error: mensaje }, 400),
    });

    montarApp("/catalogo?anio=1991");

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain("No se pudo consultar el catálogo:");
    expect(alerta.textContent).toContain(mensaje);
    expect(screen.queryByRole("table")).toBeNull();
  });

  // Un 2xx que no trae el listado prometido. `useApi<Obra[]>` no comprueba la
  // forma -`T` es una promesa, no una verificacion- y cada elemento lo revisa
  // `esObra`. Sin ese corte, el primero revienta en `.length` / `.map` y los
  // demas al pintarse: sin ErrorBoundary en `web/src`, la pantalla queda en
  // blanco.
  const CUERPOS_ILEGIBLES: [string, unknown][] = [
    ["un objeto en vez de una lista", { error: "algo salio mal" }],
    ["una lista con un elemento vacio", [{}]],
    [
      "una lista sin `version_vigente` (el backend no lo poblo)",
      [{ ...obraCompleta, version_vigente: undefined }],
    ],
    [
      "una lista con un estado que el sistema no puede producir",
      [{ ...obraCompleta, estado_declaracion: "invalida" }],
    ],
    [
      "una lista con la suma como texto",
      [{ ...obraCompleta, suma_porcentajes: "100" }],
    ],
    [
      "una lista con el IDA como objeto",
      [{ ...obraCompleta, ida: { ida: "IDA-1" } }],
    ],
  ];

  it.each(CUERPOS_ILEGIBLES)(
    "un 2xx con %s deja el catalogo en error, sin pintarlo ni tumbarlo",
    async (_caso, cuerpo) => {
      simularServidor({
        rol: "administrador",
        respuesta: () => json(cuerpo),
      });

      montarApp("/catalogo");

      const alerta = await screen.findByRole("alert");
      expect(alerta.textContent).toContain(
        "El catálogo no llegó como una lista de obras legibles.",
      );
      expect(screen.queryByRole("table")).toBeNull();
      // La pantalla sigue montada: la cabecera y los filtros siguen ahi.
      expect(
        screen.getByRole("heading", { name: "Catálogo de obras" }),
      ).toBeTruthy();
      expect(screen.getByLabelText("Título")).toBeTruthy();

      // Y sobre todo: un payload sin `version_vigente` NO se lee como "esta
      // obra no tiene declaracion". La lista entera queda en error en vez de
      // afirmar sobre una obra algo que el sistema no dijo.
      expect(screen.queryByText("Sin declaración")).toBeNull();
    },
  );

  it("un auditor en /catalogo ve 'No autorizado', sin enlace y sin pedir el catalogo", async () => {
    simularServidor({ rol: "auditor" });

    montarApp("/catalogo");

    await screen.findByRole("heading", { name: "No autorizado" });
    expect(screen.queryByRole("link", { name: "Catálogo" })).toBeNull();
    expect(
      screen.queryByRole("heading", { name: "Catálogo de obras" }),
    ).toBeNull();
    expect(consultas()).toEqual([]);
  });

  // Item 11: el tecleo deja de ser una peticion por tecla y el cambio de filtro
  // cancela la que ya no sirve. Las dos pruebas miden una cosa distinta -cuantas
  // peticiones salen, y si la vieja se cancela-, y por eso cada una tiene su
  // control: la primera, dos tecleos separados mas que la espera; la segunda, la
  // precondicion de que la peticion llego a salir con su señal.
  describe("tecleo y cancelacion (item 11)", () => {
    afterEach(() => {
      // Desmontar con los temporizadores falsos puestos deja la limpieza del
      // efecto escribiendo sobre el reloj de mentira.
      cleanup();
      vi.useRealTimers();
    });

    it("teclear diez caracteres seguidos produce UNA sola peticion, y dos tecleos separados dos", async () => {
      simularServidor({ rol: "administrador", obras: () => [obraCompleta] });

      montarApp("/catalogo");
      await screen.findByRole("table", { name: "Catálogo de obras" });

      // A partir de aqui el reloj lo lleva el test: con temporizadores de
      // verdad no se puede separar "todavia esta tecleando" de "la respuesta
      // llego".
      vi.useFakeTimers();
      const antes = consultas().length;

      // Precondicion del contador: el doble YA vio una peticion -la del
      // montaje-. Un contador que nunca sube mide lo mismo que un debounce que
      // funciona, y este repo ya pago esa trampa con el filtro del driver CDP,
      // que devolvia 0 sin haber medido nada.
      expect(antes).toBeGreaterThanOrEqual(1);

      // Diez teclas, una detras de otra y sin pausa: una palabra escrita.
      const tecleo = [
        "C",
        "Ca",
        "Cas",
        "Casa",
        "Casa ",
        "Casa d",
        "Casa de",
        "Casa de ",
        "Casa de l",
        "Casa de la",
      ];
      for (const texto of tecleo) {
        fireEvent.change(screen.getByLabelText("Título"), {
          target: { value: texto },
        });
      }

      // Mientras el tecleo sigue, no ha salido ninguna peticion...
      expect(consultas().length).toBe(antes);

      await act(async () => {
        await vi.advanceTimersByTimeAsync(DEBOUNCE_TECLEO_MS);
      });

      // ...y cuando para, sale UNA: la palabra entera, no las nueve anteriores.
      expect(consultas().length).toBe(antes + 1);
      expect(ultimaConsulta()).toBe("/api/obras?titulo=Casa+de+la&limite=20");

      // Control negativo: dos tecleos SEPARADOS mas que la espera son dos
      // peticiones. Sin el, un contador que no sube nunca -o un debounce que no
      // agrupara nada- pasaria la afirmacion de arriba igual.
      fireEvent.change(screen.getByLabelText("Título"), {
        target: { value: "Casa de la P" },
      });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(DEBOUNCE_TECLEO_MS + 1);
      });
      fireEvent.change(screen.getByLabelText("Título"), {
        target: { value: "Casa de la Pa" },
      });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(DEBOUNCE_TECLEO_MS + 1);
      });

      expect(consultas()).toEqual([
        "/api/obras?limite=20",
        "/api/obras?titulo=Casa+de+la&limite=20",
        "/api/obras?titulo=Casa+de+la+P&limite=20",
        "/api/obras?titulo=Casa+de+la+Pa&limite=20",
      ]);
    });

    it("cambiar de filtro aborta la peticion anterior, y la nueva llega igual", async () => {
      const servidor = servidorQueRespetaLaSenal("administrador");

      montarApp("/catalogo?titulo=Casa");

      // Precondicion: la peticion del primer filtro SALIO, con su señal, y
      // sigue en vuelo. El doble no resuelve hasta que el test lo diga, asi que
      // el "antes de que la primera resuelva" no depende de ningun reloj.
      await vi.waitFor(() => expect(servidor.senales).toHaveLength(1));
      expect(servidor.senales[0].aborted).toBe(false);

      fireEvent.change(screen.getByLabelText("Título"), {
        target: { value: "Casa de la" },
      });

      // El cambio de filtro cancela la peticion que ya no sirve: sin esto sale
      // igual, consume servidor y puede llegar tarde sobre la nueva.
      await vi.waitFor(() => expect(servidor.senales[0].aborted).toBe(true));
      await vi.waitFor(() => expect(servidor.senales).toHaveLength(2));
      expect(servidor.senales[1].aborted).toBe(false);
      expect(servidor.senales[1]).not.toBe(servidor.senales[0]);

      // Y la nueva llega y se pinta: el aborto de la vieja no deja la pantalla
      // cargando para siempre ni se convierte en un error en pantalla.
      await act(async () => {
        servidor.enVuelo[1](json([obraCompleta]));
      });

      await screen.findByRole("table", { name: "Catálogo de obras" });
      expect(ultimaConsulta()).toBe("/api/obras?titulo=Casa+de+la&limite=20");
      expect(screen.queryByRole("alert")).toBeNull();
    });
  });
});
