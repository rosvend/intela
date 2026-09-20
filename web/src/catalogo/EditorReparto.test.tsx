import {
  act,
  cleanup,
  createEvent,
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
import { DEBOUNCE_TECLEO_MS } from "../useValorDiferido";
import type { Obra, Titular, VersionDeclaracion } from "./tipos";

function json(cuerpo: unknown, status = 200): Response {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { "content-type": "application/json" },
  });
}

/**
 * Una respuesta que LLEGO -con su status- y cuyo cuerpo se corta a mitad: el
 * stream falla, asi que leerlo rechaza. Es la forma sintetica de un cuerpo
 * truncado, y la unica que prueba el caso sin depender de que haya un proxy de
 * por medio.
 */
function respuestaConCuerpoCortado(status: number): Response {
  const cuerpo = new ReadableStream({
    start(controlador) {
      controlador.error(new Error("la conexion se corto a mitad del cuerpo"));
    },
  });
  return new Response(cuerpo, {
    status,
    headers: { "content-type": "application/json" },
  });
}

// Fixtures tipados con el contrato generado: si `Obra`, `VersionDeclaracion` o
// `Titular` cambian en api/openapi.yaml, `tsc` rompe aqui antes que en pantalla.
// Nombres y titulos sinteticos, no de ningun canal real.
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
  estado_declaracion: "completa",
  suma_porcentajes: 100,
  version_vigente: 3,
} satisfies Obra;

const obraSinDeclarar = {
  ...obraDeclarada,
  id: "obra-3",
  titulo: "Sin Declarar Todavia",
  ida: undefined,
  estado_declaracion: "incompleta",
  suma_porcentajes: 0,
  version_vigente: null,
} satisfies Obra;

// Una obra que va por la version 7 y declara a una productora: el reparto que
// quedo escrito antes de que `R-01` se comprobara al guardar (paso 3).
const obraAvanzada = {
  ...obraDeclarada,
  id: "obra-7",
  titulo: "Obra por la Septima",
  version_vigente: 7,
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

const versionSeisCerrada: VersionDeclaracion = {
  ...versionCerrada,
  version: 6,
  vigente_desde: "2026-02-01T09:00:00Z",
};

const versionSieteAbierta: VersionDeclaracion = {
  ...versionVigente,
  version: 7,
  vigente_desde: "2026-04-01T09:00:00Z",
};

// El reparto de partida de la prueba del borde de la tolerancia: tres
// porcentajes de cuatro decimales que en aritmetica decimal suman 100 clavado y
// en coma flotante dan 99.99999999999999.
const versionConRuido: VersionDeclaracion = {
  version: 3,
  vigente_desde: "2026-03-01T09:00:00Z",
  vigente_hasta: null,
  estado: "completa",
  partes: [
    { titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: 44.289 },
    { titular_id: "tit-2", ipi: "IPI-00000002", porcentaje: 53.6457 },
    { titular_id: "tit-3", ipi: "IPI-00000003", porcentaje: 2.0653 },
  ],
};

// El reparto de una version que declaro a una productora. Existe para probar
// que el editor NO bloquea en el cliente lo que bloquea el backend: la fila se
// carga tal cual esta escrita, se puede guardar, y el 400 de `R-01` es lo que se
// enseña.
const versionConProductora: VersionDeclaracion = {
  version: 7,
  vigente_desde: "2026-04-01T09:00:00Z",
  vigente_hasta: null,
  estado: "completa",
  partes: [
    { titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: 50 },
    {
      titular_id: "tit-productor-a",
      ipi: "IPI-00000077",
      porcentaje: 50,
    },
  ],
};

// Los cuatro titulares que hacen falta para distinguir a quien se ofrece: tres
// personas naturales de clases distintas -que cobran igual- y una sociedad. El
// tercero no esta en el reparto vigente de la obra, y por eso es el que se puede
// añadir.
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
    nombre: "Beto Libretista",
    ipi: "IPI-00000002",
    persona_natural: true,
    clase: "administrado",
  },
  {
    id: "tit-3",
    nombre: "Caro Guionista",
    ipi: "IPI-00000003",
    persona_natural: true,
    clase: "socio",
  },
  {
    id: "tit-productor-a",
    nombre: "Productora del Caribe S.A.S.",
    ipi: "IPI-00000077",
    persona_natural: false,
    clase: "administrado",
  },
];

const versionCuatro: VersionDeclaracion = {
  version: 4,
  vigente_desde: "2026-06-01T09:00:00Z",
  vigente_hasta: null,
  estado: "completa",
  partes: [
    { titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: 60 },
    { titular_id: "tit-2", ipi: "IPI-00000002", porcentaje: 40 },
  ],
};

// La version que abre el PRIMER guardado de una obra: la 1. Existe porque el
// panel del exito prometia, sin condicion, conservar una version anterior que
// en este caso no hay (WARNING de la revision adversarial de la iteracion 1).
const primeraVersion: VersionDeclaracion = {
  version: 1,
  vigente_desde: "2026-09-01T09:00:00Z",
  vigente_hasta: null,
  estado: "completa",
  partes: [{ titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: 100 }],
};

// Los mensajes REALES con que contesta el handler
// (internal/infraestructura/httpapi/declaraciones.go). Se copian literales para
// que la pantalla se pruebe contra la prosa que de verdad va a recibir; si el
// backend los reformula, esto se queda viejo y hay que volver a mirarlo.
const MENSAJE_SUMA =
  "declaracion invalida: la suma de porcentajes es 120.0000%, no puede superar 100";
const MENSAJE_R01 =
  "uno de los titulares indicados no es persona natural, y solo un escritor persona natural puede recibir reparto (R-01, RD 4.5)";
const MENSAJE_DECIMALES =
  'declaracion invalida: el porcentaje del titular "tit-1" admite hasta 4 decimales';
const MENSAJE_500 = "no se pudo guardar la declaracion";
const MENSAJE_HISTORIAL = "no se pudo consultar el historial";

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
const esLaDeclaracion = (url: string) =>
  /^\/api\/obras\/[^/]+\/declaracion$/.test(url);

/**
 * Un backend falso que responde por URL y metodo. La sesion, la obra, su
 * historial, el padron y el PUT de la declaracion se responden con lo que pase
 * cada test; todo lo demas, 404.
 */
function simularServidor({
  rol = "administrador",
  obra = () => json(obraDeclarada),
  historial = () => json(HISTORIAL),
  titulares = () => json(PADRON),
  guardado = () => json(versionCuatro),
}: {
  rol?: Rol;
  obra?: () => Response;
  historial?: () => Response;
  titulares?: () => Response;
  guardado?: () => Response;
} = {}) {
  vi.mocked(fetch).mockImplementation((entrada, init) => {
    const url = String(entrada);
    const metodo = init?.method ?? "GET";
    if (url === "/api/auth/session") {
      return Promise.resolve(respuestaDeSesion(rol));
    }
    if (metodo === "PUT" && esLaDeclaracion(url)) {
      return Promise.resolve(guardado());
    }
    if (metodo === "GET" && esLaObra(url)) {
      return Promise.resolve(obra());
    }
    if (metodo === "GET" && esElHistorial(url)) {
      return Promise.resolve(historial());
    }
    if (metodo === "GET" && url.startsWith("/api/titulares")) {
      return Promise.resolve(titulares());
    }
    return Promise.resolve(json({ error: "ruta no encontrada" }, 404));
  });
}

type Peticion = {
  url: string;
  metodo: string;
  cuerpo: string;
};

/** Todas las peticiones que hizo la pantalla, en orden. */
function peticiones(): Peticion[] {
  return vi.mocked(fetch).mock.calls.map(([entrada, init]) => ({
    url: String(entrada),
    metodo: init?.method ?? "GET",
    cuerpo: init?.body === undefined ? "" : String(init.body),
  }));
}

/** Los GET de la obra y su declaracion, en orden. */
function consultas(): string[] {
  return peticiones()
    .filter(
      (p) =>
        p.metodo === "GET" &&
        p.url.startsWith("/api/obras") &&
        p.url !== "/api/auth/session",
    )
    .map((p) => p.url);
}

const guardados = () => peticiones().filter((p) => p.metodo === "PUT");

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

/** La ruta del editor de una obra, tal como la abre un enlace de la ficha. */
const LA_DECLARACION = "/catalogo/obra-1/declaracion";

/**
 * El editor montado: se espera a su titulo, que es lo que aparece cuando la obra
 * y su historial ya se resolvieron -y con ellos el borrador-. El padron sale en
 * la peticion siguiente, asi que los tests que lo miran lo esperan aparte.
 */
async function abrirElEditor(entrada: Entrada = LA_DECLARACION) {
  montarApp(entrada);
  await screen.findByRole("heading", { name: "Declaración de la obra" });
}

/** El editor con el padron ya cargado. */
async function abrirElEditorConPadron(entrada: Entrada = LA_DECLARACION) {
  await abrirElEditor(entrada);
  await screen.findByRole("table", { name: "Padrón de titulares" });
}

/** El texto visible de toda la pantalla, sin etiquetas. */
function texto(): string {
  return document.body.textContent ?? "";
}

/**
 * Lo que la ficha dice de un dato con rotulo: el `<div>` que envuelve el `dt` y
 * su `dd`. Sirve para leer una cifra sin buscarla suelta en toda la pantalla.
 */
function dato(rotulo: string): string {
  const contenedor = screen.getByText(rotulo).closest("div");
  if (!contenedor) throw new Error(`"${rotulo}" no esta dentro de un dato`);
  return contenedor.textContent ?? "";
}

/**
 * El nodo vivo -el que lleva `role="status"`- del dato con ese rotulo. Si el
 * rotulo no esta dentro de uno, revienta: es lo que el item 8a pide y lo que no
 * se puede comprobar con una busqueda por texto.
 */
function nodoVivo(rotulo: string): HTMLElement {
  const contenedor = screen.getByText(rotulo).closest("div");
  const vivo = contenedor?.querySelector<HTMLElement>('[role="status"]');
  if (!vivo) {
    throw new Error(
      `"${rotulo}" no esta dentro de un nodo vivo (role="status")`,
    );
  }
  return vivo;
}

/** El aviso de la version, que es un parrafo con su propia clase. */
function avisoDeVersion(): HTMLElement {
  const aviso = document.querySelector(".editor-aviso-version");
  if (!aviso) throw new Error("no hay ningun aviso de version en pantalla");
  return aviso as HTMLElement;
}

const botonGuardar = () =>
  screen.getByRole("button", { name: /^Guardar la declaración$/ });

/**
 * La fila del padron de ese titular. Se busca DENTRO de la tabla del padron: el
 * `titular_id` tambien esta en la tabla del borrador, y buscarlo en toda la
 * pantalla encontraria dos.
 */
function filaDelPadron(titularId: string): HTMLElement {
  const padron = screen.getByRole("table", { name: "Padrón de titulares" });
  const fila = within(padron).getByText(titularId).closest("tr");
  if (!fila) throw new Error(`"${titularId}" no esta dentro de una fila`);
  return fila;
}

