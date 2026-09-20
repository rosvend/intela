import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { MemoryRouter, type MemoryRouterProps } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "../App";
import { setToken } from "../api";
import { ProveedorDeSesion, type Rol } from "../sesion";
import { CLAVE_DE_VUELTA_AL_CATALOGO } from "./Catalogo";
import { ROTULO_NOMBRE_EN_PADRON_ACTUAL } from "./declaracion";
import type { Obra, Titular, VersionDeclaracion } from "./tipos";

function json(cuerpo: unknown, status = 200): Response {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

// Fixtures tipados con el contrato generado: si `Obra` o `VersionDeclaracion`
// cambian en api/openapi.yaml, `tsc` rompe aqui antes que en pantalla. Nombres y
// titulos sinteticos, no de ningun canal real.
const obraDeclarada = {
  id: "obra-1",
  titulo: "La Casa de las Dos Palmas",
  genero: "Drama",
  anio: 1991,
  tipo: "serie",
  ida: "IDA-1",
  coautores: [
    { nombre: "Ana Escritora", ipi: "IPI-00000001", rol: "guionista" },
  ],
  // La obra declara HOY completo y su version vigente es la 3. La version 2 de
  // abajo esta `incompleta`, y es a proposito: el estado de una version pasada
  // es un hecho historico, no el de la obra, asi que la pantalla tiene que
  // pintar el de la version -sustituirlo por el de hoy seria afirmar de una
  // version cerrada algo que el sistema no sostiene (D-008)-. Con los dos
  // iguales, este test no distinguiria una cosa de la otra.
  estado_declaracion: "completa",
  suma_porcentajes: 100,
  version_vigente: 3,
} satisfies Obra;

const obraSinDeclaracion = {
  ...obraDeclarada,
  id: "obra-3",
  titulo: "Sin Declarar Todavia",
  ida: undefined,
  estado_declaracion: "incompleta",
  suma_porcentajes: 0,
  version_vigente: null,
} satisfies Obra;

// Las tres versiones de la obra declarada, en el orden en que las devuelve el
// servidor: de la mas antigua a la mas reciente. La ventana de la 2 y la de la 3
// empiezan el MISMO dia y a la misma hora, y solo las separan microsegundos
// (`RFC3339Nano`, el caso que el plan nombra): el texto legible de las dos es
// igual, asi que lo que las distingue en pantalla es el numero de version y el
// instante exacto que va en `<time datetime>`.
const version1: VersionDeclaracion = {
  version: 1,
  vigente_desde: "2026-01-15T10:00:00Z",
  vigente_hasta: "2026-02-01T08:30:00Z",
  estado: "completa",
  partes: [
    { titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: 60 },
    { titular_id: "tit-2", ipi: "IPI-00000002", porcentaje: 40 },
  ],
};

const version2: VersionDeclaracion = {
  version: 2,
  vigente_desde: "2026-03-01T09:00:00.000001Z",
  vigente_hasta: "2026-03-01T09:00:00.000002Z",
  estado: "incompleta",
  partes: [{ titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: 74.5 }],
};

const version3: VersionDeclaracion = {
  version: 3,
  vigente_desde: "2026-03-01T09:00:00.000003Z",
  vigente_hasta: null,
  estado: "completa",
  partes: [
    { titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: 75 },
    { titular_id: "tit-2", ipi: "IPI-00000002", porcentaje: 25 },
  ],
};

const HISTORIAL: VersionDeclaracion[] = [version1, version2, version3];

/**
 * El mismo historial con dos versiones mas. La quinta trae ademas un titular que
 * NO aparece en ninguna otra -`tit-3`-, y no es un adorno: sin el, la union de los
 * identificadores de todas las versiones coincide con la de la PRIMERA, asi que
 * una consulta que solo mirara la version 1 pasaria la prueba. Con el, la
 * asercion sobre la URL distingue "la union de todas" de "las de la primera", que
 * es la propiedad que el item 9b promete.
 */
const HISTORIAL_LARGO: VersionDeclaracion[] = [
  ...HISTORIAL,
  {
    ...version3,
    version: 4,
    vigente_desde: "2026-04-01T09:00:00Z",
    vigente_hasta: "2026-04-02T09:00:00Z",
  },
  {
    ...version3,
    version: 5,
    vigente_desde: "2026-05-01T09:00:00Z",
    vigente_hasta: null,
    partes: [
      { titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: 70 },
      { titular_id: "tit-2", ipi: "IPI-00000002", porcentaje: 25 },
      { titular_id: "tit-3", ipi: "IPI-00000003", porcentaje: 5 },
    ],
  },
];

/**
 * El padron por el que esta pantalla resuelve el nombre de las partes (item 9b):
 * los dos `titular_id` que aparecen en las tres versiones. Los nombres son
 * sinteticos, como todo lo de este fichero.
 */
const PADRON: Titular[] = [
  {
    id: "tit-1",
    nombre: "Ana Escritora",
    ipi: "IPI-00000001",
    persona_natural: true,
    clase: "socio",
  },
  {
    id: "tit-2",
    nombre: "Luis Guionista",
    ipi: "IPI-00000002",
    persona_natural: true,
    clase: "administrado",
  },
];

function respuestaDeSesion(rol: Rol): Response {
  return json({
    id: "usr-1",
    email: "x@redes.co",
    nombre: "Persona de Prueba",
    rol,
    titular_id: "",
  });
}

const esLaObra = (url: string) => /^\/api\/obras\/[^/]+$/.test(url);
const esElHistorial = (url: string) =>
  /^\/api\/obras\/[^/]+\/declaracion\/historial(\?.*)?$/.test(url);

/**
 * Un backend falso que responde por URL y metodo: la sesion con el rol del test,
 * la obra con `obra()` y su historial con `historial()`. `catalogo()` responde al
 * listado -lo pide la pantalla del catalogo cuando se vuelve a ella- y `padron()`
 * al padron de titulares, que es por donde esta pantalla resuelve el nombre de las
 * partes (item 9b). **Sin `padron()` el padron responde 404**, que es lo que hace
 * falta en las pruebas que no miran esa columna: las tablas se pintan igual, con
 * el guion.
 */
function simularServidor({
  rol = "administrador",
  obra = () => json(obraDeclarada),
  historial = () => json(HISTORIAL),
  catalogo,
  padron,
}: {
  rol?: Rol;
  obra?: () => Response;
  historial?: () => Response;
  catalogo?: () => Response;
  padron?: () => Response;
} = {}) {
  vi.mocked(fetch).mockImplementation((entrada, init) => {
    const url = String(entrada);
    const metodo = init?.method ?? "GET";
    if (url === "/api/auth/session") {
      return Promise.resolve(respuestaDeSesion(rol));
    }
    if (metodo === "GET" && esLaObra(url)) {
      return Promise.resolve(obra());
    }
    if (metodo === "GET" && esElHistorial(url)) {
      return Promise.resolve(historial());
    }
    if (metodo === "GET" && padron && url.startsWith("/api/titulares")) {
      return Promise.resolve(padron());
    }
    if (metodo === "GET" && catalogo && url.startsWith("/api/obras?")) {
      return Promise.resolve(catalogo());
    }
    return Promise.resolve(json({ error: "ruta no encontrada" }, 404));
  });
}

/** Los GET que la pantalla hizo contra `/api/obras`, en orden. */
function consultas(): string[] {
  return vi
    .mocked(fetch)
    .mock.calls.map(([entrada, init]) => ({
      url: String(entrada),
      metodo: init?.method ?? "GET",
    }))
    .filter((l) => l.metodo === "GET" && l.url.startsWith("/api/obras"))
    .map((l) => l.url);
}

/**
 * TODOS los GET que la pantalla hizo, sin recortar por recurso.
 *
 * Hace falta porque `consultas()` filtra a `/api/obras` **a proposito** -varias
 * aserciones de este fichero cuentan con ese filtro- y con ese filtro una
 * afirmacion sobre otra ruta no puede ser verdadera: el
 * `expect(consultas().some((url) => url.includes("/titulares"))).toBe(false)` que
 * habia aqui era vacuo por eso, y no porque la pantalla no pidiera el padron.
 */
function todasLasConsultas(): string[] {
  return vi
    .mocked(fetch)
    .mock.calls.map(([entrada, init]) => ({
      url: String(entrada),
      metodo: init?.method ?? "GET",
    }))
    .filter((l) => l.metodo === "GET")
    .map((l) => l.url);
}

/** Las consultas al padron: lo que el item 9b promete en UNA peticion. */
function consultasAlPadron(): string[] {
  return todasLasConsultas().filter((url) => url.startsWith("/api/titulares"));
}

/**
 * Un respiro para que una peticion de mas tenga tiempo de salir antes de
 * contarla: la consulta sale en un efecto, asi que "sigue habiendo una sola" no
 * se puede afirmar en el mismo tick en que se pintaron los nombres.
 */
const respirar = () => new Promise((listo) => setTimeout(listo, 60));

/**
 * Una entrada del historial del router en memoria: la direccion, y el estado que
 * traiga. El estado es lo que distingue haber llegado desde el detalle -que a su
 * vez lo recibio del catalogo- de haber abierto la pantalla por su direccion.
 */
type Entrada = NonNullable<MemoryRouterProps["initialEntries"]>[number];

function montarApp(entrada: Entrada) {
  return render(
    <MemoryRouter initialEntries={[entrada]}>
      <ProveedorDeSesion>
        <App />
      </ProveedorDeSesion>
    </MemoryRouter>,
  );
}

/**
 * La pantalla, sin el shell: el sidebar tiene sus propios enlaces y su boton de
 * salir, y buscarlos sueltos haria que los tests dijeran cosas del armazon en vez
 * de decir cosas del historial.
 */
function pantallaDelHistorial(): HTMLElement {
  const titulo = screen.queryByRole("heading", {
    name: "Historial de la declaración",
  });
  const seccion = titulo?.closest("section");
  if (!seccion) throw new Error("la pantalla del historial no esta montada");
  return seccion;
}

/** El bloque de la version `n`, que es donde viven su estado y su ventana. */
function bloqueDeVersion(n: number): HTMLElement {
  const titulo = screen.getByRole("heading", {
    name: `Versión ${n}`,
    level: 2,
  });
  const bloque = titulo.closest("article");
  if (!bloque) throw new Error(`"Versión ${n}" no esta dentro de un bloque`);
  return bloque;
}

/** La celda de un dato con rotulo, dentro del bloque de una version. */
function dato(bloque: HTMLElement, rotulo: string): HTMLElement {
  const contenedor = within(bloque).getByText(rotulo).closest("div");
  if (!contenedor) throw new Error(`"${rotulo}" no esta dentro de un dato`);
  return contenedor;
}

/** El instante que ese dato pinta, con el valor exacto que le llego. */
function instante(bloque: HTMLElement, rotulo: string): HTMLTimeElement {
  const elemento = dato(bloque, rotulo).querySelector("time");
  if (!elemento) throw new Error(`"${rotulo}" no pinto ningun instante`);
  return elemento;
}

/** La tabla de partes de la version `n`. */
function tablaDeVersion(n: number): HTMLElement {
  return screen.getByRole("table", { name: `Partes de la versión ${n}` });
}

/**
 * Lo que la fila de ese titular dice, en la tabla de la version `n`, en su
 * columna de NOMBRE -la tercera-.
 *
 * Se lee entera -y no con un `getByText`- porque los dos valores que puede tener
 * son un nombre y el guion de "no se conoce", y una busqueda por texto no
 * distingue "no pinto nada" de "pinto el guion".
 */
function nombreDeLaFilaDeLaVersion(n: number, titularId: string): string {
  const celda = within(tablaDeVersion(n)).getByText(titularId, {
    selector: "td",
  });
  const fila = celda.closest("tr");
  if (!fila) throw new Error(`"${titularId}" no esta dentro de una fila`);
  const deLaColumna = within(fila).getAllByRole("cell")[2];
  if (!deLaColumna) throw new Error("la fila no tiene columna de nombre");
  return deLaColumna.textContent ?? "";
}

/** Los encabezados de una tabla, como texto. */
function encabezadosDe(tabla: HTMLElement): (string | null)[] {
  return within(tabla)
    .getAllByRole("columnheader")
    .map((th) => th.textContent);
}

/**
 * Espera a que las versiones esten pintadas.
 *
 * El titulo de la pantalla llega con la obra, y las versiones llegan en la
 * peticion SIGUIENTE, la del historial: quien mire bloques o tablas tiene que
 * esperar a esto y no al titulo.
 */
async function esperarLasVersiones(): Promise<void> {
  await screen.findByRole("heading", { name: "Versión 1", level: 2 });
}

describe("historial de versiones (integracion con App)", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    setToken("tok");
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("el administrador ve todas las versiones, de la mas antigua a la mas reciente", async () => {
    simularServidor();

    montarApp("/catalogo/obra-1/historial");

    await screen.findByRole("heading", { name: "Historial de la declaración" });
    await esperarLasVersiones();

    // La obra, para saber de que historial es y para distinguir "sin versiones"
    // de "esa obra no esta"; y despues el historial, que es donde estan las
    // versiones. En ese orden: el historial no se pide hasta que la obra se lee.
    expect(consultas()).toEqual([
      "/api/obras/obra-1",
      "/api/obras/obra-1/declaracion/historial",
    ]);

    // De que obra es: el titulo, y el identificador para poder conciliarlo con
    // la API.
    expect(screen.getByText(/La Casa de las Dos Palmas/)).toBeTruthy();
    expect(screen.getByText(/obra-1/)).toBeTruthy();

    // Las tres versiones, en el ORDEN en que las devolvio el servidor: la
    // pantalla no las reordena ni las renumera.
    expect(
      screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent),
    ).toEqual(["Versión 1", "Versión 2", "Versión 3"]);

    // Cada una con SU estado: el de la version 2 es `incompleta` aunque la obra
    // declare hoy completo. Sustituirlo por el de la obra -o re-derivarlo de las
    // partes- diria de una version cerrada algo que el sistema no sostiene.
    expect(version2.estado).toBe("incompleta");
    expect(obraDeclarada.estado_declaracion).toBe("completa");
    const incompleta = within(bloqueDeVersion(2)).getByText("Incompleta");
    expect(incompleta.className).toContain("badge-estado-incompleta");
    // Ambar, nunca rojo: no sumar 100 no es un error, y el rojo esta reservado a
    // lo que de verdad falla.
    expect(incompleta.className).not.toMatch(/error/);
    expect(within(bloqueDeVersion(1)).getByText("Completa")).toBeTruthy();
    expect(within(bloqueDeVersion(3)).getByText("Completa")).toBeTruthy();
    // Ni un `invalida`, que el backend no puede persistir.
    expect(document.body.textContent).not.toMatch(/inv[aá]lida/i);
    expect(screen.queryByRole("alert")).toBeNull();

    // Y con su reparto: una tabla por version, cada una con su nombre accesible.
    expect(
      within(tablaDeVersion(1))
        .getAllByRole("row")
        .slice(1)
        .map((fila) =>
          within(fila)
            .getAllByRole("cell")
            .map((td) => td.textContent),
        ),
    ).toEqual([
      ["tit-1", "IPI-00000001", "—", "60.0000%"],
      ["tit-2", "IPI-00000002", "—", "40.0000%"],
    ]);
    expect(within(tablaDeVersion(2)).getByText("74.5000%")).toBeTruthy();
    expect(within(tablaDeVersion(3)).getAllByRole("row")).toHaveLength(3);
  });

  it("distingue la version que rige hoy de las cerradas, con el instante exacto", async () => {
    simularServidor();

    montarApp("/catalogo/obra-1/historial");
    await screen.findByRole("heading", { name: "Historial de la declaración" });
    await esperarLasVersiones();

    // La que rige hoy es la que el servidor manda sin cerrar, y lo dice con
    // palabras: `vigente_hasta` en `null` enseñado como un hueco dejaria al
    // lector sin saber si el dato no llego o si la version sigue abierta.
    expect(version3.vigente_hasta).toBeNull();
    expect(
      within(dato(bloqueDeVersion(3), "Vigente hasta")).getByText("Sin cerrar"),
    ).toBeTruthy();
    expect(screen.getAllByText("Sin cerrar")).toHaveLength(1);
    expect(
      dato(bloqueDeVersion(3), "Vigente hasta").querySelector("time"),
    ).toBeNull();

    // Las cerradas llevan las DOS fechas, y cada una es exactamente el instante
    // que mando el servidor: el texto legible baja a minutos, y dos versiones
    // abiertas el mismo dia solo se distinguen por los microsegundos.
    for (const version of [version1, version2]) {
      expect(
        instante(bloqueDeVersion(version.version), "Vigente desde").dateTime,
      ).toBe(version.vigente_desde);
      expect(
        instante(bloqueDeVersion(version.version), "Vigente hasta").dateTime,
      ).toBe(version.vigente_hasta);
    }

    // Los dos instantes que solo se separan por microsegundos llegan enteros, y
    // la pantalla no confunde el de una version con el de la vecina.
    expect(instante(bloqueDeVersion(2), "Vigente desde").dateTime).toBe(
      "2026-03-01T09:00:00.000001Z",
    );
    expect(instante(bloqueDeVersion(3), "Vigente desde").dateTime).toBe(
      "2026-03-01T09:00:00.000003Z",
    );
  });

  it("una obra sin ninguna version declarada lo dice, y no como un fallo", async () => {
    // Una obra sin declaracion devuelve lista VACIA, no 404: el hecho es que no
    // hay ninguna version, y la pantalla no puede confundirlo con no haber
    // podido cargar.
    simularServidor({
      obra: () => json(obraSinDeclaracion),
      historial: () => json([]),
    });

    montarApp("/catalogo/obra-3/historial");

    await screen.findByText("Esta obra no tiene ninguna versión declarada.");

    // Se dijo el hecho -y de que obra-, y se dijo que no es un error.
    expect(screen.getByText(/Sin Declarar Todavia/)).toBeTruthy();
    expect(document.body.textContent).toMatch(
      /no es un fallo de esta pantalla/i,
    );
    // Sin alerta -no hay nada roto-, sin estado de carga, sin tabla de partes y
    // sin mensaje de error.
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.queryByText(/No se pudo consultar/)).toBeNull();
    // Y sin ninguna etiqueta de estado: no hay version de la que decir un
    // estado, y no se inventa la de la obra.
    expect(document.querySelectorAll(".badge-estado")).toHaveLength(0);
    // El historial SI se pidio: es su respuesta -vacia- la que se esta contando.
    expect(consultas()).toEqual([
      "/api/obras/obra-3",
      "/api/obras/obra-3/declaracion/historial",
    ]);
  });

  it("una version sin partes lo dice en vez de dejar la tabla vacia", async () => {
    simularServidor({ historial: () => json([{ ...version3, partes: [] }]) });

    montarApp("/catalogo/obra-1/historial");

    await screen.findByText("Esta versión no trae ninguna parte declarada.");
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("las partes llevan el titular_id y el nombre del padron, con UNA peticion para toda la pantalla", async () => {
    simularServidor({ padron: () => json(PADRON) });

    montarApp("/catalogo/obra-1/historial");
    await screen.findByRole("heading", { name: "Historial de la declaración" });
    await esperarLasVersiones();
    await within(tablaDeVersion(1)).findByText("Ana Escritora");

    // El rotulo dice de donde sale el nombre -del padron de HOY, no de la fecha
    // de la version (D-006)-, y el `titular_id` esta al lado para poder conciliar
    // la pantalla con la API. Con mas motivo en una version antigua: el nombre de
    // hoy no es el que el titular tenia entonces.
    expect(encabezadosDe(tablaDeVersion(2))).toEqual([
      "Titular",
      "IPI",
      ROTULO_NOMBRE_EN_PADRON_ACTUAL,
      "Porcentaje",
    ]);
    const fila = within(tablaDeVersion(2)).getAllByRole("row")[1];
    if (!fila) throw new Error("la version 2 no pinto ninguna fila");
    // La tercera celda es el NOMBRE de hoy, resuelto contra el padron: la version
    // 2 es de marzo y el nombre es el que el titular tiene ahora (D-006).
    expect(
      within(fila)
        .getAllByRole("cell")
        .map((td) => td.textContent),
    ).toEqual(["tit-1", "IPI-00000001", "Ana Escritora", "74.5000%"]);
    expect(nombreDeLaFilaDeLaVersion(3, "tit-2")).toBe("Luis Guionista");

    // UNA peticion para la pantalla ENTERA, y con los identificadores de todas
    // sus versiones sin repetir: `tit-1` sale en las tres y se pide una vez. Una
    // peticion por tabla seria una por version, y su numero creceria con el
    // historial.
    expect(consultasAlPadron()).toEqual(["/api/titulares?ids=tit-1&ids=tit-2"]);
    await respirar();
    expect(consultasAlPadron()).toHaveLength(1);

    // Y la columna no afirma que un titular falte: el guion es "no se conoce".
    expect(document.body.textContent).not.toMatch(/no existe/i);
    expect(document.body.textContent).not.toMatch(/no est[aá] en el padron/i);
  });

  it("mas versiones NO son mas peticiones: cinco versiones, una sola consulta al padron", async () => {
    simularServidor({
      historial: () => json(HISTORIAL_LARGO),
      padron: () => json(PADRON),
    });

    montarApp("/catalogo/obra-1/historial");
    const deLaQuinta = await screen.findByRole("table", {
      name: "Partes de la versión 5",
    });
    await within(deLaQuinta).findByText("Ana Escritora");
    await respirar();

    // Es la propiedad que el item 9b promete y la que el defecto habria roto: una
    // version mas -y un titular NUEVO en ella- no anade otra consulta, anade un
    // identificador a la que ya se hace. Los tres viajan en la misma URL.
    expect(consultasAlPadron()).toEqual([
      "/api/titulares?ids=tit-1&ids=tit-2&ids=tit-3",
    ]);
    // Y el titular que el padron no tiene se queda con el guion, en esa misma
    // respuesta: el guion no significa que falte, significa que no se conoce.
    expect(nombreDeLaFilaDeLaVersion(5, "tit-3")).toBe("—");
  });

  it("con ninguna parte que nombrar no se pide el padron", async () => {
    // Control negativo de `ids`: vacio NO es "ninguno" en el contrato, es el
    // padron ENTERO, asi que una consulta sin identificadores no puede salir. Por
    // eso la pantalla monta sin nombres en vez de pedirlos con una lista vacia.
    simularServidor({ historial: () => json([{ ...version3, partes: [] }]) });

    montarApp("/catalogo/obra-1/historial");
    await screen.findByText("Esta versión no trae ninguna parte declarada.");
    await respirar();

    expect(consultasAlPadron()).toEqual([]);
    expect(todasLasConsultas()).not.toContain("/api/titulares");
  });

  it("la tabla de partes dice lo mismo en el detalle y en el historial", async () => {
    // Las dos pantallas montan el MISMO componente, y esto es lo que las ata: si
    // alguien vuelve a escribir el mapeo de las partes en una de las dos -una
    // copia local, que es la forma exacta en que esto estaba escrito antes-, la
    // otra sigue con la compartida y las dos tablas dejan de coincidir.
    simularServidor();
    montarApp("/catalogo/obra-1");
    await screen.findByRole("table", {
      name: "Partes de la declaración vigente",
    });
    const enElDetalle = encabezadosDe(
      screen.getByRole("table", { name: "Partes de la declaración vigente" }),
    );

    cleanup();
    simularServidor();
    montarApp("/catalogo/obra-1/historial");
    await screen.findByRole("heading", { name: "Historial de la declaración" });
    await esperarLasVersiones();
    const enElHistorial = encabezadosDe(tablaDeVersion(3));

    expect(enElHistorial).toEqual(enElDetalle);
    expect(enElHistorial).toContain(ROTULO_NOMBRE_EN_PADRON_ACTUAL);
  });

  it("un 404 de la obra se dice como lo que es, aunque su historial ya se haya pedido", async () => {
    // El historial se pide junto a la obra, en el mismo tick, porque solo
    // depende del `id` de la ruta. El de una obra que no existe llega como lista
    // vacia, no como 404: sin leer la obra, esa lista vacia se pintaria como
    // "esta obra no tiene ninguna version", que es afirmar algo de una obra que
    // no esta. Por eso lo que decide es la OBRA, y el historial no se pinta.
    simularServidor({
      obra: () => json({ error: "esa obra no esta en el catalogo" }, 404),
    });

    montarApp("/catalogo/obra-1/historial");

    await screen.findByRole("heading", {
      name: "Esa obra no está en el catálogo",
    });
    expect(screen.getByText("obra-1")).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.queryByRole("table")).toBeNull();
    expect(document.body.textContent).not.toMatch(/no tiene ninguna versión/i);
    expect(consultas()).toEqual([
      "/api/obras/obra-1",
      "/api/obras/obra-1/declaracion/historial",
    ]);
    // La vuelta no es la ficha -volveria a decir lo mismo-, sino el catalogo.
    expect(
      screen
        .getByRole("link", { name: /Volver al catálogo/ })
        .getAttribute("href"),
    ).toBe("/catalogo");
  });

  it("un fallo al leer el historial se muestra con el mensaje del backend", async () => {
    simularServidor({
      historial: () =>
        json({ error: "no se pudo consultar el historial" }, 500),
    });

    montarApp("/catalogo/obra-1/historial");

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain(
      "No se pudo consultar el historial de la declaración:",
    );
    expect(alerta.textContent).toContain("no se pudo consultar el historial");

    // Lo que si llego se sigue viendo: de que obra es el historial viene de otra
    // lectura, y esconderlo por un fallo ajeno seria negar lo que el sistema ya
    // dijo. Lo que no se pinta son las versiones.
    expect(
      screen.getByRole("heading", { name: "Historial de la declaración" }),
    ).toBeTruthy();
    expect(screen.getByText(/La Casa de las Dos Palmas/)).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.queryByText(/Versión 3/)).toBeNull();
  });

  it("un fallo al leer la obra se muestra con el mensaje del backend", async () => {
    simularServidor({ obra: () => json({ error: "la base esta caida" }, 500) });

    montarApp("/catalogo/obra-1/historial");

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain("No se pudo consultar la obra:");
    expect(alerta.textContent).toContain("la base esta caida");
    expect(consultas()).toEqual([
      "/api/obras/obra-1",
      "/api/obras/obra-1/declaracion/historial",
    ]);
  });

  it("la obra y su historial se piden en el mismo tick, no uno detras del otro", async () => {
    simularServidor();
    // La obra NO resuelve nunca: si el historial saliera despues de ella, no
    // saldria, y esta prueba lo vería.
    const responder = vi.mocked(fetch).getMockImplementation();
    vi.mocked(fetch).mockImplementation((entrada, init) =>
      String(entrada) === "/api/obras/obra-1"
        ? new Promise<Response>(() => {})
        : (responder as typeof fetch)(entrada, init),
    );

    montarApp("/catalogo/obra-1/historial");

    await vi.waitFor(() =>
      expect(consultas()).toEqual([
        "/api/obras/obra-1",
        "/api/obras/obra-1/declaracion/historial",
      ]),
    );
  });

  it("mientras llega la respuesta dice que esta cargando, sin versiones", async () => {
    vi.mocked(fetch).mockImplementation((entrada) => {
      if (String(entrada) === "/api/auth/session") {
        return Promise.resolve(respuestaDeSesion("administrador"));
      }
      return new Promise<Response>(() => {});
    });

    montarApp("/catalogo/obra-1/historial");

    await screen.findByRole("link", { name: "Catálogo" });
    expect(screen.getByRole("status")).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.queryByText(/Versión 1/)).toBeNull();
  });

  it("es de solo lectura: no ofrece ningun control ni enlaza al editor", async () => {
    simularServidor();

    montarApp("/catalogo/obra-1/historial");
    await screen.findByRole("heading", { name: "Historial de la declaración" });
    await esperarLasVersiones();

    // El contrato dice de si mismo que no hay forma de editar una version
    // pasada, unicamente de abrir una nueva, y esa pantalla es la ficha (paso
    // 8). Un boton o un formulario aqui sugeriria lo contrario.
    const pantalla = pantallaDelHistorial();
    expect(within(pantalla).queryByRole("button")).toBeNull();
    expect(within(pantalla).queryByRole("textbox")).toBeNull();
    expect(pantalla.querySelector("form")).toBeNull();
    // Y el unico enlace que dibuja es la vuelta a la obra: ni al editor, ni a
    // ninguna otra ruta de #30.
    expect(
      within(pantalla)
        .getAllByRole("link")
        .map((enlace) => enlace.getAttribute("href")),
    ).toEqual(["/catalogo/obra-1"]);
  });

  it("un auditor no ve el historial: el guard de rol se hereda por el prefijo de la ruta", async () => {
    simularServidor({ rol: "auditor" });

    montarApp("/catalogo/obra-1/historial");

    await screen.findByRole("heading", { name: "No autorizado" });
    expect(
      screen.queryByRole("heading", { name: "Historial de la declaración" }),
    ).toBeNull();
    expect(consultas()).toEqual([]);
  });

  it("un 2xx que no trae la lista prometida deja el historial en error, sin tumbarlo", async () => {
    simularServidor({ historial: () => json({ error: "algo salio mal" }) });

    montarApp("/catalogo/obra-1/historial");

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain(
      "El historial no llegó como una lista de versiones legibles.",
    );
    expect(screen.queryByRole("table")).toBeNull();
  });

  // Los campos que lee esta pantalla y que hasta el paso 7 no leia nadie:
  // `vigente_desde` y `estado`. Los comprueba `esVersionDeclaracion` -la guarda
  // del tipo, compartida con el detalle de obra-, asi que un cuerpo al que le
  // falte uno se corta antes de pintarse.
  const HISTORIALES_ILEGIBLES: [string, unknown][] = [
    [
      "una version sin `vigente_desde` (el backend no lo poblo)",
      [{ ...version3, vigente_desde: undefined }],
    ],
    [
      "una version con `estado` fuera del enum",
      [{ ...version3, estado: "invalida" }],
    ],
    ["una version sin `estado`", [{ ...version3, estado: undefined }]],
    [
      "una version con `vigente_desde` como objeto",
      [{ ...version3, vigente_desde: { instante: "ayer" } }],
    ],
  ];

  it.each(HISTORIALES_ILEGIBLES)(
    "un 2xx con %s no se pinta: el historial entero queda en error",
    async (_caso, historial) => {
      simularServidor({ historial: () => json(historial) });

      montarApp("/catalogo/obra-1/historial");

      const alerta = await screen.findByRole("alert");
      expect(alerta.textContent).toContain(
        "El historial no llegó como una lista de versiones legibles.",
      );
      expect(screen.queryByRole("table")).toBeNull();
      // En particular: un estado que el sistema no puede producir NO se pinta
      // como "Incompleta", y una version sin fecha no se pinta sin fecha.
      expect(document.body.textContent).not.toMatch(/inv[aá]lida/i);
      expect(document.body.textContent).not.toMatch(/Versión 3/);
    },
  );

  it("el detalle enlaza con el historial de SU obra, y con ese id se llega", async () => {
    // S4 del issue #30: la version anterior sigue visible en el historial. Esta
    // es la mitad que suele faltar -que la pantalla se pueda alcanzar-, y va por
    // obra: un enlace que llevara al historial de otra obra pasaria un test que
    // solo mirara el texto del enlace.
    simularServidor();

    montarApp("/catalogo/obra-1");
    await screen.findByRole("table", {
      name: "Partes de la declaración vigente",
    });

    const enlace = screen.getByRole("link", {
      name: "Ver el historial completo",
    });
    expect(enlace.getAttribute("href")).toBe("/catalogo/obra-1/historial");

    fireEvent.click(enlace);

    // Y llega a ELLA: la pantalla del historial de esa obra, con sus tres
    // versiones pedidas por su id.
    await screen.findByRole("heading", { name: "Historial de la declaración" });
    await esperarLasVersiones();
    expect(screen.getByText(/La Casa de las Dos Palmas/)).toBeTruthy();
    expect(consultas()).toContain("/api/obras/obra-1/declaracion/historial");
    expect(
      screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent),
    ).toEqual(["Versión 1", "Versión 2", "Versión 3"]);
  });

  it("cada obra enlaza con su propio historial", async () => {
    simularServidor({ obra: () => json(obraSinDeclaracion) });

    montarApp("/catalogo/obra-3");
    await screen.findByText("Sin declaración");

    // Esta obra no tiene ninguna version, asi que no hay historial que ofrecer:
    // la ficha ya dice que no hay declaracion -que es exactamente lo que diria
    // la lista vacia- y el enlace solo repetiria el hecho en otra pantalla.
    expect(
      screen.queryByRole("link", { name: "Ver el historial completo" }),
    ).toBeNull();
    expect(screen.queryByText(/La Casa de las Dos Palmas/)).toBeNull();
  });

  it("desde el historial se vuelve al detalle, y la busqueda del catalogo sobrevive el viaje", async () => {
    simularServidor();

    // El administrador viene de una busqueda del catalogo; el detalle le pasa
    // esa busqueda al historial y el historial se la devuelve al detalle.
    montarApp({
      pathname: "/catalogo/obra-1",
      state: { [CLAVE_DE_VUELTA_AL_CATALOGO]: "titulo=Casa" },
    });
    await screen.findByRole("table", {
      name: "Partes de la declaración vigente",
    });

    fireEvent.click(
      screen.getByRole("link", { name: "Ver el historial completo" }),
    );
    await screen.findByRole("heading", { name: "Historial de la declaración" });

    // La vuelta es a la OBRA, que es de donde se llega a esta pantalla, y con el
    // id de la obra que se esta viendo.
    const volver = screen.getByRole("link", { name: /Volver a la obra/ });
    expect(volver.getAttribute("href")).toBe("/catalogo/obra-1");

    fireEvent.click(volver);

    // Y el detalle vuelve a ofrecer el catalogo CON la busqueda: la direccion la
    // entrego el catalogo, la conservo el detalle, la llevo el historial y la
    // devolvio. Si el historial no la hubiera reenviado, aqui pondria "/catalogo"
    // a secas y la busqueda del administrador se habria perdido por el camino.
    await screen.findByRole("table", {
      name: "Partes de la declaración vigente",
    });
    expect(
      screen
        .getByRole("link", { name: /Volver al catálogo/ })
        .getAttribute("href"),
    ).toBe("/catalogo?titulo=Casa");
  });

  it("abierto por su direccion, la vuelta a la obra no pierde nada", async () => {
    simularServidor();

    // Sin venir de ningun sitio no hay busqueda del catalogo que devolver, y el
    // enlace a la obra sigue llevando a una direccion que existe.
    montarApp("/catalogo/obra-1/historial");

    await screen.findByRole("heading", { name: "Historial de la declaración" });
    expect(
      screen
        .getByRole("link", { name: /Volver a la obra/ })
        .getAttribute("href"),
    ).toBe("/catalogo/obra-1");
  });
});
