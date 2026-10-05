import { describe, expect, it } from "vitest";
import { MUESTRAS, RUTAS, puntoEn, type Punto } from "./trazoLogo";

const dist = (a: Punto, b: Punto) => Math.hypot(a.x - b.x, a.y - b.y);

/** Giro entre muestras consecutivas, en radianes, con signo. */
function giros(ruta: readonly Punto[]): number[] {
  const n = ruta.length;
  return ruta.map((p, i) => {
    const a = ruta[(i - 1 + n) % n];
    const b = ruta[(i + 1) % n];
    const h1 = Math.atan2(p.y - a.y, p.x - a.x);
    const h2 = Math.atan2(b.y - p.y, b.x - p.x);
    return Math.atan2(Math.sin(h2 - h1), Math.cos(h2 - h1));
  });
}

const centroide = (ruta: readonly Punto[]): Punto => ({
  x: ruta.reduce((s, p) => s + p.x, 0) / ruta.length,
  y: ruta.reduce((s, p) => s + p.y, 0) / ruta.length,
});

describe("trazoLogo", () => {
  it("trae una ruta por hebra de la marca: cuatro", () => {
    expect(RUTAS).toHaveLength(4);
  });

  describe.each(RUTAS.map((r, i) => [i, r] as const))("la ruta %i", (_, r) => {
    it("tiene N muestras", () => {
      expect(r).toHaveLength(MUESTRAS);
    });

    it("es un lazo cerrado: el ultimo punto queda junto al primero", () => {
      const paso = dist(r[0], r[1]);
      expect(dist(r[r.length - 1], r[0])).toBeLessThan(paso * 1.5);
    });

    it("reparte las muestras por arco, casi equidistantes", () => {
      const pasos = r.map((p, i) => dist(p, r[(i + 1) % r.length]));
      const media = pasos.reduce((a, b) => a + b, 0) / pasos.length;
      for (const d of pasos) {
        expect(Math.abs(d - media)).toBeLessThan(media * 0.02);
      }
    });

    it("es suave: ningun pico de curvatura", () => {
      // Lo que se veia en la ruta calcada a mano: codos donde el giro de un
      // paso se disparaba. En un lazo convexo y suave el giro es siempre del
      // mismo signo, no lejos de la media, y cambia poco de un paso al otro.
      const g = giros(r);
      const media = (2 * Math.PI) / g.length;
      for (const x of g) {
        expect(x).toBeGreaterThan(0);
        expect(x).toBeLessThan(media * 2.5);
      }
      for (let i = 0; i < g.length; i++) {
        expect(Math.abs(g[i] - g[(i + 1) % g.length])).toBeLessThan(
          media * 0.1,
        );
      }
    });

    it("vive dentro de la caja [0,1]", () => {
      for (const p of r) {
        expect(p.x).toBeGreaterThanOrEqual(0);
        expect(p.x).toBeLessThanOrEqual(1);
        expect(p.y).toBeGreaterThanOrEqual(0);
        expect(p.y).toBeLessThanOrEqual(1);
      }
    });
  });

  it("las rutas son distintas: cada una tiene su centro", () => {
    const c = RUTAS.map(centroide);
    for (let i = 0; i < c.length; i++) {
      for (let j = i + 1; j < c.length; j++) {
        expect(dist(c[i], c[j])).toBeGreaterThan(0.1);
      }
    }
  });

  it("juntas ocupan la caja y quedan centradas: la marca armada", () => {
    const todos = RUTAS.flat();
    const xs = todos.map((p) => p.x);
    const ys = todos.map((p) => p.y);
    expect(Math.max(...xs) - Math.min(...xs)).toBeGreaterThan(0.98);
    expect(Math.max(...ys) - Math.min(...ys)).toBeGreaterThan(0.98);
    expect((Math.max(...xs) + Math.min(...xs)) / 2).toBeCloseTo(0.5, 2);
    expect((Math.max(...ys) + Math.min(...ys)) / 2).toBeCloseTo(0.5, 2);
  });

  it("la marca tiene simetria de cuarto de vuelta: cada hebra es la anterior girada", () => {
    for (let k = 1; k < RUTAS.length; k++) {
      const girada = RUTAS[k - 1].map((p) => ({ x: 1 - p.y, y: p.x }));
      for (const p of RUTAS[k]) {
        const cerca = Math.min(...girada.map((q) => dist(p, q)));
        expect(cerca).toBeLessThan(0.01);
      }
    }
  });

  it("puntoEn envuelve: s y s+1 dan el mismo punto", () => {
    for (let r = 0; r < RUTAS.length; r++) {
      expect(puntoEn(r, 0.3).x).toBeCloseTo(puntoEn(r, 1.3).x, 9);
      expect(puntoEn(r, -0.25).y).toBeCloseTo(puntoEn(r, 0.75).y, 9);
    }
  });

  it("puntoEn interpola entre muestras de la ruta pedida", () => {
    const a = puntoEn(2, 0);
    const b = puntoEn(2, 1 / MUESTRAS);
    const m = puntoEn(2, 0.5 / MUESTRAS);
    expect(a).toEqual(RUTAS[2][0]);
    expect(m.x).toBeCloseTo((a.x + b.x) / 2, 9);
    expect(m.y).toBeCloseTo((a.y + b.y) / 2, 9);
  });
});
