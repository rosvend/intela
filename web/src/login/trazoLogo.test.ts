import { describe, expect, it } from "vitest";
import { MUESTRAS, TABLA, puntoEn } from "./trazoLogo";

const dist = (a: { x: number; y: number }, b: { x: number; y: number }) =>
  Math.hypot(a.x - b.x, a.y - b.y);

describe("trazoLogo", () => {
  it("tiene N muestras", () => {
    expect(TABLA).toHaveLength(MUESTRAS);
  });

  it("es un lazo cerrado: el ultimo punto queda junto al primero", () => {
    const paso = dist(TABLA[0], TABLA[1]);
    expect(dist(TABLA[TABLA.length - 1], TABLA[0])).toBeLessThan(paso * 1.5);
  });

  it("reparte las muestras por arco, casi equidistantes", () => {
    const pasos = TABLA.map((p, i) => dist(p, TABLA[(i + 1) % TABLA.length]));
    const media = pasos.reduce((a, b) => a + b, 0) / pasos.length;
    for (const d of pasos) {
      expect(Math.abs(d - media)).toBeLessThan(media * 0.05);
    }
  });

  it("vive dentro de la caja [0,1]", () => {
    for (const p of TABLA) {
      expect(p.x).toBeGreaterThanOrEqual(0);
      expect(p.x).toBeLessThanOrEqual(1);
      expect(p.y).toBeGreaterThanOrEqual(0);
      expect(p.y).toBeLessThanOrEqual(1);
    }
  });

  it("ocupa la caja y queda centrado", () => {
    const xs = TABLA.map((p) => p.x);
    const ys = TABLA.map((p) => p.y);
    const ancho = Math.max(...xs) - Math.min(...xs);
    const alto = Math.max(...ys) - Math.min(...ys);
    expect(Math.max(ancho, alto)).toBeGreaterThan(0.95);
    expect((Math.max(...xs) + Math.min(...xs)) / 2).toBeCloseTo(0.5, 2);
    expect((Math.max(...ys) + Math.min(...ys)) / 2).toBeCloseTo(0.5, 2);
  });

  it("se cruza consigo mismo, como el nudo de la marca", () => {
    // Un aro simple no se cruza; el entrelazado si, y varias veces.
    let cruces = 0;
    const n = 200;
    const p = Array.from({ length: n }, (_, i) => puntoEn(i / n));
    const corta = (
      a: (typeof p)[0],
      b: (typeof p)[0],
      c: (typeof p)[0],
      d: (typeof p)[0],
    ) => {
      const o = (u: typeof a, v: typeof a, w: typeof a) =>
        Math.sign((v.x - u.x) * (w.y - u.y) - (v.y - u.y) * (w.x - u.x));
      return o(a, b, c) !== o(a, b, d) && o(c, d, a) !== o(c, d, b);
    };
    for (let i = 0; i < n; i++) {
      for (let j = i + 2; j < n; j++) {
        if (i === 0 && j === n - 1) continue;
        if (corta(p[i], p[(i + 1) % n], p[j], p[(j + 1) % n])) cruces++;
      }
    }
    expect(cruces).toBeGreaterThanOrEqual(6);
  });

  it("puntoEn envuelve: s y s+1 dan el mismo punto", () => {
    expect(puntoEn(0.3).x).toBeCloseTo(puntoEn(1.3).x, 9);
    expect(puntoEn(-0.25).y).toBeCloseTo(puntoEn(0.75).y, 9);
  });

  it("puntoEn interpola entre muestras", () => {
    const a = puntoEn(0);
    const b = puntoEn(1 / MUESTRAS);
    const m = puntoEn(0.5 / MUESTRAS);
    expect(m.x).toBeCloseTo((a.x + b.x) / 2, 9);
    expect(m.y).toBeCloseTo((a.y + b.y) / 2, 9);
  });
});
