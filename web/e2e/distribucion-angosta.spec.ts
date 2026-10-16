import { expect, test, type Page } from "@playwright/test";

/*
 * #224: a anchos de telefono la Distribucion partia las cifras entre digitos
 * (`overflow-wrap: anywhere` pintaba "$ 1.1" / "11." / "111" ...), la lista de
 * corridas se quedaba sin pagador (su columna `minmax(0, 1fr)` en 0 frente al
 * monto) y la pagina scrolleaba de lado con la barra de 240px sin plegar. Todo
 * es layout, que jsdom no calcula: aqui se mide lo que el navegador pinta.
 */

// El escenario del issue: 3 corridas del periodo 2026-02 sobre 2 bolsas. Dos
// corridas sobre la de Caracol y una sobre la de RCN; total 1.111.111.110.111,10.
const PROCESOS = [
  {
    id: "proc-b1-1",
    circuito: "nacional",
    etapa: "verificacion",
    periodo: "2026-02",
    bolsa_id: "b1",
    snapshot_id: "s1",
    revision: 1,
    firmas: [],
  },
  {
    id: "proc-b1-2",
    circuito: "nacional",
    etapa: "recaudo",
    periodo: "2026-02",
    bolsa_id: "b1",
    snapshot_id: "s2",
    revision: 1,
    firmas: [],
  },
  {
    id: "proc-b2-1",
    circuito: "nacional",
    etapa: "liquidacion_final",
    periodo: "2026-02",
    bolsa_id: "b2",
    snapshot_id: "s3",
    revision: 1,
    firmas: [],
  },
];

const BOLSAS = [
  {
    id: "b1",
    usuario_id: "caracol",
    periodo: "2026-02",
    circuito: "nacional",
    bruto: "123456789012.34",
  },
  {
    id: "b2",
    usuario_id: "rcn",
    periodo: "2026-02",
    circuito: "nacional",
    bruto: "987654321098.76",
  },
];

const RESPUESTAS: Record<string, unknown> = {
  "/api/auth/session": {
    id: "usr-1",
    email: "admin@redes.co",
    nombre: "Persona de Prueba",
    rol: "administrador",
    titular_id: "",
  },
  "/api/procesos": PROCESOS,
  "/api/bolsas": BOLSAS,
};

async function abrirDistribucion(page: Page) {
  await page.addInitScript(() =>
    localStorage.setItem("intela.token", "token-de-prueba"),
  );
  // Lo que no esta en RESPUESTAS (alertas, conteo de identificacion) contesta
  // 404: la pantalla lo pinta como "sin datos" y no cambia lo que se mide.
  await page.route("**/api/**", (route) => {
    const cuerpo = RESPUESTAS[new URL(route.request().url()).pathname];
    return route.fulfill({
      status: cuerpo === undefined ? 404 : 200,
      contentType: "application/json",
      body: JSON.stringify(cuerpo ?? { error: "no simulado" }),
    });
  });
  await page.goto("/distribucion/proc-b1-1");
  await expect(page.locator(".corridas-lista .corrida-item")).toHaveCount(3);
  await expect(page.getByLabel("$ 1.111.111.110.111,10")).toBeAttached();
  await page.evaluate(() => document.fonts.ready);
}

type CifraPintada = {
  clase: string;
  lineas: string[];
  desborda: boolean;
};

/**
 * Cada importe en pesos que se ve en la pantalla, con el texto de cada linea
 * en que quedo pintado. La linea sale de la posicion de cada caracter
 * (`Range.getClientRects`), no del DOM: es justo lo que jsdom no sabe.
 */
function cifrasPintadas(page: Page): Promise<CifraPintada[]> {
  return page.evaluate(() => {
    const SELECTOR = ".cifra-valor, .corrida-monto, .corrida-cifras-valor";
    const cifras: CifraPintada[] = [];
    for (const el of document.querySelectorAll<HTMLElement>(SELECTOR)) {
      const caracteres: { c: string; top: number }[] = [];
      const recorrido = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
      for (let n = recorrido.nextNode(); n; n = recorrido.nextNode()) {
        // El importe exacto para lectores de pantalla no se pinta.
        if (n.parentElement?.closest(".solo-lector")) continue;
        const texto = n.textContent ?? "";
        for (let i = 0; i < texto.length; i++) {
          const rango = document.createRange();
          rango.setStart(n, i);
          rango.setEnd(n, i + 1);
          const caja = [...rango.getClientRects()].find((r) => r.width > 0);
          if (caja) caracteres.push({ c: texto[i], top: caja.top });
        }
      }
      const lineas: { top: number; texto: string }[] = [];
      for (const { c, top } of caracteres) {
        const linea = lineas.find((l) => Math.abs(l.top - top) < 4);
        if (linea) linea.texto += c;
        else lineas.push({ top, texto: c });
      }
      const pintadas = lineas
        .sort((a, b) => a.top - b.top)
        .map((l) => l.texto.trim());
      if (!pintadas.join("").includes("$")) continue;
      cifras.push({
        clase: el.className,
        lineas: pintadas,
        desborda: el.scrollWidth > el.clientWidth,
      });
    }
    return cifras;
  });
}