/** El campo del porcentaje de esa fila del borrador. */
const campoPorcentaje = (titularId: string) =>
  screen.getByLabelText(`Porcentaje de ${titularId}`);

const campoIpi = (titularId: string) =>
  screen.getByLabelText(`IPI de ${titularId}`);

function escribirPorcentaje(titularId: string, valor: string) {
  fireEvent.change(campoPorcentaje(titularId), { target: { value: valor } });
}

/** Anade al reparto el titular de esa fila del padron. */
function agregarAlReparto(titularId: string) {
  fireEvent.click(
    within(filaDelPadron(titularId)).getByRole("button", {
      name: "Añadir al reparto",
    }),
  );
}

/** La rejilla del borrador: la tabla que se edita, distinta de la del padron. */
function rejilla(): HTMLElement {
  return screen.getByRole("table", { name: "Borrador del reparto" });
}

/**
 * La fila del borrador de ese titular. Se busca DENTRO de la rejilla y por el
 * campo del IPI -que lleva su `aria-label`-: el `titular_id` esta tambien en la
 * tabla del padron como celda suelta, asi que buscarlo por texto en toda la
 * pantalla encontraria dos.
 */
function filaDelBorrador(titularId: string): HTMLElement {
  const campo = within(rejilla()).getByLabelText(`IPI de ${titularId}`);
  const fila = campo.closest("tr");
  if (!fila) throw new Error(`"${titularId}" no esta dentro de una fila`);
  return fila;
}

/**
 * Un clic en el boton de quitar de la fila de ese titular.
 *
 * El nombre accesible del boton nombra la fila como la nombra la rejilla: con el
 * nombre del titular cuando la fila lo sabe -la que se elige del padron, que es
 * de donde sale el nombre- y con su `titular_id` cuando no -las filas sembradas
 * desde el historial, que no lo traen (D-006)-. Por eso la etiqueta se compone
 * aqui con ese criterio y no con una cadena fija: la cadena con el `titular_id` a
 * secas dejo de resolver en cuanto el boton empezo a nombrar por el nombre, y un
 * helper que solo supiera de identificadores ataria estas pruebas a la mitad de
 * los casos.
 *
 * `titularId` es lo que identifica la FILA -el campo del IPI se rotula con el,
 * siempre-; `nombreDelTitular` es lo que la fila dice de si misma.
 */
function quitarDelReparto(titularId: string, nombreDelTitular?: string) {
  fireEvent.click(
    within(filaDelBorrador(titularId)).getByRole("button", {
      name: `Quitar a ${nombreDelTitular ?? titularId} del reparto`,
    }),
  );
}

