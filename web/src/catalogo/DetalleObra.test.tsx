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

// El caso que el listado ya distingue y que esta pantalla tiene que distinguir
// igual: `incompleta` con una version abierta es "declarada y no suma 100"; el
// estado `incompleta` con `version_vigente` en `null` es "nadie la declaro".
const obraIncompleta = {
  ...obraCompleta,
  id: "obra-2",
  titulo: "Noche de Bodas",
  estado_declaracion: "incompleta",
  suma_porcentajes: 74.5,
  version_vigente: 2,
} satisfies Obra;

const obraSinDeclaracion = {
  ...obraCompleta,
  id: "obra-3",
  titulo: "Sin Declarar Todavia",
  ida: undefined,
  estado_declaracion: "incompleta",
  suma_porcentajes: 0,
  version_vigente: null,
} satisfies Obra;

const versionCerrada: VersionDeclaracion = {
  version: 2,
  vigente_desde: "2026-01-15T10:00:00Z",
  vigente_hasta: "2026-03-01T09:00:00Z",
  estado: "completa",
  partes: [
    { titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: 60 },
    { titular_id: "tit-2", ipi: "IPI-00000002", porcentaje: 40 },
  ],
};

const versionVigente: VersionDeclaracion = {
  version: 3,
  vigente_desde: "2026-03-01T09:00:00Z",
  vigente_hasta: null,
  estado: "completa",
  partes: [
    { titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: 75 },
    { titular_id: "tit-2", ipi: "IPI-00000002", porcentaje: 25 },
  ],
};

const HISTORIAL: VersionDeclaracion[] = [versionCerrada, versionVigente];

/**
 * El padron por el que esta pantalla resuelve el nombre de las partes (item 9b):
 * dos entradas, las de los dos `titular_id` de la version vigente. Los nombres son
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

const versionDosIncompleta: VersionDeclaracion = {
  version: 2,
  vigente_desde: "2026-03-01T09:00:00Z",
  vigente_hasta: null,
  estado: "incompleta",
  partes: [{ titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: 74.5 }],
};

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
 * al padron de titulares, que es por donde esta pantalla resuelve el nombre de
 * las partes (item 9b). **Sin `padron()` el padron responde 404**, que es lo que
 * hace falta en las pruebas que no miran esa columna: la tabla se pinta igual, con
 * el guion.
 */