/**
 * Los saltos de linea que caen donde no deben. Uno vale solo si la linea
 * termina en "$" o en el "." de un grupo de miles y la siguiente empieza por
 * digito: nunca dentro de un grupo de tres, ni entre la coma y los centavos,
 * ni separando la unidad ("M", "mil") de su numero.
 */
function saltosIndebidos(lineas: string[]): string[] {
  const malos: string[] = [];
  for (let i = 0; i + 1 < lineas.length; i++) {
    const fin = lineas[i].at(-1) ?? "";
    const inicio = lineas[i + 1][0] ?? "";
    if (!((fin === "." || fin === "$") && /\d/.test(inicio))) {
      malos.push(`${lineas[i]} | ${lineas[i + 1]}`);
    }
  }
  return malos;
}

for (const ancho of [320, 360, 420]) {
  test.describe(`Distribucion a ${ancho}px`, () => {
    test.use({ viewport: { width: ancho, height: 900 } });

    test.beforeEach(async ({ page }) => {
      await abrirDistribucion(page);
    });

    test("ninguna cifra se parte a mitad de un grupo de digitos ni desborda su caja", async ({
      page,
    }) => {
      const cifras = await cifrasPintadas(page);
      // Las tres clases de importe de la pantalla: el total del periodo, el
      // compacto de cada corrida en la lista y la bolsa bruta del detalle.
      expect(cifras.map((c) => c.clase)).toEqual(
        expect.arrayContaining([
          "cifra-valor",
          "corrida-monto",
          "corrida-cifras-valor",
        ]),
      );
      for (const cifra of cifras) {
        expect(saltosIndebidos(cifra.lineas), cifra.lineas.join(" / ")).toEqual(
          [],
        );
        expect(cifra.desborda, cifra.lineas.join(" / ")).toBe(false);
      }
    });

    test("cada corrida de la lista muestra su pagador entero", async ({
      page,
    }) => {
      const nombres = page.locator(".corridas-lista .corrida-nombre");
      await expect(nombres).toHaveCount(3);
      for (const nombre of await nombres.all()) {
        const caja = await nombre.boundingBox();
        expect(caja?.width ?? 0).toBeGreaterThan(0);
        // Ni en 0 ni abreviado con "...": el texto entero cabe en su caja.
        const pagador = nombre.locator("> span").first();
        expect(
          await pagador.evaluate((el) => el.scrollWidth <= el.clientWidth),
        ).toBe(true);
      }
      await expect(
        page.locator(".corridas-lista .corrida-nombre > span:first-child"),
      ).toHaveText(["RCN", "Caracol", "Caracol · corrida 2"]);
    });

    test("la pagina no scrollea de lado", async ({ page }) => {
      const anchos = await page.evaluate(() => {
        const contenido = document.querySelector(".contenido");
        return {
          pagina:
            document.documentElement.scrollWidth -
            document.documentElement.clientWidth,
          contenido: contenido
            ? contenido.scrollWidth - contenido.clientWidth
            : 0,
        };
      });
      expect(anchos).toEqual({ pagina: 0, contenido: 0 });
    });

    test("la burbuja del asistente no tapa el menu de usuario", async ({
      page,
    }) => {
      // Con la barra plegada el boton de perfil es solo el avatar: si algo lo
      // tapa, el click de Playwright falla en vez de llegar a otro elemento.
      await page
        .getByRole("button", { name: "Abrir menú de usuario" })
        .click({ timeout: 5_000 });
      // El menu se abre junto al riel, donde queda la burbuja: cada opcion
      // tiene que recibir el click de punta a punta. `trial` comprueba que el
      // click llegaria sin hacerlo, asi que "Salir" no cierra la sesion.
      const opciones = await page.getByRole("menuitem").all();
      expect(opciones).toHaveLength(2);
      for (const opcion of opciones) {
        const caja = await opcion.boundingBox();
        const { width, height } = caja ?? { width: 0, height: 0 };
        for (const x of [8, width / 2, width - 8]) {
          await opcion.click({
            trial: true,
            position: { x, y: height / 2 },
            timeout: 5_000,
          });
        }
      }
    });
  });
}