/** Un clic en Guardar, que dispara el submit del formulario. */
function guardar() {
  fireEvent.click(botonGuardar());
}
describe("editor de reparto (integracion con App)", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    setToken("tok");
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("el borrador arranca con el reparto vigente y con el aviso de que version se cierra", async () => {
    simularServidor();

    await abrirElEditorConPadron();

    // Las tres lecturas, y la obra antes que su historial: el reparto vigente
    // es lo que da forma al borrador.
    expect(consultas()).toEqual([
      "/api/obras/obra-1",
      "/api/obras/obra-1/declaracion/historial",
    ]);
    // El padron se pide SIN `persona_natural`: el contrato devuelve el padron
    // entero justamente para poder explicar a quien no se ofrece.
    expect(
      peticiones().filter((p) => p.url.startsWith("/api/titulares")),
    ).toEqual([{ url: "/api/titulares?limite=50", metodo: "GET", cuerpo: "" }]);

    expect(
      screen.getByRole("heading", { name: "Declaración de la obra" }),
    ).toBeTruthy();
    expect(screen.getByText(/La Casa de las Dos Palmas/)).toBeTruthy();

    // El aviso nombra las dos versiones, y las saca del historial: la que esta
    // abierta es la que se cierra.
    expect(avisoDeVersion().textContent).toMatch(
      /cerrará la versión 3 y abrirá la versión 4/,
    );

    // El borrador viene con el reparto de la version ABIERTA (75/25), no con el
    // de la cerrada (60/40): son las partes de la que rige hoy.
    expect(campoPorcentaje("tit-1")).toHaveProperty("value", "75");
    expect(campoPorcentaje("tit-2")).toHaveProperty("value", "25");
    expect(campoIpi("tit-1")).toHaveProperty("value", "IPI-00000001");
    expect(screen.queryByDisplayValue("60")).toBeNull();

    // El total va ROTULADO como lo que es -una cuenta de esta pantalla- y el
    // estado del borrador no es el de la version guardada.
    expect(dato("Total del borrador (calculado en esta pantalla)")).toContain(
      "100.0000%",
    );
    expect(dato("Estado del borrador")).toContain("Completa");
    expect(texto()).toMatch(
      /El estado de la declaración guardada lo calcula el servidor/,
    );
    expect(botonGuardar()).toHaveProperty("disabled", false);
    expect(guardados()).toHaveLength(0);
  });

  // S2: "blocks save when the total exceeds 100". Los dos estados que el
  // backend rechaza con 400 son los dos que la pantalla bloquea: sin partes y
  // con la suma por encima de 100.
  it("un borrador vacio no se puede guardar, y lo dice", async () => {
    simularServidor();
    await abrirElEditor();

    quitarDelReparto("tit-1");
    quitarDelReparto("tit-2");

    expect(dato("Estado del borrador")).toContain("Sin nada declarado");
    expect(dato("Total del borrador (calculado en esta pantalla)")).toContain(
      "0.0000%",
    );
    expect(botonGuardar()).toHaveProperty("disabled", true);
    expect(
      screen.getByText(/Añade al menos una parte desde el padrón/),
    ).toBeTruthy();
    expect(
      screen.getByText("El borrador no tiene ninguna parte."),
    ).toBeTruthy();

    guardar();
    expect(guardados()).toHaveLength(0);
  });

  it("un total por encima de 100 bloquea el guardado, sin mandar nada", async () => {
    simularServidor();
    await abrirElEditor();

    escribirPorcentaje("tit-1", "120");

    expect(dato("Estado del borrador")).toContain("Pasa de 100");
    expect(dato("Total del borrador (calculado en esta pantalla)")).toContain(
      "145.0000%",
    );
    // La razon que se da NO es que la pantalla lo bloquee: es que el servidor
    // rechaza esa suma con un 400.
    expect(texto()).toMatch(/el servidor rechaza con un 400/);
    expect(botonGuardar()).toHaveProperty("disabled", true);

    guardar();
    expect(guardados()).toHaveLength(0);
  });

  // S3: un total por debajo de 100 se marca como incompleta y NO es un error.
  it("un total por debajo de 100 se marca Incompleta y se puede guardar igual", async () => {
    simularServidor();
    await abrirElEditor();

    escribirPorcentaje("tit-1", "60");
    escribirPorcentaje("tit-2", "0");

    // 60 + 0: el total es 60 y el borrador es guardable. El 0 queda escrito como
    // lo que es -un cero que alguien tecleo-, no como una fila sin rellenar.
    expect(dato("Estado del borrador")).toContain("Incompleta");
    expect(dato("Total del borrador (calculado en esta pantalla)")).toContain(
      "60.0000%",
    );
    // La consecuencia de R-04, no el defecto: se retiene el importe entero.
    expect(texto()).toMatch(/no es un error/);
    expect(texto()).toMatch(/R-04 \(RD 13.1.3\)/);
    expect(texto()).toMatch(/en reserva/);
    expect(texto()).toMatch(/nunca se prorratea/);
    // Ni "inválida" -que el backend no puede persistir- ni un tono de fallo.
    expect(texto()).not.toMatch(/inv[aá]lida/i);
    expect(botonGuardar()).toHaveProperty("disabled", false);
  });

  it("el borde de la tolerancia: el ruido de la coma flotante cuenta como 100 y pasarse de verdad no", async () => {
    // 44.289 + 53.6457 + 2.0653 suman 100 en decimal y 99.99999999999999 en
    // coma flotante. Sin la tolerancia la pantalla diria "99.9999%" junto a
    // "Incompleta": se contradiria con sus propios datos.
    simularServidor({ historial: () => json([versionConRuido]) });
    await abrirElEditor();

    expect(dato("Total del borrador (calculado en esta pantalla)")).toContain(
      "100.0000%",
    );
    expect(dato("Estado del borrador")).toContain("Completa");
    expect(botonGuardar()).toHaveProperty("disabled", false);

    // Un solo decimal de mas en la ultima fila y el total pasa de 100 de
    // verdad: ahi si esta excedido y no se guarda.
    escribirPorcentaje("tit-3", "2.0654");
    expect(dato("Total del borrador (calculado en esta pantalla)")).toContain(
      "100.0001%",
    );
    expect(dato("Estado del borrador")).toContain("Pasa de 100");
    expect(botonGuardar()).toHaveProperty("disabled", true);
  });

  // S2, la segunda mitad: "and shows the backend error if it slips through". El
  // cliente bloquea su caso, pero la autoridad es el backend, y su mensaje es
  // lo que se enseña -entero y sin reescribir- cuando rechaza el.
  it("un 400 del servidor se muestra tal cual, con su mensaje entero", async () => {
    simularServidor({
      guardado: () => json({ error: MENSAJE_SUMA }, 400),
    });
    await abrirElEditor();

    escribirPorcentaje("tit-1", "60");
    escribirPorcentaje("tit-2", "40");
    guardar();

    // Primero: el cliente mando el cuerpo que tenia escrito, sin redondear ni
    // corregir nada por su cuenta.
    expect(guardados()).toHaveLength(1);
    expect(guardados()[0].url).toBe("/api/obras/obra-1/declaracion");
    expect(JSON.parse(guardados()[0].cuerpo)).toEqual([
      { titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: 60 },
      { titular_id: "tit-2", ipi: "IPI-00000002", porcentaje: 40 },
    ]);

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain("El servidor rechazó la declaración");
    expect(within(alerta).getByText(MENSAJE_SUMA).textContent).toBe(
      MENSAJE_SUMA,
    );
    // El 400 tambien dice que no se guardo nada, y aqui el mecanismo es
    // directo: `repertorio.NuevaDeclaracion` y `R-01` salen antes de
    // `Gestion.Guardar`, que es el unico camino que escribe.
    expect(alerta.textContent).toMatch(/No se guardó nada/);
    // Y el borrador sigue montado, con lo que se escribio: no se pierde nada.
    expect(campoPorcentaje("tit-1")).toHaveProperty("value", "60");
  });

  it("el 400 de R-01 se muestra con su mensaje, que nombra la regla", async () => {
    // Una obra cuya version vigente declara a una productora. El editor la
    // carga como lo que es -una fila del reparto escrito- y NO la bloquea: la
    // barrera de R-01 es el backend, y su mensaje es el que explica el rechazo.
    simularServidor({
      obra: () => json(obraAvanzada),
      historial: () => json([versionConProductora]),
      guardado: () => json({ error: MENSAJE_R01 }, 400),
    });
    await abrirElEditor("/catalogo/obra-7/declaracion");

    guardar();

    const alerta = await screen.findByRole("alert");
    expect(within(alerta).getByText(MENSAJE_R01).textContent).toBe(MENSAJE_R01);
    expect(alerta.textContent).toContain("R-01");
    expect(alerta.textContent).toContain("RD 4.5");
    // El identificador de la fila se mando tal cual: el servidor rechaza la
    // parte que nombra a una sociedad, y la pantalla no la habia quitado.
    expect(JSON.parse(guardados()[0].cuerpo)[1]).toEqual({
      titular_id: "tit-productor-a",
      ipi: "IPI-00000077",
      porcentaje: 50,
    });
    // Y el borrador sigue ahi: se puede corregir sin volver a teclearlo.
    expect(campoPorcentaje("tit-productor-a")).toHaveProperty("value", "50");
  });

  it("un porcentaje con mas de 4 decimales viaja sin redondear y el 400 lo dice", async () => {
    simularServidor({
      guardado: () => json({ error: MENSAJE_DECIMALES }, 400),
    });
    await abrirElEditor();

    escribirPorcentaje("tit-1", "66.66665");
    escribirPorcentaje("tit-2", "33,33335");

    // La coma se lee como separador decimal, y los cinco decimales NO se
    // redondean: redondear declararia una cifra que nadie escribio, y el
    // contrato dice que mas de cuatro decimales se rechaza sin redondear.
    guardar();
    expect(JSON.parse(guardados()[0].cuerpo)).toEqual([
      { titular_id: "tit-1", ipi: "IPI-00000001", porcentaje: 66.66665 },
      { titular_id: "tit-2", ipi: "IPI-00000002", porcentaje: 33.33335 },
    ]);

    const alerta = await screen.findByRole("alert");
    expect(within(alerta).getByText(MENSAJE_DECIMALES).textContent).toBe(
      MENSAJE_DECIMALES,
    );
  });

  it("una fila sin un porcentaje legible no se manda: el cuerpo lleva un numero en cada parte", async () => {
    simularServidor();
    await abrirElEditor();

    escribirPorcentaje("tit-2", "");

    expect(botonGuardar()).toHaveProperty("disabled", true);
    expect(
      screen.getByText(/Hay 1 fila sin un porcentaje que se lea como número/),
    ).toBeTruthy();

    guardar();
    expect(guardados()).toHaveLength(0);
  });

  // D-009. El 5xx NO puede afirmar que no se guardo nada: `Store.Guardar` corre
  // dentro de `EnTransaccion`, asi que un fallo del COMMIT es indistinguible de
  // uno limpio y la version PUDO haberse abierto.
  it("un 5xx no afirma que no se guardó: dice que no se sabe y manda al historial", async () => {
    simularServidor({ guardado: () => json({ error: MENSAJE_500 }, 500) });
    await abrirElEditor();

    guardar();

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain(
      "No se sabe si la declaración se guardó",
    );
    // El mensaje del backend se enseña igual; lo que no se hace es sacar de el
    // una conclusion que no sostiene.
    expect(within(alerta).getByText(MENSAJE_500).textContent).toBe(MENSAJE_500);
    expect(alerta.textContent).toMatch(/pudo haberse abierto igual/);
    expect(alerta.textContent).toMatch(/Compruébalo en el historial/);
    expect(
      within(alerta).getByRole("link", {
        name: "Ver el historial de la declaración",
      }),
    ).toBeTruthy();
    // Lo que NO puede decir.
    expect(alerta.textContent).not.toMatch(/No se guardó nada/);
    expect(alerta.textContent).not.toMatch(/no se registró/i);
    expect(alerta.textContent).not.toMatch(/vuelve a intentarlo/i);
  });

  it("sin respuesta del servidor tampoco se sabe si se guardó", async () => {
    simularServidor();
    await abrirElEditor();
    // Se cambia el doble despues de montar: la pantalla ya tiene sus lecturas.
    vi.mocked(fetch).mockImplementation((entrada, init) => {
      const url = String(entrada);
      const metodo = init?.method ?? "GET";
      if (url === "/api/auth/session") {
        return Promise.resolve(respuestaDeSesion("administrador"));
      }
      if (metodo === "PUT") return Promise.reject(new TypeError("sin red"));
      return Promise.resolve(json({ error: "ruta no encontrada" }, 404));
    });

    guardar();

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain(
      "No se sabe si la declaración se guardó",
    );
    expect(alerta.textContent).toMatch(/no se pudo contactar al servidor/);
    expect(alerta.textContent).toMatch(/pudo haberse abierto igual/);
    expect(alerta.textContent).not.toMatch(/No se guardó nada/);
  });

  it("el 200 ilegible no deja afirmar el numero de version", async () => {
    simularServidor({ guardado: () => json({ version: "cuatro" }) });
    await abrirElEditor();

    guardar();

    const panel = await screen.findByRole("alert");
    expect(panel.textContent).toContain(
      "El servidor contestó sin error, sin la versión nueva",
    );
    expect(panel.textContent).toMatch(
      /no puede decir qué versión se abrió ni con qué estado quedó/,
    );
    expect(
      within(panel).getByRole("link", {
        name: "Ver el historial de la declaración",
      }),
    ).toBeTruthy();
  });

  // CRITICAL de la revision adversarial de la iteracion 1: en esta rama el
  // cliente deja de saber que version abrio el servidor -el 200 lo prueba, el
  // cuerpo no lo dice-, y el aviso caia al historial en memoria, que ya era
  // viejo, para afirmar el numero que el propio 200 acababa de cerrar. Dos
  // textos de la MISMA pantalla que se contradecian, y el de arriba mentia.
  it("tras un 200 con cuerpo ilegible el aviso deja de afirmar que version se cierra", async () => {
    simularServidor({ guardado: () => json({ version: "cuatro" }) });
    await abrirElEditor();

    // Antes de guardar el aviso si nombra la version que rige, y la saca del
    // historial: eso no cambia.
    expect(avisoDeVersion().textContent).toMatch(
      /cerrará la versión 3 y abrirá la versión 4/,
    );

    guardar();

    await screen.findByRole("alert");
    const aviso = avisoDeVersion().textContent ?? "";
    // Lo que SI se sabe: un 200 es lo que el contrato promete cuando una
    // version se abre. Lo que no: cual es.
    expect(aviso).toMatch(
      /El servidor contestó sin error, así que una versión se abrió/,
    );
    expect(aviso).toMatch(/esta pantalla no puede decir cuál es/);
    // Y la consecuencia practica, que el panel de abajo no da: guardar otra vez
    // abre una version de mas.
    expect(aviso).toMatch(
      /Compruébalo en el historial antes de volver a guardar/,
    );
    expect(aviso).toMatch(/guardar otra vez abre una versión más/);
    // La consecuencia que si se sostiene en todos los casos.
    expect(aviso).toMatch(
      /la versión anterior, si la hay, no se borra ni se modifica/,
    );
    // NINGUN numero de version: ni el 3 del historial, ni el que el 200 acaba
    // de cerrar.
    expect(aviso).not.toMatch(/versión \d/);
    // Y no se reutiliza el texto del historial ilegible: aqui el historial se
    // leyo perfectamente, lo que cambio es el GUARDADO.
    expect(aviso).not.toMatch(/No se pudo leer el historial/);
    expect(aviso).not.toMatch(/tampoco se ha podido cargar/);
  });

  // La otra mitad del CRITICAL: un 5xx deja la misma duda -`Store.Guardar`
  // corre dentro de `EnTransaccion`, asi que el COMMIT pudo entrar- y el aviso
  // afirmaba igual cual se cerraria.
  it("tras un 5xx el aviso no afirma que version se cerrara", async () => {
    simularServidor({ guardado: () => json({ error: MENSAJE_500 }, 500) });
    await abrirElEditor();

    guardar();

    await screen.findByRole("alert");
    const aviso = avisoDeVersion().textContent ?? "";
    expect(aviso).toMatch(/No se sabe si el guardado abrió una versión/);
    expect(aviso).toMatch(
      /esta pantalla no puede decir qué versión se cerrará ni con qué número se abre la nueva/,
    );
    expect(aviso).toMatch(
      /la versión anterior, si la hay, no se borra ni se modifica/,
    );
    expect(aviso).not.toMatch(/versión \d/);
    expect(aviso).not.toMatch(/No se pudo leer el historial/);
  });

  it("sin respuesta del servidor el aviso tampoco afirma que version se cerrara", async () => {
    simularServidor();
    await abrirElEditor();
    // El doble se cambia despues de montar: la pantalla ya tiene sus lecturas.
    vi.mocked(fetch).mockImplementation((entrada, init) => {
      const url = String(entrada);
      const metodo = init?.method ?? "GET";
      if (url === "/api/auth/session") {
        return Promise.resolve(respuestaDeSesion("administrador"));
      }
      if (metodo === "PUT") return Promise.reject(new TypeError("sin red"));
      return Promise.resolve(json({ error: "ruta no encontrada" }, 404));
    });

    guardar();

    await screen.findByRole("alert");
    const aviso = avisoDeVersion().textContent ?? "";
    expect(aviso).toMatch(/No se sabe si el guardado abrió una versión/);
    expect(aviso).not.toMatch(/versión \d/);
  });

  // El limite del arreglo, y por eso tiene su propio test: un 4xx NO abre
  // ninguna version, asi que el historial en memoria sigue siendo la autoridad
  // y el aviso no cae a la forma sin numeros. Si esto se rompiera, la pantalla
  // diria menos de lo que sabe.
  it("tras un 400 el historial vuelve a ser la autoridad del aviso", async () => {
    simularServidor({ guardado: () => json({ error: MENSAJE_SUMA }, 400) });
    await abrirElEditor();

    guardar();

    await screen.findByRole("alert");
    expect(avisoDeVersion().textContent).toMatch(
      /cerrará la versión 3 y abrirá la versión 4/,
    );
  });

  // Concern 26 de la pasada 3, y el WARNING del reviewer: el status de un 4xx SI
  // llega, y un 4xx prueba que no se escribio nada. Lo que puede faltar es su
  // cuerpo. Mientras `api()` tire el status al no poder leerlo, este rechazo cae
  // en `incierta`, se pinta como un guardado en duda y la duda envenena la
  // pantalla hasta el proximo 200 legible -que es afirmar no saber algo que el
  // sistema sabe, y nombrar dos causas que no ocurrieron-.
  it("un 400 cuyo cuerpo no se puede leer es un rechazo, no un guardado en duda", async () => {
    simularServidor({ guardado: () => respuestaConCuerpoCortado(400) });
    await abrirElEditor();

    // La guarda contra el verde por vacio, la misma de los tests vecinos: antes
    // de guardar el aviso SI numera, y los numeros los saca del historial.
    expect(avisoDeVersion().textContent).toMatch(
      /cerrará la versión 3 y abrirá la versión 4/,
    );

    guardar();

    // El panel del desenlace, que es el que se pinta cuando la respuesta llega.
    await screen.findByRole("alert");

    // La asercion lee el ELEMENTO del aviso y no el documento: el panel de abajo
    // habla del mismo guardado, y una asercion sobre todo el DOM pasaria por su
    // texto.
    const aviso = avisoDeVersion().textContent ?? "";
    expect(aviso).not.toMatch(/No se sabe si el guardado abrió una versión/);
    // Y la mitad positiva, que es la que no deja pasar un aviso mudo: sin cuerpo
    // legible el rechazo sigue siendo un rechazo, asi que el historial vuelve a
    // ser la autoridad y el aviso sigue numerando.
    expect(aviso).toMatch(/cerrará la versión 3 y abrirá la versión 4/);

    // Y el panel, que es el que dice la consecuencia del rechazo: no se escribio
    // nada, asi que reintentar es seguro.
    await screen.findByText(
      /No se guardó nada: corrige el reparto y vuelve a guardarlo\./,
    );
  });

  // COBERTURA DEL ORDEN del arreglo `iter-1/step-8.1`, que es lo que impide que
  // un numero viejo sobreviva a un guardado en duda. El caso peligroso no es el
  // PRIMER guardado -que ya cubren los tests del 200 ilegible y del 5xx- sino
  // quien YA guardo una vez y recibe el 5xx en el SEGUNDO: ahi `versionGuardada`
  // esta fijada, y si la guarda del desenlace se comprobara DESPUES, el aviso
  // caeria a esa version -la que el primer PUT dio- y afirmaria con seguridad
  // una version que puede estar cerrada. Es la clase exacta del CRITICAL.
  it("un guardado legible y despues un 5xx dejan el aviso sin ningun numero de version", async () => {
    let guardadosHechos = 0;
    simularServidor({
      guardado: () => {
        guardadosHechos += 1;
        // El primero abre la version 4 con un cuerpo legible; el segundo falla
        // con 5xx -o sea `incierta`-, que es el desenlace que deja la duda.
        return guardadosHechos === 1
          ? json(versionCuatro)
          : json({ error: MENSAJE_500 }, 500);
      },
    });
    await abrirElEditor();

    guardar();
    await screen.findByRole("heading", {
      name: "El servidor abrió la versión 4",
    });

    // El aviso SI nombra los numeros despues del primer guardado, y esto no es
    // decoracion: es lo que impide que el test pase por vacio, porque si el
    // aviso no nombrara numeros nunca la negativa de abajo pasaria sola.
    expect(avisoDeVersion().textContent).toMatch(
      /cerrará la versión 4 y abrirá la versión 5/,
    );

    guardar();

    await screen.findByRole("alert");
    const aviso = avisoDeVersion().textContent ?? "";
    // El segundo guardado deja el estado en duda, y eso DESPLAZA la version que
    // el PUT anterior habia dado: ya no se sostiene como "la que esta abierta".
    // Ni el 4 del primer PUT, ni el 5 deducido.
    expect(aviso).not.toMatch(/versión \d/);
    expect(aviso).toMatch(/No se sabe si el guardado abrió una versión/);
  });

  // La otra mitad del mismo orden: el segundo guardado no falla, contesta 200,
  // pero con un cuerpo que no tiene la forma del contrato. La version del primer
  // PUT tampoco se sostiene aqui, porque el 200 acaba de cerrarla.
  it("un guardado legible y despues un 200 ilegible dejan el aviso sin ningun numero", async () => {
    let guardadosHechos = 0;
    simularServidor({
      guardado: () => {
        guardadosHechos += 1;
        return guardadosHechos === 1
          ? json(versionCuatro)
          : json({ version: "cuatro" });
      },
    });
    await abrirElEditor();

    guardar();
    await screen.findByRole("heading", {
      name: "El servidor abrió la versión 4",
    });

    // La misma guarda contra el verde por vacio que en el test de arriba.
    expect(avisoDeVersion().textContent).toMatch(
      /cerrará la versión 4 y abrirá la versión 5/,
    );

    guardar();

    await screen.findByRole("alert");
    const aviso = avisoDeVersion().textContent ?? "";
    expect(aviso).not.toMatch(/versión \d/);
    expect(aviso).toMatch(
      /El servidor contestó sin error, así que una versión se abrió/,
    );
  });

  // CRITICAL de la PASADA 2 de la revision adversarial (`findings/review-iter-1-pass2.md`): el
  // arreglo del 8.1 derivaba la duda del `resultado` del render, que es transitorio -`guardar()`
  // lo pone a `null` al empezar-, asi que el guardado SIGUIENTE devolvia el numero viejo al
  // aviso. Aqui el primero contesta 200 con un cuerpo que no es del contrato -el servidor cerro
  // la v3 y abrio la v4, y el cliente lo sabe a medias- y el segundo es un 400: la duda tiene que
  // SOBREVIVIR al 400, porque un 4xx prueba que ESA peticion no abrio nada, no lo que hizo la
  // anterior. Es la clase de D-009 reentrando por la puerta de al lado.
  it("tras un 200 ilegible, un 400 posterior no devuelve el numero viejo al aviso", async () => {
    let guardadosHechos = 0;
    simularServidor({
      guardado: () => {
        guardadosHechos += 1;
        return guardadosHechos === 1
          ? json({ version: "cuatro" })
          : json({ error: MENSAJE_SUMA }, 400);
      },
    });
    await abrirElEditor();

    // La guarda contra el verde por vacio: antes de guardar el aviso SI numera,
    // y los numeros los saca del historial. Sin esto, un aviso que no numerara
    // nunca haria pasar sola la negativa de abajo.
    expect(avisoDeVersion().textContent).toMatch(
      /cerrará la versión 3 y abrirá la versión 4/,
    );

    guardar();

    await screen.findByRole("alert");
    expect(avisoDeVersion().textContent).toMatch(
      /así que una versión se abrió/,
    );

    guardar();

    // El panel del 400, que es el que dice "No se guardó nada".
    await screen.findByText(/corrige el reparto y vuelve a guardarlo/);

    const aviso = avisoDeVersion().textContent ?? "";
    // Ningun numero: ni el 3 del historial -que el 200 acaba de cerrar-, ni el 4
    // que el primero dejo sin leer.
    expect(aviso).not.toMatch(/versión \d/);
    // Y dice DE QUE guardado habla: el panel de abajo cuenta otro, posterior, y
    // sin esa frase el aviso se lee como si describiera el que acaba de
    // rechazarse. La asercion es del parrafo ENTERO -las dos frases pegadas a la
    // prosa del 200 ilegible- porque el defecto que vigila esta ahi: una frase
    // nueva mal encajada en el JSX pierde el espacio o lo duplica y el texto se
    // lee mal sin que ninguna asercion de fragmento lo note.
    expect(aviso).toContain(
      "decir cuál es. El rechazo que cuenta el panel de abajo es de otro guardado, posterior: prueba que ESA petición no abrió ninguna versión, no lo que hizo la que dejó esta duda, así que no la resuelve. Compruébalo en el historial antes de volver a guardar:",
    );
  });

  // El mismo defecto por la puerta de en vuelo: `guardar()` pone `resultado` a
  // `null` al empezar, asi que el aviso volvia al numero viejo MIENTRAS el
  // segundo `PUT` estaba sin contestar, que es un estado que dura lo que tarde la
  // red y que nadie mira salvo que lo fije un test.
  it("mientras el segundo guardado esta en vuelo el aviso no vuelve al numero viejo", async () => {
    simularServidor({ guardado: () => json({ version: "cuatro" }) });
    await abrirElEditor();

    guardar();
    await screen.findByRole("alert");
    expect(avisoDeVersion().textContent).not.toMatch(/versión \d/);

    // A partir de aqui el PUT no contesta: el doble se cambia despues de montar,
    // que es lo unico que deja mirar la ventana en vuelo.
    vi.mocked(fetch).mockImplementation((entrada, init) => {
      const url = String(entrada);
      const metodo = init?.method ?? "GET";
      if (url === "/api/auth/session") {
        return Promise.resolve(respuestaDeSesion("administrador"));
      }
      if (metodo === "PUT") return new Promise<Response>(() => {});
      return Promise.resolve(json({ error: "ruta no encontrada" }, 404));
    });

    guardar();

    // El panel del desenlace se ha borrado -es el `setResultado(null)` del
    // arranque de `guardar()`-, o sea que la duda que queda en pantalla es la
    // que sobrevivio a ese borrado y no la de un render con desenlace.
    expect(screen.queryByRole("alert")).toBeNull();
    const aviso = avisoDeVersion().textContent ?? "";
    expect(aviso).toMatch(/así que una versión se abrió/);
    expect(aviso).not.toMatch(/versión \d/);
  });

  // WARNING de la PASADA 2: un 2xx cuyo cuerpo no parsea salia de `api()` como un
  // `Error` cualquiera y caia en `incierta`, cuya prosa dice que no se sabe si el
  // guardado abrio una version. El cliente SI recibio el 200, o sea que una
  // version se abrio -el mismo mecanismo que sostiene `guardadaSinLeer`-, y la
  // respuesta no se perdio: el aviso afirmaba de menos y nombraba una causa que no
  // ocurrio.
  it("un 200 con un cuerpo que no parsea dice que la version se abrio, no que no se sepa", async () => {
    simularServidor({
      guardado: () =>
        new Response("esto no es json", {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
    });
    await abrirElEditor();

    // La misma guarda contra el verde por vacio que en los tests de arriba.
    expect(avisoDeVersion().textContent).toMatch(
      /cerrará la versión 3 y abrirá la versión 4/,
    );

    guardar();

    // El panel del desenlace es el del cuerpo ilegible, no el del guardado
    // incierto: los dos dicen cosas distintas y la pantalla no puede elegir el
    // equivocado.
    await screen.findByRole("heading", {
      name: "El servidor contestó sin error, sin la versión nueva",
    });
    const aviso = avisoDeVersion().textContent ?? "";
    expect(aviso).toMatch(
      /El servidor contestó sin error, así que una versión se abrió/,
    );
    expect(aviso).not.toMatch(/No se sabe si el guardado abrió una versión/);
    expect(aviso).toMatch(/esta pantalla no puede decir cuál es/);
    expect(aviso).not.toMatch(/versión \d/);
  });

  // El caso que el 8.2 dejo a medias: `versionGuardada` ya esta fijada por un
  // guardado legible, y despues llegan un 5xx -que deja la duda- y un 400. El 400
  // no resuelve la duda, asi que el numero 4 del primer PUT -y el 5 que se
  // deducia- no pueden volver: la v5 puede existir.
  it("un guardado legible, un 5xx y un 400 dejan el aviso sin ningun numero", async () => {
    let guardadosHechos = 0;
    simularServidor({
      guardado: () => {
        guardadosHechos += 1;
        if (guardadosHechos === 1) return json(versionCuatro);
        if (guardadosHechos === 2) return json({ error: MENSAJE_500 }, 500);
        return json({ error: MENSAJE_SUMA }, 400);
      },
    });
    await abrirElEditor();

    guardar();
    await screen.findByRole("heading", {
      name: "El servidor abrió la versión 4",
    });
    // La guarda contra el verde por vacio, con los dos numeros que el aviso sabe
    // en ese momento.
    expect(avisoDeVersion().textContent).toMatch(
      /cerrará la versión 4 y abrirá la versión 5/,
    );

    guardar();
    await screen.findByRole("alert");
    expect(avisoDeVersion().textContent).not.toMatch(/versión \d/);

    guardar();
    await screen.findByText(/corrige el reparto y vuelve a guardarlo/);

    const aviso = avisoDeVersion().textContent ?? "";
    expect(aviso).not.toMatch(/versión \d/);
    expect(aviso).toMatch(/No se sabe si el guardado abrió una versión/);
    // La otra mitad de la combinacion nueva -duda incierta pendiente + panel de
    // un 400-, tambien pegada a su prosa entera: los dos textos hablan de
    // guardados distintos y el aviso lo dice sin pisar la consecuencia.
    expect(aviso).toContain(
      "puede decir qué versión se cerrará ni con qué número se abre la nueva. El rechazo que cuenta el panel de abajo es de otro guardado, posterior: prueba que ESA petición no abrió ninguna versión, no lo que hizo la que dejó esta duda, así que no la resuelve. Lo que sí se sabe: la versión anterior, si la hay, no se borra ni se modifica",
    );
  });

  // D-009: los tres estados del aviso. El primero -con historial- lo fija el
  // test de arriba; los otros dos, estos. Ninguno afirma un numero que no tenga,
  // y por eso la comprobacion es `versión \d` y no un numero concreto: el dia
  // que el mockup invente otro, el test sigue valiendo.
  it("sin declaracion previa el aviso dice que se abrira la primera version, sin numero", async () => {
    simularServidor({
      obra: () => json(obraSinDeclarar),
      historial: () => json([]),
    });

    await abrirElEditor("/catalogo/obra-3/declaracion");

    expect(avisoDeVersion().textContent).toMatch(
      /se abrirá la primera versión/,
    );
    expect(avisoDeVersion().textContent).toMatch(
      /No hay ninguna versión anterior que cerrar/,
    );
    // Ni un numero deducido -"se abrira la version 1"- ni la consecuencia
    // hablando de una version anterior que no existe.
    expect(avisoDeVersion().textContent).not.toMatch(/versión \d/);
    expect(avisoDeVersion().textContent).not.toMatch(
      /versión anterior, si la hay/,
    );
    // Y sin reparto que cargar el borrador empieza sin filas.
    expect(
      screen.getByText("El borrador no tiene ninguna parte."),
    ).toBeTruthy();
  });

  it("cuando el historial no se puede leer, lo dice y deja editar igual", async () => {
    simularServidor({
      historial: () => json({ error: MENSAJE_HISTORIAL }, 500),
    });

    await abrirElEditor();

    const aviso = avisoDeVersion().textContent ?? "";
    expect(aviso).toMatch(/No se pudo leer el historial de esta declaración/);
    expect(aviso).toContain(MENSAJE_HISTORIAL);
    expect(aviso).toMatch(/no se borra ni se modifica/);
    expect(aviso).toMatch(/queda en el historial/);
    // Lo que no se puede afirmar: ningun numero, ni que el borrador venga con
    // un reparto que nadie pudo leer.
    expect(aviso).not.toMatch(/versión \d/);
    expect(aviso).toMatch(/tampoco se ha podido cargar con el reparto vigente/);
    expect(
      screen.getByText("El borrador no tiene ninguna parte."),
    ).toBeTruthy();
    // El padron si esta: lo que falta es el reparto escrito, no los titulares.
    expect(
      within(filaDelPadron("tit-1")).getByRole("button", {
        name: "Añadir al reparto",
      }),
    ).toBeTruthy();
  });

  it("el aviso nombra las versiones del historial y nunca las del mockup", async () => {
    // Una obra por la version 7: el copy fijo "se cerrara la version 2 y se
    // abrira una version 3" del mockup seria falso aqui, y es exactamente el
    // defecto que D-009 cierra.
    simularServidor({
      obra: () => json(obraAvanzada),
      historial: () => json([versionSeisCerrada, versionSieteAbierta]),
    });

    await abrirElEditor("/catalogo/obra-7/declaracion");

    expect(avisoDeVersion().textContent).toMatch(
      /cerrará la versión 7 y abrirá la versión 8/,
    );
    expect(avisoDeVersion().textContent).not.toContain("versión 2");
    expect(avisoDeVersion().textContent).not.toContain("versión 3");
  });

  it("un historial sin ninguna version abierta no deja afirmar cual se cierra", async () => {
    simularServidor({ historial: () => json([versionCerrada]) });

    await abrirElEditor();

    const aviso = avisoDeVersion().textContent ?? "";
    expect(aviso).toMatch(
      /Con lo que el servidor devolvió en el historial, esta pantalla no puede decir qué versión se cerrará/,
    );
    expect(aviso).toMatch(/no se borra ni se modifica/);
    expect(aviso).not.toMatch(/versión \d/);
  });

  // Hard rule 2: solo se ofrecen los titulares que el backend aceptaria, y a
  // quien no se ofrece se le explica por que en vez de faltar.
  it("el padron ofrece a las personas naturales y explica por que no ofrece a una sociedad", async () => {
    simularServidor();
    await abrirElEditorConPadron();

    const padron = screen.getByRole("table", { name: "Padrón de titulares" });

    // Ana es socia y Beto administrado: los dos son personas naturales y los dos
    // se ofrecen. La clase no decide quien puede ser parte.
    expect(within(padron).getByText("Ana Escritora")).toBeTruthy();
    expect(within(padron).getAllByText("socio")).toHaveLength(2);
    expect(within(padron).getAllByText("administrado")).toHaveLength(2);
    expect(
      within(filaDelPadron("tit-3")).getByRole("button", {
        name: "Añadir al reparto",
      }),
    ).toBeTruthy();

    // La productora existe en el padron y NO se ofrece, con la razon delante.
    const productora = filaDelPadron("tit-productor-a");
    expect(
      within(productora).getByText("Productora del Caribe S.A.S."),
    ).toBeTruthy();
    expect(
      within(productora).queryByRole("button", { name: "Añadir al reparto" }),
    ).toBeNull();
    expect(productora.textContent).toMatch(
      /No: solo un escritor persona natural/,
    );
    expect(productora.textContent).toContain("R-01");
    expect(productora.textContent).toContain("RD 4.5");

    // Y a quien ya esta en el reparto no se le ofrece otra vez: el backend
    // rechaza un titular repetido.
    expect(filaDelPadron("tit-1").textContent).toContain(
      "Ya está en el reparto",
    );
  });

  // El padron SI admite filtrar por identificador: `GET /titulares?ids=` es la
  // consulta acotada que documenta el contrato y que usa el detalle para pedir
  // los nombres. Esta nota decia lo contrario -era verdad cuando se escribio- y
  // era la unica copia del hecho, y la unica que lee quien escribe el reparto,
  // que nadie actualizo cuando el contrato cambio.
  it("la nota del padron no afirma que no se pueda filtrar por identificador", async () => {
    simularServidor();
    await abrirElEditorConPadron();

    // Se busca la nota por lo que SI dice y que sigue siendo verdad: una pagina
    // que no trae al titular no prueba que no exista. Si esa frase se quita, la
    // prueba falla aqui y no pasa en silencio por no tener nada que mirar.
    const nota = screen.getByText(/una página que no traiga al titular/);

    expect(nota.textContent).not.toMatch(/no admite filtrar/i);
    expect(nota.textContent).not.toMatch(/no se puede filtrar/i);
    expect(nota.textContent).not.toMatch(/sin filtro por identificador/i);
  });

  it("anadir un titular del padron crea su fila con el IPI y sin porcentaje inventado", async () => {
    simularServidor();
    await abrirElEditorConPadron();

    agregarAlReparto("tit-3");

    // El porcentaje nace VACIO: la cifra la escribe quien declara, y un 0 de
    // relleno acabaria guardado como si se hubiera declarado.
    expect(campoPorcentaje("tit-3")).toHaveProperty("value", "");
    expect(campoIpi("tit-3")).toHaveProperty("value", "IPI-00000003");
    // Y mientras falte ese numero no se manda nada: el cuerpo lleva una cifra
    // en cada parte.
    expect(botonGuardar()).toHaveProperty("disabled", true);

    // Con el porcentaje escrito, el borrador entero es guardable.
    escribirPorcentaje("tit-1", "40");
    escribirPorcentaje("tit-2", "30");
    escribirPorcentaje("tit-3", "30");
    expect(dato("Estado del borrador")).toContain("Completa");
    expect(botonGuardar()).toHaveProperty("disabled", false);
  });

  it("el padron se pagina con el limite explicito y se vuelve a pedir al buscar", async () => {
    simularServidor();
    await abrirElEditorConPadron();

    // La busqueda va al servidor y vuelve a la primera pagina: una pagina 3 de
    // otra busqueda no existe.
    fireEvent.change(screen.getByLabelText("Buscar por nombre"), {
      target: { value: "Ana" },
    });

    const delPadron = () =>
      peticiones().filter((p) => p.url.startsWith("/api/titulares"));
    // La consulta espera a que el tecleo pare (item 11), asi que la peticion no
    // sale en el mismo act del `change`. Lo que se afirma es CUAL es la
    // consulta y con que parametros, no en que instante sale.
    await vi.waitFor(() =>
      expect(delPadron().at(-1)?.url).toBe(
        "/api/titulares?nombre=Ana&limite=50",
      ),
    );

    // Y el limite viaja explicito, sin `persona_natural`, en todas.
    expect(delPadron().every((p) => p.url.includes("limite=50"))).toBe(true);
    expect(delPadron().every((p) => !p.url.includes("persona_natural"))).toBe(
      true,
    );
  });

  it("sin padron se dice, y lo que ya esta en el borrador se sigue guardando", async () => {
    simularServidor({
      titulares: () => json({ error: "no se pudo consultar el padron" }, 500),
    });

    await abrirElEditor();

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain(
      "No se pudo consultar el padrón de titulares:",
    );
    expect(alerta.textContent).toContain("no se pudo consultar el padron");
    // El padron es la oferta, no la autoridad: el borrador que vino del
    // historial se guarda igual.
    expect(botonGuardar()).toHaveProperty("disabled", false);
    guardar();
    expect(guardados()).toHaveLength(1);
  });

  // S4 del issue #30, con el camino entero: guardar abre una version y la
  // anterior sigue en el historial.
  it("guardar abre una version nueva y la anterior sigue visible en el historial", async () => {
    let historial = HISTORIAL;
    simularServidor({
      historial: () => json(historial),
      guardado: () => {
        // Lo que hace el backend: cierra la abierta y añade una nueva al final.
        historial = [...HISTORIAL, versionCuatro];
        return json(versionCuatro);
      },
    });
    await abrirElEditor();

    escribirPorcentaje("tit-1", "60");
    escribirPorcentaje("tit-2", "40");
    guardar();

    const panel = await screen.findByRole("heading", {
      name: "El servidor abrió la versión 4",
    });
    const caja = panel.closest("section");
    if (!caja) throw new Error("el panel del guardado no tiene seccion");
    expect(within(caja).getByText("Completa")).toBeTruthy();
    expect(
      within(caja).getByText(/La versión anterior no se borra ni se modifica/),
    ).toBeTruthy();

    // El aviso queda al dia SIN releer el historial: la version que el servidor
    // acaba de abrir es la que se cerrara la proxima vez. Con el historial
    // viejo diria que se cierra la 3, que ya no rige.
    expect(avisoDeVersion().textContent).toMatch(
      /cerrará la versión 4 y abrirá la versión 5/,
    );

    // Y el enlace al historial lleva a la version anterior, que sigue ahi.
    fireEvent.click(
      screen.getByRole("link", { name: "Ver el historial de la declaración" }),
    );

    await screen.findByRole("heading", { name: "Historial de la declaración" });
    await screen.findByText("Versión 4");
    expect(screen.getByText("Versión 2")).toBeTruthy();
    expect(screen.getByText("Versión 3")).toBeTruthy();
    // El reparto de la version 2 -60 y 40- sigue visible tal como se declaro
    // entonces: un periodo pasado tiene que poder reproducirse con el split que
    // estaba vigente entonces.
    expect(screen.getAllByText("60.0000%").length).toBeGreaterThan(0);
  });

  // WARNING de la revision adversarial: el parrafo del exito prometia, sin
  // condicion, que "la version anterior no se borra ni se modifica" tambien
  // cuando la que se abre es la 1, donde no hay ninguna version anterior. El
  // aviso hermano de la misma pantalla ya se curaba con "si la hay" y el aviso
  // previo de este mismo flujo dice que no hay ninguna que cerrar.
  it("guardar la primera declaracion no promete una version anterior que no hay", async () => {
    simularServidor({
      obra: () => json(obraSinDeclarar),
      historial: () => json([]),
      guardado: () => json(primeraVersion),
    });

    await abrirElEditor("/catalogo/obra-3/declaracion");
    await screen.findByRole("table", { name: "Padrón de titulares" });

    // El aviso previo ya lo decia: no hay ninguna version anterior que cerrar.
    expect(avisoDeVersion().textContent).toMatch(
      /No hay ninguna versión anterior que cerrar/,
    );

    // El historial vacio no trae filas, asi que el reparto se arma desde el
    // padron: es el camino real de la primera declaracion de una obra.
    agregarAlReparto("tit-1");
    escribirPorcentaje("tit-1", "100");
    guardar();

    const panel = await screen.findByRole("heading", {
      name: "El servidor abrió la versión 1",
    });
    const caja = panel.closest("section");
    if (!caja) throw new Error("el panel del guardado no tiene seccion");
    // Lo que si se sostiene con la version 1: la declaracion queda en el
    // historial y un periodo pasado se reproduce con el reparto que regia
    // entonces.
    expect(
      within(caja).getByText(/Esta es la primera declaración de la obra/),
    ).toBeTruthy();
    expect(caja.textContent).toMatch(
      /antes de esta versión no había ninguno declarado/,
    );
    // Y lo que no: una version anterior que conserve nada.
    expect(caja.textContent).not.toMatch(
      /La versión anterior no se borra ni se modifica/,
    );
    expect(caja.textContent).not.toMatch(
      /sigue en el historial con los porcentajes que regían hasta ahora/,
    );
    // El aviso de arriba si queda al dia con la version que el servidor abrio:
    // la que se cerrara la proxima vez es esta.
    expect(avisoDeVersion().textContent).toMatch(
      /cerrará la versión 1 y abrirá la versión 2/,
    );
  });

  it("un 404 al guardar dice que la obra ya no esta, con el mensaje del servidor", async () => {
    simularServidor({
      guardado: () => json({ error: "esa obra no esta en el catalogo" }, 404),
    });
    await abrirElEditor();

    guardar();

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain("Esa obra no está en el catálogo");
    expect(alerta.textContent).toContain("esa obra no esta en el catalogo");
    // Un 404 cierra la pregunta: no hay ninguna version que pueda haberse
    // abierto sobre una obra que no esta.
    expect(alerta.textContent).not.toMatch(/pudo haberse abierto/);
    // Y por eso mismo SI se puede decir que no se guardo nada: el 404 sale como
    // error de `Gestion.Guardar` y `EnTransaccion` revierte.
    expect(alerta.textContent).toMatch(/No se guardó nada/);
    // Con el aviso de que reintentar no lo arregla: sin obra no hay version que
    // abrir. Es lo que le dice a quien edita que no vuelva a guardar.
    expect(alerta.textContent).toMatch(/volver a guardarlo no escribe ninguna/);
  });

  // El 401 tiene su propio desenlace y NO es un panel: `api()`
  // (web/src/api.ts:91-94) trata un 401 de una llamada con token como sesion
  // vencida -borra el token y avisa a `ProveedorDeSesion`-, `RutaProtegida`
  // manda al login y el editor se desmonta. Por eso la pantalla no afirma nada
  // del guardado en este caso: no hay donde. Se prueba el desenlace real, que es
  // lo que la pantalla hace, en vez de fabricar un panel que no puede existir.
  it("un 401 al guardar cierra la sesion y vuelve el login, sin panel que afirme nada", async () => {
    simularServidor({
      guardado: () => json({ error: "sesion invalida o expirada" }, 401),
    });
    await abrirElEditor();

    guardar();

    await screen.findByRole("heading", { name: "Iniciar sesión" });
    // Ni panel de rechazo ni aviso de guardado incierto: la pantalla se fue.
    expect(screen.queryByRole("alert")).toBeNull();
    expect(
      screen.queryByRole("heading", { name: "Declaración de la obra" }),
    ).toBeNull();
    // Y el guardado se intento una sola vez: no hay reintento automatico.
    expect(guardados()).toHaveLength(1);
  });

  it("un 403 se dice como lo que es: el rol no basta", async () => {
    simularServidor({ guardado: () => json({ error: "no autorizado" }, 403) });
    await abrirElEditor();

    guardar();

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain(
      "El servidor no autoriza este guardado",
    );
    expect(alerta.textContent).toContain("no autorizado");
    expect(alerta.textContent).not.toMatch(/pudo haberse abierto/);
    // Y tambien aqui se dice que no se guardo nada: el 403 lo produce
    // `requiereRol` sobre el grupo `/obras`, antes de llegar al handler.
    expect(alerta.textContent).toMatch(/No se guardó nada/);
    // Un 403 no se arregla corrigiendo el reparto, y el aviso lo dice para que
    // nadie reintente en vano.
    expect(alerta.textContent).toMatch(
      /volver a guardarlo con esta sesión dará lo mismo/,
    );
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

    montarApp("/catalogo/obra-1/declaracion");

    await vi.waitFor(() =>
      expect(consultas().slice(0, 2)).toEqual([
        "/api/obras/obra-1",
        "/api/obras/obra-1/declaracion/historial",
      ]),
    );
  });

  it("una obra inexistente lo dice, sin montar el editor", async () => {
    simularServidor({
      obra: () => json({ error: "esa obra no esta en el catalogo" }, 404),
    });

    montarApp("/catalogo/obra-9/declaracion");

    await screen.findByRole("heading", {
      name: "Esa obra no está en el catálogo",
    });
    expect(screen.getByText("obra-9")).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
    // El historial se pide junto a la obra, en el mismo tick, y no se pinta.
    expect(consultas()).toEqual([
      "/api/obras/obra-9",
      "/api/obras/obra-9/declaracion/historial",
    ]);
    expect(
      screen
        .getByRole("link", { name: /Volver al catálogo/ })
        .getAttribute("href"),
    ).toBe("/catalogo");
  });

  it("un fallo al leer la obra se muestra con el mensaje del backend", async () => {
    simularServidor({
      obra: () => json({ error: "la base esta caida" }, 500),
    });

    montarApp(LA_DECLARACION);

    const alerta = await screen.findByRole("alert");
    expect(alerta.textContent).toContain("No se pudo consultar la obra:");
    expect(alerta.textContent).toContain("la base esta caida");
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("mientras llega la respuesta dice que esta cargando, sin formulario", async () => {
    vi.mocked(fetch).mockImplementation((entrada) => {
      if (String(entrada) === "/api/auth/session") {
        return Promise.resolve(respuestaDeSesion("administrador"));
      }
      return new Promise<Response>(() => {});
    });

    montarApp(LA_DECLARACION);

    await screen.findByRole("link", { name: "Catálogo" });
    expect(screen.getByRole("status")).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.queryByRole("button", { name: /^Guardar/ })).toBeNull();
  });

  it("un auditor no ve el editor: el guard de rol se hereda por el prefijo de la ruta", async () => {
    simularServidor({ rol: "auditor" });

    montarApp(LA_DECLARACION);

    await screen.findByRole("heading", { name: "No autorizado" });
    expect(
      screen.queryByRole("heading", { name: "Declaración de la obra" }),
    ).toBeNull();
    expect(consultas()).toEqual([]);
  });

  it("vuelve a la obra, que es de donde se llega", async () => {
    simularServidor();
    await abrirElEditor();

    expect(
      screen
        .getByRole("link", { name: /Volver a la obra/ })
        .getAttribute("href"),
    ).toBe("/catalogo/obra-1");
  });

  // Un 2xx que no trae lo prometido: `useApi<T>` no comprueba la forma -`T` es
  // una promesa, no una verificacion- y sin ErrorBoundary en `web/src` el primer
  // campo mal formado deja la pantalla en blanco.
  it("un padron con una entrada ilegible deja el padron en error, sin tumbarlo", async () => {
    simularServidor({
      titulares: () => json([{ id: "tit-1", nombre: "Ana Escritora" }]),
    });

    await abrirElEditor("/catalogo/obra-1/declaracion");

    const alerta = screen.getByRole("alert");
    expect(alerta.textContent).toContain(
      "El padrón no llegó como una lista de titulares legibles.",
    );
    expect(
      screen.queryByRole("table", { name: "Padrón de titulares" }),
    ).toBeNull();
    // El borrador, que viene de otra lectura, sigue montado y guardable.
    expect(botonGuardar()).toHaveProperty("disabled", false);
  });

  it("un historial sin ninguna version abierta SI dice que el borrador esta vacio", async () => {
    // El item 6: `filasDePartida` devuelve `[]` cuando no hay ninguna version
    // abierta -las dos preguntan por `versionAbierta`-, asi que la rejilla sale
    // sin filas. El aviso decia `sinBorrador: false` y por tanto no lo decia:
    // afirmaba que el borrador existe mientras la tabla estaba vacia.
    simularServidor({ historial: () => json([versionCerrada]) });

    await abrirElEditor();

    // La premisa del caso: ninguna version abierta, y el borrador vacio.
    expect(screen.queryByLabelText(/^Porcentaje de /)).toBeNull();
    expect(avisoDeVersion().textContent).toContain(
      "El borrador tampoco se ha podido cargar con el reparto vigente.",
    );
  });

  it("con la version abierta presente NO se dice que el borrador falte", async () => {
    // Control negativo: la frase es de un hecho del historial, no un texto fijo
    // que se cuele en todas las pantallas.
    simularServidor();

    await abrirElEditor();

    expect(campoPorcentaje("tit-1")).toBeTruthy();
    expect(texto()).not.toContain(
      "El borrador tampoco se ha podido cargar con el reparto vigente.",
    );
  });

  it("si el historial no cuadra con la version vigente, no ofrece guardar y lo dice", async () => {
    // El item 7: la pantalla que ESCRIBE tiene que ser tan estricta como la que
    // lee. La obra declara vigente la 7 y el historial solo trae la 3 abierta:
    // guardar cerraria una version que `GET /obras/{id}` no esta sosteniendo.
    simularServidor({ obra: () => json(obraAvanzada) });

    await abrirElEditor("/catalogo/obra-7/declaracion");

    // El borrador SI tiene filas y el total SI es 100: lo que falta no es el
    // reparto, es que las dos lecturas de la obra no cuadran. Sin esto, un
    // arreglo que cerrara la puerta por cualquier motivo pasaria igual.
    expect(campoPorcentaje("tit-1")).toHaveProperty("value", "75");
    expect(dato("Total del borrador (calculado en esta pantalla)")).toContain(
      "100",
    );

    expect(botonGuardar()).toHaveProperty("disabled", true);
    expect(texto()).toContain(
      "El servidor declara vigente la versión 7, pero el historial no trae esa versión como única versión abierta.",
    );
    // Y el texto no atribuye el bloqueo al borrador, que esta bien: sin la
    // guarda del motivo, esta pantalla diria "el total pasa de 100" con el
    // total en 100.
    expect(texto()).not.toContain("El total pasa de 100");

    // Ni con un clic se guarda.
    guardar();
    expect(guardados()).toHaveLength(0);
  });

  it("cuando el historial SI cuadra con la version vigente, se ofrece guardar", async () => {
    // Control positivo: la puerta no se cierra de mas. Es la misma obra de
    // arriba con el historial coherente.
    simularServidor({
      obra: () => json(obraAvanzada),
      historial: () => json([versionSeisCerrada, versionSieteAbierta]),
    });

    await abrirElEditor("/catalogo/obra-7/declaracion");

    expect(botonGuardar()).toHaveProperty("disabled", false);
    expect(texto()).not.toContain(
      "pero el historial no trae esa versión como única versión abierta",
    );
  });

  it("el total, el estado y el aviso de la version se anuncian", async () => {
    // El item 8a. Sin `role="status"`, cambiar un porcentaje mueve dos cifras
    // que un lector de pantalla no anuncia: quien no ve la pantalla no se
    // entera de que su reparto ya no suma 100.
    simularServidor();
    await abrirElEditor();

    const total = nodoVivo("Total del borrador (calculado en esta pantalla)");
    const estado = nodoVivo("Estado del borrador");
    const antesDelTotal = total.textContent;
    const antesDelEstado = estado.textContent;

    // El nodo vivo es el que CAMBIA: si el que lleva el rol fuera otro, estas
    // dos afirmaciones no dirian nada del que se anuncia.
    escribirPorcentaje("tit-1", "60");
    expect(total.textContent).not.toBe(antesDelTotal);
    expect(estado.textContent).not.toBe(antesDelEstado);

    // Y el aviso de la version tambien: es lo que hay que entender ANTES de
    // tocar nada, y aparece sin que nadie haya movido el foco.
    expect(avisoDeVersion().getAttribute("role")).toBe("status");
  });

  // El item 8b. El foco es lo unico que quien navega con teclado tiene para no
  // perderse: si al quitar una fila cae a `<body>`, el `Tab` siguiente empieza
  // otra vez por el principio de la pagina.
  it("al quitar la fila 2 el foco pasa a la fila que ocupa su lugar, y no a <body>", async () => {
    simularServidor();
    await abrirElEditorConPadron();

    // Tres filas: las dos del reparto vigente mas la que se anade del padron.
    // Asi la fila quitada no es la ultima, que es el caso que el arreglo trata
    // por separado.
    agregarAlReparto("tit-3");

    quitarDelReparto("tit-2");

    // Las dos cosas POR SEPARADO, y ninguna vale sola: "no es `<body>`" lo
    // cumpliria tambien el foco en cualquier otro sitio de la pantalla -que
    // sigue perdiendo el sitio de la rejilla-, y "esta en la rejilla" no dice
    // nada contra `<body>` si la rejilla no existiera.
    const activo = document.activeElement;
    expect(activo).not.toBe(document.body);
    expect(activo?.tagName === "INPUT" || activo?.tagName === "BUTTON").toBe(
      true,
    );
    expect(rejilla().contains(activo)).toBe(true);
    // Y es el control de la fila que se corrio al indice de la quitada: tit-2
    // estaba en el indice 1 y quien lo ocupa ahora es tit-3.
    expect(filaDelBorrador("tit-3").contains(activo)).toBe(true);
  });

  it("al quitar la primera fila el foco va a la que sube a ese indice, no a la ultima", async () => {
    simularServidor();
    await abrirElEditorConPadron();
    agregarAlReparto("tit-3");

    quitarDelReparto("tit-1");

    // Con tres filas, "la que sube al indice 0" es tit-2 y "la ultima" es tit-3.
    // Con dos filas serian la misma y esta prueba no diria cual de las dos reglas
    // se cumple. Lo que se exige es el hueco; saltar a la ultima seria saltarse
    // una fila, que es perder el sitio de otra manera.
    const activo = document.activeElement;
    expect(filaDelBorrador("tit-2").contains(activo)).toBe(true);
    expect(filaDelBorrador("tit-3").contains(activo)).toBe(false);
  });

  it("el salto de foco no se repite al teclear: la intencion se consume una sola vez", async () => {
    simularServidor();
    await abrirElEditorConPadron();

    agregarAlReparto("tit-3");
    quitarDelReparto("tit-2");

    // El efecto corre en CADA cambio del borrador -teclear tambien cambia
    // `filas`-, y lo que lo limita a un solo salto es consumir la intencion al
    // usarla. Sin eso, quien escribe no podria terminar una cifra: el foco
    // volveria a la fila del relevo en cada tecla, aunque el foco estuviera en
    // otra fila.
    const campo = campoPorcentaje("tit-1");
    campo.focus();
    escribirPorcentaje("tit-1", "70");

    expect(document.activeElement).toBe(campo);
  });

  it("al anadir un titular del padron el foco esta en la fila nueva", async () => {
    simularServidor();
    await abrirElEditorConPadron();

    agregarAlReparto("tit-3");

    const activo = document.activeElement;
    expect(activo).not.toBe(document.body);
    expect(rejilla().contains(activo)).toBe(true);
    // La fila NUEVA, no la primera de la rejilla ni el padron: se acaba de
    // elegir un titular y lo que se escribe despues es su porcentaje.
    expect(filaDelBorrador("tit-3").contains(activo)).toBe(true);
  });

  it("con la rejilla vacia no hay control al que llevar el foco, y la prueba no lo exige", async () => {
    // Control positivo, para que el caso de arriba no pase por el motivo
    // equivocado: la regla "el foco acaba en un control de la rejilla" no se
    // puede exigir cuando quitar la ultima fila deja el borrador sin ninguna.
    // Sin filas no hay tabla ni control, asi que no hay destino que prometer: lo
    // que se exige aqui es lo contrario -que la pantalla diga que el borrador
    // quedo vacio-, y que el foco se quede donde cae. `<body>` es lo MEDIDO en
    // este caso, y esta declarado como lo esperado.
    simularServidor();
    await abrirElEditorConPadron();

    quitarDelReparto("tit-1");
    quitarDelReparto("tit-2");

    expect(
      screen.queryByRole("table", { name: "Borrador del reparto" }),
    ).toBeNull();
    expect(
      screen.getByText("El borrador no tiene ninguna parte."),
    ).toBeTruthy();
    expect(document.activeElement).toBe(document.body);
  });

  it("el boton de quitar nombra a quien quita, en vez de leerse al reves", async () => {
    simularServidor();
    await abrirElEditorConPadron();

    // El nombre accesible -lo que anuncia un lector de pantalla- decia "Quitar
    // de tit-1", que se lee como si se quitara algo DE tit-1 y no dice a quien
    // se saca del reparto.
    const boton = within(filaDelBorrador("tit-1")).getByRole("button", {
      name: "Quitar a tit-1 del reparto",
    });
    // Y el texto visible no cambia de significado: sigue diciendo la accion y el
    // titular que ya estaban escritos.
    expect(boton.textContent?.trim()).toBe("Quitar de tit-1");
  });

  it("un historial con una entrada ilegible se lee como no leido, sin numeros inventados", async () => {
    simularServidor({
      historial: () => json([{ ...versionVigente, vigente_hasta: undefined }]),
    });

    await abrirElEditor();

    const aviso = avisoDeVersion().textContent ?? "";
    expect(aviso).toMatch(/no puede decir qué versión se cerrará/);
    expect(aviso).not.toMatch(/versión \d/);
    // Y sin reparto legible el borrador no se rellena a medias: se dice que no
    // se pudo cargar y no se pinta ni una fila.
    expect(aviso).toMatch(/tampoco se ha podido cargar con el reparto vigente/);
    expect(screen.queryByLabelText(/Porcentaje de/)).toBeNull();
  });

  // El item 9a. La rejilla tenia el nombre EN LA MANO -`agregarTitular(titular)`
  // recibe el `Titular` entero del padron- y lo tiraba: guardaba su `id` y su
  // `ipi`, y la primera columna pintaba el `titular_id`.
  it("la fila que se anade del padron dice el nombre del titular, con su identificador al lado", async () => {
    simularServidor();
    await abrirElEditorConPadron();

    // Antes de pulsar nada: el titular que se va a pulsar ES el que se va a
    // buscar despues por su nombre, y el padron de este test le da ese nombre.
    // Sin esta comprobacion, un padron que no lo trajera -o un clic que no
    // casara ninguna fila- dejaria lo de abajo midiendo el vacio, y la prueba
    // pasaria por el motivo equivocado.
    expect(filaDelPadron("tit-3").textContent).toContain("Caro Guionista");

    agregarAlReparto("tit-3");

    // El nombre se busca DENTRO de la rejilla: el padron de abajo dice el mismo
    // nombre del mismo titular, asi que buscarlo en toda la pantalla lo
    // encontraria en la tabla de abajo aunque la fila del borrador no lo dijera.
    const fila = filaDelBorrador("tit-3");
    expect(within(fila).getByText("Caro Guionista")).toBeTruthy();
    // Y la PRIMERA COLUMNA dice el nombre con el identificador al lado, en ese
    // orden: el identificador es la identidad que viaja en la declaracion y lo
    // que deja conciliar la fila con la API, y el nombre es como se lee.
    expect(fila.querySelector("td")?.textContent?.trim()).toMatch(
      /^Caro Guionista\s+tit-3$/,
    );
  });

  // El control negativo, y es el que impide que la prueba de arriba pase por el
  // padron: la fila sembrada desde el historial NO trae nombre, porque la `Parte`
  // no lo lleva (D-006). Eso no es un hueco a rellenar ni un dato que falte en el
  // test: es lo que la fila sabe.
  it("una fila sembrada desde el historial no inventa un nombre, y se queda con su identificador", async () => {
    simularServidor();
    await abrirElEditorConPadron();

    // `tit-1` esta en el reparto vigente -su fila nace de `filasDePartida`- y
    // esta en el padron: el nombre SI esta disponible en la pantalla, y lo que se
    // mide es que la fila del borrador no lo tiene.
    expect(filaDelPadron("tit-1").textContent).toContain("Ana Escritora");

    const fila = filaDelBorrador("tit-1");
    // La primera columna dice el identificador y nada mas: ni el nombre, ni un
    // guion que anuncie un dato que falta. Un guion aqui diria que la fila esta
    // incompleta cuando lo que pasa es que el nombre no viaja en la version.
    expect(fila.querySelector("td")?.textContent?.trim()).toBe("tit-1");
    expect(within(fila).queryByText("Ana Escritora")).toBeNull();
  });

  // El cierre del paso 11: su boton de quitar se nombro con el `titular_id`
  // porque la fila no tenia el nombre. Ahora la fila que lo sabe lo dice, y la que
  // no lo sabe sigue nombrando por su identificador.
  it("el boton de quitar nombra al titular por su nombre, y cae al identificador cuando la fila no lo tiene", async () => {
    simularServidor();
    await abrirElEditorConPadron();

    agregarAlReparto("tit-3");

    // La fila recien elegida del padron: el nombre accesible dice el nombre.
    expect(
      within(filaDelBorrador("tit-3")).getByRole("button", {
        name: "Quitar a Caro Guionista del reparto",
      }),
    ).toBeTruthy();
    // Y la que vino del historial -sin nombre- sigue diciendo su identificador.
    // Es el respaldo MEDIDO, no una suposicion: si el nombre se usara a secas, o
    // si se dejara de usar cuando lo hay, una de las dos busquedas no resolveria.
    expect(
      within(filaDelBorrador("tit-1")).getByRole("button", {
        name: "Quitar a tit-1 del reparto",
      }),
    ).toBeTruthy();

    // El helper de esta suite compone la etiqueta con ese mismo criterio, y esta
    // llamada lo ejerce: con una cadena fija por `titular_id` no resolveria.
    quitarDelReparto("tit-3", "Caro Guionista");
    expect(within(rejilla()).queryByLabelText("IPI de tit-3")).toBeNull();
  });
});

describe("el detalle de obra lleva al editor (S4, alcanzable por un enlace real)", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    setToken("tok");
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  const OBRAS: [string, Obra, string, string][] = [
    [
      "una obra ya declarada",
      obraDeclarada,
      "Editar el reparto",
      "/catalogo/obra-1/declaracion",
    ],
    [
      "una obra sin declarar",
      obraSinDeclarar,
      "Declarar el reparto",
      "/catalogo/obra-3/declaracion",
    ],
  ];

  it.each(OBRAS)(
    "el detalle de %s enlaza con el editor de ESA obra",
    async (_caso, obra, texto, destino) => {
      simularServidor({
        obra: () => json(obra),
        historial: () =>
          json(obra.version_vigente === null ? [] : [versionVigente]),
      });

      montarApp(`/catalogo/${obra.id}`);
      await screen.findByRole("link", { name: texto });

      const enlace = screen.getByRole("link", { name: texto });
      expect(enlace.getAttribute("href")).toBe(destino);
      // El destino lleva el `id` de la obra que se esta viendo: un enlace al
      // editor de otra obra pasaria un test que solo mirara el texto.
      expect(destino).toContain(obra.id);
    },
  );

  it("el enlace de la ficha abre el editor de verdad", async () => {
    simularServidor();

    montarApp("/catalogo/obra-1");
    fireEvent.click(
      await screen.findByRole("link", { name: "Editar el reparto" }),
    );

    await screen.findByRole("table", { name: "Padrón de titulares" });
    expect(
      screen.getByRole("heading", { name: "Declaración de la obra" }),
    ).toBeTruthy();
    // El editor pide la obra del `id` de la direccion, que es el que llevaba el
    // enlace.
    expect(consultas()).toContain("/api/obras/obra-1");
  });
});

/**
 * El 🟠, y por que este describe NO puede afirmar "0 PUT tras un Enter".
 *
 * `jsdom` no implementa el envio implicito del formulario: medido con el jsdom
 * del repo, un `keydown` sintetico con Enter en un `<input type="text">` no
 * dispara `submit`, mientras que `requestSubmit()` y `click()` si. Un
 * `expect(guardados()).toHaveLength(0)` despues de un Enter **pasa en el arbol
 * roto tambien**, asi que no seria evidencia de nada.
 *
 * Lo que si discrimina, y es lo que se afirma aqui, es la caja blanca: el
 * handler del formulario CANCELA el evento. Sin `onKeyDown` el evento llega al
 * `form` sin cancelar y el navegador real hace el envio implicito. La otra mitad
 * de la prueba es el probe de navegador (`Intela-wt/_verify-135/probe-enter.mjs`,
 * 0 PUT con Enter y 1 PUT con el boton), que jsdom no puede sustituir.
 */
describe("el Enter de un campo de texto no guarda (el 🟠)", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    setToken("tok");
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("Enter en el porcentaje cancela el evento, y la fila sigue sin guardarse", async () => {
    simularServidor();
    await abrirElEditorConPadron();
    escribirPorcentaje("tit-1", "7");

    const evento = createEvent.keyDown(campoPorcentaje("tit-1"), {
      key: "Enter",
    });
    fireEvent(campoPorcentaje("tit-1"), evento);

    // El handler tiene que haber cancelado el envio implicito. Sin `onKeyDown`
    // esto es `false`, y en un navegador de verdad ese `false` es una version
    // nueva abierta con el "7" a medio teclear.
    expect(evento.defaultPrevented).toBe(true);
    // Y el camino legitimo no se toco: nada salio por su cuenta.
    expect(guardados()).toHaveLength(0);
  });

  it("Enter en el campo del IPI tambien se cancela", async () => {
    simularServidor();
    await abrirElEditorConPadron();

    const evento = createEvent.keyDown(campoIpi("tit-1"), { key: "Enter" });
    fireEvent(campoIpi("tit-1"), evento);

    expect(evento.defaultPrevented).toBe(true);
  });

  it("cualquier otra tecla en el campo no se cancela", async () => {
    simularServidor();
    await abrirElEditorConPadron();

    // Control negativo: el handler cancela Enter y solo Enter. Un handler que
    // cancelara todo dejaria el formulario sin teclado.
    const evento = createEvent.keyDown(campoPorcentaje("tit-1"), {
      key: "Tab",
    });
    fireEvent(campoPorcentaje("tit-1"), evento);

    expect(evento.defaultPrevented).toBe(false);
  });

  it("Enter con el foco en el boton NO se cancela: ahi se guarda", async () => {
    simularServidor();
    await abrirElEditorConPadron();

    // El control que PM-3 exige: el arreglo no puede desactivar el camino
    // legitimo. El boton no es un `input` de texto, asi que el handler lo deja
    // pasar -y en un navegador real eso es el envio que se quiere-.
    const evento = createEvent.keyDown(botonGuardar(), { key: "Enter" });
    fireEvent(botonGuardar(), evento);

    expect(evento.defaultPrevented).toBe(false);
    // Y el clic sigue guardando: es el camino que nadie pidio cambiar. El
    // `fetch` de doble queda registrado de forma sincrona, asi que el `PUT` ya
    // esta contado aqui -mismo criterio que el resto de esta suite-.
    guardar();
    expect(guardados()).toHaveLength(1);
  });

  it("un clic de paginacion del padron pide de inmediato, sin esperar al debounce", async () => {
    const llena = Array.from({ length: 50 }, (_, i) => ({
      ...PADRON[0],
      id: `tit-p${i}`,
    }));
    simularServidor({ titulares: () => json(llena) });
    await abrirElEditorConPadron();

    const delPadron = () =>
      peticiones().filter((p) => p.url.startsWith("/api/titulares"));

    // El reloj lo lleva el test y NUNCA avanza: si la peticion sale, sale sin
    // esperar.
    vi.useFakeTimers();
    try {
      const antes = delPadron().length;
      fireEvent.click(screen.getByRole("button", { name: "Siguiente" }));

      expect(delPadron().length).toBe(antes + 1);
      expect(delPadron().at(-1)?.url).toBe(
        "/api/titulares?limite=50&desplazamiento=50",
      );
    } finally {
      vi.useRealTimers();
    }
  });

  // Item 11 en el otro buscador de la pantalla: el padron del editor. Es un
  // consumidor distinto de `useApi` -y de la misma constante compartida-, asi
  // que sin esta prueba quitarle el `debounceMs` a este sitio no lo notaria
  // nadie.
  describe("tecleo en la busqueda del padron (item 11)", () => {
    afterEach(() => {
      cleanup();
      vi.useRealTimers();
    });

    it("teclear en la busqueda no pide una vez por tecla: agrupa la palabra", async () => {
      simularServidor();
      await abrirElEditorConPadron();

      const delPadron = () =>
        peticiones().filter((p) => p.url.startsWith("/api/titulares"));

      // Precondicion: el padron ya se pidio al montar. Un contador que nunca
      // sube no distingue "agrupa el tecleo" de "no pide nada", y las dos cosas
      // no son lo mismo.
      expect(delPadron().length).toBeGreaterThanOrEqual(1);

      // El reloj lo lleva el test: con temporizadores de verdad, "todavia esta
      // tecleando" y "ya pidio" no se separan sin dormir la prueba.
      vi.useFakeTimers();
      const antes = delPadron().length;

      for (const texto of ["A", "An", "Ana", "Ana ", "Ana E", "Ana Es"]) {
        fireEvent.change(screen.getByLabelText("Buscar por nombre"), {
          target: { value: texto },
        });
      }

      // Nada ha salido mientras se teclea...
      expect(delPadron().length).toBe(antes);

      await act(async () => {
        await vi.advanceTimersByTimeAsync(DEBOUNCE_TECLEO_MS);
      });

      // ...y al parar sale UNA, la de la palabra entera.
      expect(delPadron().length).toBe(antes + 1);
      expect(delPadron().at(-1)?.url).toBe(
        "/api/titulares?nombre=Ana+Es&limite=50",
      );
    });
  });
});