function simularServidor({
  rol = "administrador",
  obra = () => json(obraCompleta),
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

/** Los GET que la pantalla hizo, en orden. */
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
 * se puede afirmar en el mismo tick en que se pinto el nombre.
 */
const respirar = () => new Promise((listo) => setTimeout(listo, 60));

/**
 * Una entrada del historial del router en memoria: la direccion, y el estado que
 * traiga. El estado es lo que distingue haber llegado desde el catalogo -que
 * entrega ahi su busqueda- de haber abierto la ficha por su direccion.
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

function tablaDePartes(): HTMLElement {
  return screen.getByRole("table", {
    name: "Partes de la declaración vigente",
  });
}

/** La fila de la tabla de partes que contiene ese `titular_id`. */
function filaDePartes(titularId: string): HTMLElement {
  const fila = screen.getByText(titularId).closest("tr");
  if (!fila) throw new Error(`"${titularId}" no esta dentro de una fila`);
  return fila;
}

/**
 * Lo que la fila de ese titular dice en su columna de NOMBRE, que es la tercera.
 *
 * Se lee entera -y no con un `getByText`- porque los dos valores que puede tener
 * son un nombre y el guion de "no se conoce", y una busqueda por texto no
 * distingue "no pinto nada" de "pinto el guion".
 */
function nombreDeLaFila(titularId: string): string {
  const celda = within(filaDePartes(titularId)).getAllByRole("cell")[2];
  if (!celda) throw new Error("la fila no tiene columna de nombre");
  return celda.textContent ?? "";
}

/**
 * Lo que la ficha dice de un dato con rotulo: el `<div>` que envuelve el `dt` y
 * su `dd`. Sirve para leer la cifra que se pinta sin tener que buscarla suelta
 * en toda la pantalla.
 */
function dato(rotulo: string): HTMLElement {
  const contenedor = screen.getByText(rotulo).closest("div");
  if (!contenedor) throw new Error(`"${rotulo}" no esta dentro de un dato`);
  return contenedor;
}

describe("detalle de obra (integracion con App)", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    setToken("tok");
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("el administrador ve la obra y las partes de la declaracion vigente", async () => {
    simularServidor();

    montarApp("/catalogo/obra-1");

    // Se espera a la TABLA y no al titulo: el titulo llega con la obra, y las
    // partes llegan en la peticion siguiente, la del historial.
    await screen.findByRole("table", {
      name: "Partes de la declaración vigente",
    });

    // Las partes salen del historial, que es la unica lectura donde estan: la
    // obra trae las cifras de cabecera, no el reparto.
    expect(consultas()).toEqual([
      "/api/obras/obra-1",
      // Basta la version abierta: el servidor pagina el historial desde la mas
      // reciente, y `limite=1` es esa.
      "/api/obras/obra-1/declaracion/historial?limite=1",
    ]);

    // Los metadatos, cada uno con su rotulo.
    expect(within(dato("Género")).getByText("Drama")).toBeTruthy();
    expect(within(dato("Año")).getByText("1991")).toBeTruthy();
    expect(within(dato("Tipo")).getByText("serie")).toBeTruthy();
    expect(within(dato("IDA")).getByText("IDA-1")).toBeTruthy();
    expect(
      within(dato("Identificador de la obra")).getByText("obra-1"),
    ).toBeTruthy();

    // Y las dos cifras que calcula el servidor, tal cual llegan.
    expect(
      within(dato("Suma de los porcentajes declarados")).getByText("100.0000%"),
    ).toBeTruthy();
    expect(within(dato("Versión vigente")).getByText("3")).toBeTruthy();
    expect(within(dato("Estado")).getByText("Completa")).toBeTruthy();

    const encabezados = within(tablaDePartes())
      .getAllByRole("columnheader")
      .map((th) => th.textContent);
    expect(encabezados).toEqual([
      "Titular",
      "IPI",
      ROTULO_NOMBRE_EN_PADRON_ACTUAL,
      "Porcentaje",
    ]);

    // Las partes de la version ABIERTA (75 / 25), no las de la cerrada (60 / 40):
    // el historial trae las dos y la vigente es la que no tiene `vigente_hasta`.
    expect(
      within(filaDePartes("tit-1"))
        .getAllByRole("cell")
        .map((td) => td.textContent),
    ).toEqual(["tit-1", "IPI-00000001", "—", "75.0000%"]);
    expect(
      within(filaDePartes("tit-2"))
        .getAllByRole("cell")
        .map((td) => td.textContent),
    ).toEqual(["tit-2", "IPI-00000002", "—", "25.0000%"]);
    expect(screen.queryByText("60.0000%")).toBeNull();

    // La vuelta al catalogo: sin haber pasado por el catalogo no hay busqueda
    // que devolver, asi que lleva al catalogo entero. Lo que se prueba con el
    // clic esta mas abajo.
    expect(
      screen
        .getByRole("link", { name: /Volver al catálogo/ })
        .getAttribute("href"),
    ).toBe("/catalogo");
  });

  // El detalle tambien se abre SIN pasar por el catalogo: la direccion
  // tecleada, un enlace guardado, una pestaña nueva, un enlace de otra pantalla.
  // En esos casos la entrada no trae ninguna busqueda del catalogo, y la vuelta
  // no puede quedarse sin destino ni inventarse uno a partir de un estado que no
  // es suyo: lleva al catalogo, que existe siempre.
  const ENTRADAS_SIN_CATALOGO: [string, Entrada][] = [
    ["una direccion abierta sin estado", "/catalogo/obra-1"],
    [
      "el estado que puso otra pantalla",
      { pathname: "/catalogo/obra-1", state: { otraCosa: "x" } },
    ],
    [
      "la clave del catalogo con algo que no es una busqueda",
      {
        pathname: "/catalogo/obra-1",
        state: { [CLAVE_DE_VUELTA_AL_CATALOGO]: { titulo: "Casa" } },
      },
    ],
  ];

  it.each(ENTRADAS_SIN_CATALOGO)(
    "con %s la vuelta va al catalogo, no a una direccion inventada",
    async (_caso, entrada) => {
      simularServidor({ catalogo: () => json([obraCompleta]) });

      montarApp(entrada);

      await screen.findByRole("heading", {
        name: obraCompleta.titulo,
        level: 1,
      });
      expect(
        screen
          .getByRole("link", { name: /Volver al catálogo/ })
          .getAttribute("href"),
      ).toBe("/catalogo");
    },
  );

  it("sin venir del catalogo, la vuelta no deja al administrador sin salida", async () => {
    simularServidor({ catalogo: () => json([obraCompleta]) });

    montarApp("/catalogo/obra-1");
    await screen.findByRole("heading", {
      name: obraCompleta.titulo,
      level: 1,
    });

    fireEvent.click(screen.getByRole("link", { name: /Volver al catálogo/ }));

    // Llega al catalogo de verdad -su pantalla y su consulta-, y sin ningun
    // filtro que nadie haya puesto: es el mismo destino que tenia antes de este
    // arreglo, asi que quien no viene del catalogo no pierde nada por el.
    await screen.findByRole("heading", { name: "Catálogo de obras" });
    expect(
      await screen.findByRole("table", { name: "Catálogo de obras" }),
    ).toBeTruthy();
    expect(consultas()).toContain("/api/obras?limite=20");
    expect(consultas().some((url) => url.includes("titulo="))).toBe(false);
    expect(screen.getByLabelText("Título")).toHaveProperty("value", "");
  });

  it("una obra sin declaracion NO se pinta como una declarada incompleta", async () => {
    simularServidor({
      obra: () => json(obraSinDeclaracion),
      historial: () => json([]),
    });

    montarApp("/catalogo/obra-3");

    await screen.findByText("Sin declaración");

    // Las dos obras llegan del backend con el MISMO estado: la unica diferencia
    // es que una tiene una version vigente y la otra no.
    expect(obraSinDeclaracion.estado_declaracion).toBe("incompleta");
    expect(obraSinDeclaracion.version_vigente).toBeNull();

    expect(screen.queryByText("Incompleta")).toBeNull();
    expect(screen.queryByText("Completa")).toBeNull();
    expect(within(dato("Versión vigente")).getByText("—")).toBeTruthy();

    // La suma va igual, porque es lo que el backend manda para ese caso.
    expect(
      within(dato("Suma de los porcentajes declarados")).getByText("0.0000%"),
    ).toBeTruthy();

    // Sin declaracion no hay partes ni tabla de partes.
    expect(screen.queryByRole("table")).toBeNull();

    // Y el historial no se pide: `version_vigente` en `null` ya dijo que no hay
    // ninguna declaracion -el mismo hecho que diria una lista vacia-, asi que
    // una segunda peticion solo podria repetirlo. Tampoco se menciona un
    // historial vacio en pantalla: el hecho se dice UNA vez.
    expect(consultas()).toEqual(["/api/obras/obra-3"]);
    expect(document.body.textContent).not.toMatch(/historial/i);
    expect(document.body.textContent).toMatch(/no tiene ninguna declaración/i);
  });

  it("una declaracion incompleta se explica como el estado valido que es, en ambar", async () => {
    simularServidor({
      obra: () => json(obraIncompleta),
      historial: () => json([versionDosIncompleta]),
    });

    montarApp("/catalogo/obra-2");

    // La tabla primero: las partes van en la peticion del historial, que sale
    // despues de la de la obra.
    await screen.findByRole("table", {
      name: "Partes de la declaración vigente",
    });

    const etiqueta = within(dato("Estado")).getByText("Incompleta");
    expect(etiqueta.className).toContain("badge-estado-incompleta");
    expect(etiqueta.className).not.toMatch(/error/);

    // La consecuencia se explica -R-04, la reserva, que no se prorratea- y el
    // texto dice que no es un error: un total por debajo de 100 no lo es.
    expect(document.body.textContent).toMatch(/R-04/);
    expect(document.body.textContent).toMatch(/en reserva/);
    expect(document.body.textContent).toMatch(/nunca se prorratea/);
    expect(document.body.textContent).toMatch(/no es un error/i);
    // Y no afirma nada sobre la suma que el estado no pruebe: `incompleta` es
    // tambien una parte sin IPI con la suma en 100, y el contrato manda los dos
    // campos justamente porque ninguno se deduce del otro.
    expect(document.body.textContent).not.toMatch(/no suma 100/i);

    // Ni un estado `invalida` -que el backend no puede persistir- ni el rojo
    // reservado a lo que de verdad falla.
    expect(document.body.textContent).not.toMatch(/inv[aá]lida/i);
    expect(screen.queryByRole("alert")).toBeNull();

    expect(
      within(dato("Suma de los porcentajes declarados")).getByText("74.5000%"),
    ).toBeTruthy();
    expect(within(filaDePartes("tit-1")).getByText("74.5000%")).toBeTruthy();
  });

  it("pinta el estado y la suma del backend aunque no cuadren con las partes", async () => {
    // El contrato manda las tres cosas y ninguna se deduce de otra: la suma y el
    // estado los calcula el servidor sobre la version vigente, y las partes son
    // el reparto declarado. Si el cliente sumara las partes para pintar el
    // total, corregiria al backend y la pantalla dejaria de decir lo que el
    // sistema sabe.
    const discordante = {
      ...obraCompleta,
      estado_declaracion: "incompleta",
      suma_porcentajes: 12.5,
    } satisfies Obra;
    const unaParteAlCien: VersionDeclaracion = {
      ...versionVigente,
      partes: [{ titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: 100 }],
    };
    simularServidor({
      obra: () => json(discordante),
      historial: () => json([unaParteAlCien]),
    });

    montarApp("/catalogo/obra-1");

    await screen.findByRole("table", {
      name: "Partes de la declaración vigente",
    });

    expect(within(dato("Estado")).getByText("Incompleta")).toBeTruthy();
    expect(
      within(dato("Suma de los porcentajes declarados")).getByText("12.5000%"),
    ).toBeTruthy();
    // Y la parte sigue diciendo lo suyo: 100.0000% en su fila, que es lo
    // declarado por ese titular.
    expect(within(filaDePartes("tit-1")).getByText("100.0000%")).toBeTruthy();
  });

  it("el titular_id va visible y el nombre sale del padron, en UNA peticion por los ids que la tabla muestra", async () => {
    simularServidor({ padron: () => json(PADRON) });

    montarApp("/catalogo/obra-1");
    await screen.findByRole("table", {
      name: "Partes de la declaración vigente",
    });

    // El rotulo dice de donde sale el nombre -del padron de HOY, no de la fecha
    // de la version (D-006)-, y el `titular_id` esta al lado para poder
    // conciliar la pantalla con la API.
    expect(
      within(tablaDePartes()).getByRole("columnheader", {
        name: ROTULO_NOMBRE_EN_PADRON_ACTUAL,
      }),
    ).toBeTruthy();
    expect(within(filaDePartes("tit-1")).getByText("tit-1")).toBeTruthy();
    expect(
      within(filaDePartes("tit-1")).getByText("IPI-00000001"),
    ).toBeTruthy();

    // Y la columna dice el NOMBRE que el padron devolvio para ese identificador
    // (item 9b), en las dos filas.
    await within(tablaDePartes()).findByText("Ana Escritora");
    expect(nombreDeLaFila("tit-1")).toBe("Ana Escritora");
    expect(nombreDeLaFila("tit-2")).toBe("Luis Guionista");

    // UNA peticion, y con los identificadores EXACTOS de las filas que la tabla
    // muestra: ni una por fila, ni el padron entero paginado y cruzado aqui.
    expect(consultasAlPadron()).toEqual(["/api/titulares?ids=tit-1&ids=tit-2"]);

    // Y sigue siendo una despues de esperar: un `path` que cambiara entre
    // renders volveria a pedir el padron en bucle.
    await respirar();
    expect(consultasAlPadron()).toHaveLength(1);

    // Lo que la columna NO puede decir es que el titular falte: el guion es "no
    // se conoce" y nunca "no esta".
    expect(document.body.textContent).not.toMatch(/no existe/i);
    expect(document.body.textContent).not.toMatch(/no est[aá] en el padron/i);
  });

  it("una fila que el padron no reconoce se queda con el guion, sin decir que el titular falte", async () => {
    // El padron contesta, pero solo por uno de los dos identificadores: `ids` es
    // un filtro, no una promesa de existencia, asi que la otra fila no puede
    // afirmar ni el nombre ni que le falte.
    simularServidor({
      padron: () => json(PADRON.filter((titular) => titular.id === "tit-1")),
    });

    montarApp("/catalogo/obra-1");
    const laTabla = await screen.findByRole("table", {
      name: "Partes de la declaración vigente",
    });
    await within(laTabla).findByText("Ana Escritora");

    expect(nombreDeLaFila("tit-1")).toBe("Ana Escritora");
    expect(nombreDeLaFila("tit-2")).toBe("—");
    // Y se le pregunto por ella: lo que falta es la fila del padron, no la
    // pregunta, que es justo lo que distingue el guion de un dato sin pedir.
    expect(consultasAlPadron()).toEqual(["/api/titulares?ids=tit-1&ids=tit-2"]);
    expect(document.body.textContent).not.toMatch(/no existe/i);
  });

  it("si el padron no se puede leer, la tabla sigue y la columna se queda en el guion", async () => {
    // El nombre es un dato de HOY que acompaña al reparto, y quien viene a esta
    // pantalla viene a leer el reparto: un aviso de error por una columna
    // accesoria taparia las cifras que si llegaron.
    simularServidor({ padron: () => json({ error: "caido" }, 500) });

    montarApp("/catalogo/obra-1");
    await screen.findByRole("table", {
      name: "Partes de la declaración vigente",
    });
    await respirar();

    expect(consultasAlPadron()).toHaveLength(1);
    expect(nombreDeLaFila("tit-1")).toBe("—");
    expect(within(filaDePartes("tit-1")).getByText("75.0000%")).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("un 404 de la obra se dice como lo que es, no como un fallo del sistema", async () => {
    // El caso propio del issue #30: una obra que se listo hace un momento y ya
    // no esta. El 404 no es un error de la pantalla ni del servidor.
    simularServidor({
      obra: () => json({ error: "esa obra no esta en el catalogo" }, 404),
    });

    montarApp("/catalogo/obra-1");

    await screen.findByRole("heading", {
      name: "Esa obra no está en el catálogo",
    });
    expect(screen.getByText("obra-1")).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.queryByText(/No se pudo consultar la obra/)).toBeNull();
    expect(screen.queryByRole("table")).toBeNull();
    // Y no se pide el historial de una obra que no esta.
    expect(consultas()).toEqual(["/api/obras/obra-1"]);
    // El aviso tambien ofrece la vuelta, y a la misma direccion que la ficha:
    // las dos salidas de la pantalla resuelven el destino una sola vez.
    expect(
      screen
        .getByRole("link", { name: /Volver al catálogo/ })
        .getAttribute("href"),
    ).toBe("/catalogo");
  });

  it("un fallo al leer la obra se muestra con el mensaje del backend", async () => {
    const mensaje = "no se pudo consultar la obra";
    simularServidor({ obra: () => json({ error: mensaje }, 500) });

    montarApp("/catalogo/obra-1");

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain("No se pudo consultar la obra:");
    expect(alerta.textContent).toContain(mensaje);
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("si el historial no se puede leer, la ficha y las cifras del backend siguen", async () => {
    simularServidor({
      historial: () =>
        json({ error: "no se pudo consultar el historial" }, 500),
    });

    montarApp("/catalogo/obra-1");

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain(
      "No se pudo consultar el historial de la declaración:",
    );
    expect(alerta.textContent).toContain("no se pudo consultar el historial");

    // Lo que si llego se sigue viendo: la obra y sus dos cifras vienen de otra
    // lectura, y esconderlas por un fallo ajeno seria negar lo que el sistema ya
    // dijo. Lo que no se pinta son las partes.
    expect(
      screen.getByRole("heading", { name: "La Casa de las Dos Palmas" }),
    ).toBeTruthy();
    expect(within(dato("Estado")).getByText("Completa")).toBeTruthy();
    expect(
      within(dato("Suma de los porcentajes declarados")).getByText("100.0000%"),
    ).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
  });

  // El historial y la obra son dos lecturas del mismo hecho: cual version rige.
  // Si no coinciden -una declaracion guardada entre las dos peticiones, un
  // historial que no trae ninguna abierta- la pantalla no elige: dice que no
  // coinciden y no pinta partes, en vez de afirmar un reparto que la obra no
  // esta sosteniendo.
  const HISTORIALES_SIN_LA_VIGENTE: [string, VersionDeclaracion[]][] = [
    ["una lista vacia", []],
    ["ninguna version abierta", [versionCerrada]],
    ["otra version abierta", [{ ...versionVigente, version: 2 }]],
    [
      "dos versiones abiertas",
      [versionVigente, { ...versionVigente, version: 4 }],
    ],
  ];

  it.each(HISTORIALES_SIN_LA_VIGENTE)(
    "un historial con %s no se pinta como la declaracion vigente",
    async (_caso, historial) => {
      simularServidor({ historial: () => json(historial) });

      montarApp("/catalogo/obra-1");

      const alerta = await screen.findByRole("alert");
      expect(alerta.textContent).toContain(
        "El servidor declara vigente la versión 3",
      );
      expect(alerta.textContent).toContain("No se muestran partes");
      expect(screen.queryByRole("table")).toBeNull();

      // Las cifras de la obra siguen siendo las del backend.
      expect(within(dato("Estado")).getByText("Completa")).toBeTruthy();
      expect(
        within(dato("Suma de los porcentajes declarados")).getByText(
          "100.0000%",
        ),
      ).toBeTruthy();
    },
  );

  it("una version vigente sin partes lo dice en vez de dejar la tabla vacia", async () => {
    simularServidor({
      historial: () => json([{ ...versionVigente, partes: [] }]),
    });

    montarApp("/catalogo/obra-1");

    await screen.findByText(
      "La versión vigente no trae ninguna parte declarada.",
    );
    expect(screen.queryByRole("table")).toBeNull();
  });

  // Un 2xx que no trae lo prometido. `useApi<Obra>` y `useApi<VersionDeclaracion[]>`
  // no comprueban la forma -`T` es una promesa, no una verificacion-, y sin
  // ErrorBoundary en `web/src` el primer campo mal formado deja la pantalla en
  // blanco. Cada payload pasa por `esObra` / `esVersionDeclaracion`.
  const OBRAS_ILEGIBLES: [string, unknown][] = [
    ["un objeto que no es una obra", {}],
    [
      "una obra sin `version_vigente` (el backend no lo poblo)",
      { ...obraCompleta, version_vigente: undefined },
    ],
    [
      "una obra con un estado que el sistema no puede producir",
      { ...obraCompleta, estado_declaracion: "invalida" },
    ],
    [
      "una obra con la suma como texto",
      { ...obraCompleta, suma_porcentajes: "100" },
    ],
    [
      "una obra con el IDA como objeto",
      { ...obraCompleta, ida: { ida: "IDA-1" } },
    ],
  ];

  it.each(OBRAS_ILEGIBLES)(
    "un 2xx con %s deja la pantalla en error, sin pintarlo ni tumbarla",
    async (_caso, cuerpo) => {
      simularServidor({ obra: () => json(cuerpo) });

      montarApp("/catalogo/obra-1");

      const alerta = await screen.findByRole("alert");
      expect(alerta.textContent).toContain(
        "La obra no llegó con los datos que esta pantalla lee.",
      );
      expect(screen.queryByRole("table")).toBeNull();

      // Y sobre todo: un payload sin `version_vigente` NO se lee como "esta obra
      // no tiene declaracion". La pantalla entera queda en error en vez de
      // afirmar sobre una obra algo que el sistema no dijo.
      expect(screen.queryByText("Sin declaración")).toBeNull();
      // Ni se pide el historial de una obra que no se pudo leer.
      expect(consultas()).toEqual(["/api/obras/obra-1"]);
    },
  );

  const HISTORIALES_ILEGIBLES: [string, unknown][] = [
    ["un objeto en vez de una lista", { error: "algo salio mal" }],
    ["una lista con un elemento vacio", [{}]],
    [
      "una version sin `vigente_hasta` (el backend no lo poblo)",
      [{ ...versionVigente, vigente_hasta: undefined }],
    ],
    [
      "una version con `version` como texto",
      [{ ...versionVigente, version: "3" }],
    ],
    ["una version sin `partes`", [{ ...versionVigente, partes: undefined }]],
    [
      "una parte con el porcentaje como texto",
      [
        {
          ...versionVigente,
          partes: [
            { titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: "75" },
          ],
        },
      ],
    ],
    [
      "una parte con el `titular_id` como objeto",
      [
        {
          ...versionVigente,
          partes: [
            {
              titular_id: { id: "tit-1" },
              ipi: "IPI-00000001",
              porcentaje: 75,
            },
          ],
        },
      ],
    ],
  ];

  it.each(HISTORIALES_ILEGIBLES)(
    "un 2xx con %s deja las partes sin pintar, con un error explicito",
    async (_caso, historial) => {
      simularServidor({ historial: () => json(historial) });

      montarApp("/catalogo/obra-1");

      const alerta = await screen.findByRole("alert");
      expect(alerta.textContent).toContain(
        "El historial no llegó como una lista de versiones legibles.",
      );
      expect(screen.queryByRole("table")).toBeNull();

      // En particular, una version SIN `vigente_hasta` no se lee como abierta:
      // seria pintar como reparto de hoy el de una version ya cerrada. Y una
      // parte con el porcentaje como texto no se pinta como cifra.
      expect(screen.queryByText("75.0000%")).toBeNull();
    },
  );

  it("mientras llega la respuesta dice que esta cargando, sin ficha", async () => {
    vi.mocked(fetch).mockImplementation((entrada) => {
      if (String(entrada) === "/api/auth/session") {
        return Promise.resolve(respuestaDeSesion("administrador"));
      }
      return new Promise<Response>(() => {});
    });

    montarApp("/catalogo/obra-1");

    // Primero el shell -la sesion ya resolvio-, y entonces el estado de carga
    // es el de la obra: RutaProtegida monta el suyo mientras no hay usuario.
    await screen.findByRole("link", { name: "Catálogo" });
    expect(screen.getByRole("status")).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("un auditor no ve el detalle: el guard de rol se hereda por el prefijo de la ruta", async () => {
    simularServidor({ rol: "auditor" });

    montarApp("/catalogo/obra-1");

    await screen.findByRole("heading", { name: "No autorizado" });
    expect(
      screen.queryByRole("heading", { name: "La Casa de las Dos Palmas" }),
    ).toBeNull();
    expect(consultas()).toEqual([]);
  });

  // S4 del issue #30: "saving creates a new version; the previous version is
  // still visible in history". Para que la version anterior se vea hay que poder
  // LLEGAR al historial, y el unico enlace que lleva es este. Va por obra -el
  // `id` del destino es el de la obra que se esta viendo- porque un enlace que
  // llevara al historial de otra obra pasaria un test que solo mirara el texto.
  //
  // El otro enlace de la ficha, el del editor (paso 8), se prueba en
  // `EditorReparto.test.tsx`, que es donde vive esa pantalla: alli se comprueba
  // el `id` del destino, que el texto cambia segun la obra tenga declaracion o
  // no, y que el clic abre el editor de verdad. Aqui estaba el test que exigia
  // <EnConstruccion> en `/catalogo/:id/declaracion`, y dejo de tener sentido el
  // dia que la ruta paso a tener pantalla: un test de un placeholder caduca
  // cuando el placeholder desaparece, no cuando alguien lo mira.
  const OBRAS_CON_HISTORIAL: [string, Obra, VersionDeclaracion[], string][] = [
    [
      "una obra declarada completa",
      obraCompleta,
      HISTORIAL,
      "/catalogo/obra-1/historial",
    ],
    [
      "una obra declarada incompleta",
      obraIncompleta,
      [versionDosIncompleta],
      "/catalogo/obra-2/historial",
    ],
  ];

  it.each(OBRAS_CON_HISTORIAL)(
    "el detalle de %s enlaza con el historial de ESA obra",
    async (_caso, obra, historial, destino) => {
      simularServidor({
        obra: () => json(obra),
        historial: () => json(historial),
      });

      montarApp(`/catalogo/${obra.id}`);
      await screen.findByRole("table", {
        name: "Partes de la declaración vigente",
      });

      const enlace = screen.getByRole("link", {
        name: "Ver el historial completo",
      });
      expect(enlace.getAttribute("href")).toBe(destino);
      expect(destino).toContain(obra.id);
    },
  );

  it("la obra sin ninguna declaracion no ofrece un historial que ya sabe vacio", async () => {
    // Esta pantalla no pide el historial cuando `version_vigente` es `null` -el
    // hecho ya esta dicho-, y el enlace sigue la misma regla: llevaria a una
    // pantalla que repite "no hay ninguna version".
    simularServidor({
      obra: () => json(obraSinDeclaracion),
      historial: () => json([]),
    });

    montarApp("/catalogo/obra-3");
    await screen.findByText("Sin declaración");

    expect(
      screen.queryByRole("link", { name: "Ver el historial completo" }),
    ).toBeNull();
    expect(consultas()).toEqual(["/api/obras/obra-3"]);
  });
});
