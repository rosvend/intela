import { expect, test } from "@playwright/test";

/*
 * #224: el riel de iconos a 720px o menos cambia el shell de todos los roles,
 * no solo el de Distribucion. El titular es quien mas entra desde el telefono:
 * sus dos pantallas no pueden scrollear de lado.
 */

const LIQUIDACIONES = {
  titular_id: "tit-ana",
  periodo: "",
  lineas: [
    {
      periodo: "2025-01",
      obra_id: "obra-serie",
      titulo: "La Casa de las Dos Palmas",
      bruto: "2000000.00",
      admin: "0",
      social: "0",
      reserva: "0",
      neto: "1950000.00",
    },
    {
      periodo: "2025-01",
      obra_id: "obra-cine",
      titulo: "El Último Plano",
      bruto: "800000.00",
      admin: "0",
      social: "0",
      reserva: "0",
      neto: "500000.00",
    },
  ],
  totales: {
    bruto: "2800000.00",
    admin: "0",
    social: "0",
    reserva: "0",
    neto: "2450000.00",
  },
};

const RESPUESTAS: Record<string, unknown> = {
  "/api/auth/session": {
    id: "usr-ana",
    email: "ana@redes.co",
    nombre: "Ana Escritora",
    rol: "titular",
    titular_id: "tit-ana",
  },
  "/api/tablero/ultima-liquidacion": {
    periodo: "2025-01",
    neto: "2450000.00",
    obras: 2,
  },
  "/api/tablero/mis-obras": {
    obras: [
      {
        id: "obra-serie",
        titulo: "La Casa de las Dos Palmas",
        estado: "completa",
      },
      {
        id: "obra-sketch",
        titulo: "Sketch de Medianoche",
        estado: "incompleta",
      },
    ],
  },
  "/api/mis-liquidaciones/obras": LIQUIDACIONES,
  "/api/mis-ingresos": { ingresos: [] },
};

const PANTALLAS = [
  { ruta: "/", lista: "La Casa de las Dos Palmas" },
  { ruta: "/mis-liquidaciones", lista: "El Último Plano" },
];

for (const ancho of [320, 360]) {
  for (const { ruta, lista } of PANTALLAS) {
    test(`el titular en ${ruta} a ${ancho}px no scrollea de lado`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: ancho, height: 900 });
      await page.addInitScript(() =>
        localStorage.setItem("intela.token", "token-de-prueba"),
      );
      await page.route("**/api/**", (route) => {
        const cuerpo = RESPUESTAS[new URL(route.request().url()).pathname];
        return route.fulfill({
          status: cuerpo === undefined ? 404 : 200,
          contentType: "application/json",
          body: JSON.stringify(cuerpo ?? { error: "no simulado" }),
        });
      });
      await page.goto(ruta);
      await expect(page.getByText(lista).first()).toBeVisible();
      await page.evaluate(() => document.fonts.ready);

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
  }
}
