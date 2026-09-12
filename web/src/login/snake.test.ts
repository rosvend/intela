import { describe, expect, it } from "vitest";
import {
  AJUSTES,
  avanzar,
  estadoInicial,
  posicion,
  rumbo,
  segmentos,
  DEFINICIONES,
  dimensionar,
  type GameState,
} from "./snake";

const ANCHO = 720;
const ALTO = 900;

function correr(segundos: number, hz = 60): GameState {
  let e = estadoInicial(ANCHO, ALTO);
  const dt = 1 / hz;
  for (let i = 0; i < segundos * hz; i++) e = avanzar(e, dt);
  return e;
}

describe("la escena", () => {
  it("trae varias serpientes, con figuras distintas", () => {
    const e = estadoInicial(ANCHO, ALTO);
    expect(e.serpientes.length).toBeGreaterThan(1);

    // Figuras y no la misma desfasada: dos trazos iguales en paralelo se leen
    // como un error de repeticion.
    const figuras = new Set(
      e.serpientes.map((s) => `${s.trayectoria.fx}:${s.trayectoria.fy}`),
    );
    expect(figuras.size).toBe(e.serpientes.length);
  });

  it("las diferencia por grosor y opacidad, para dar profundidad", () => {
    const e = estadoInicial(ANCHO, ALTO);
    expect(new Set(e.serpientes.map((s) => s.grosor)).size).toBe(
      e.serpientes.length,
    );
    expect(new Set(e.serpientes.map((s) => s.alfa)).size).toBe(
      e.serpientes.length,
    );
  });
});

describe("las trayectorias son fijas", () => {
  it("no dependen de nada mas que del instante", () => {
    // Es lo que hace que el movimiento sea "un conjunto de movimientos fijos":
    // sin azar y sin entrada, el mismo t da siempre el mismo punto.
    for (const def of DEFINICIONES) {
      const s = dimensionar(def, ANCHO, ALTO);
      const a = posicion(s.trayectoria, 3.5, ANCHO, ALTO);
      const b = posicion(s.trayectoria, 3.5, ANCHO, ALTO);
      expect(a).toEqual(b);
    }
  });

  it("dos corridas de la misma duracion acaban igual", () => {
    const a = correr(4);
    const b = correr(4);
    expect(a.serpientes.map((s) => s.t)).toEqual(b.serpientes.map((s) => s.t));
    expect(a.serpientes[0].rastro[0]).toEqual(b.serpientes[0].rastro[0]);
  });

  it("se quedan dentro del panel", () => {
    // La razon de elegir una curva cerrada y no una lista de giros: los giros
    // derivan y hay que envolver por los bordes, que corta el cuerpo.
    const e = correr(30);
    for (const s of e.serpientes) {
      for (const p of s.rastro) {
        expect(p.x).toBeGreaterThanOrEqual(0);
        expect(p.x).toBeLessThanOrEqual(ANCHO);
        expect(p.y).toBeGreaterThanOrEqual(0);
        expect(p.y).toBeLessThanOrEqual(ALTO);
      }
    }
  });

  it("recorren la figura entera, no un trozo", () => {
    const e = correr(60);
    const s = e.serpientes[0];
    const puntos = Array.from({ length: 400 }, (_, i) =>
      posicion(s.trayectoria, (i / 400) * 60, ANCHO, ALTO),
    );
    const xs = puntos.map((p) => p.x);
    const ys = puntos.map((p) => p.y);
    // Cubre la mayor parte de la amplitud que declara la trayectoria.
    expect(Math.max(...xs) - Math.min(...xs)).toBeGreaterThan(
      ANCHO * s.trayectoria.rx * 1.5,
    );
    expect(Math.max(...ys) - Math.min(...ys)).toBeGreaterThan(
      ALTO * s.trayectoria.ry * 1.5,
    );
  });

  it("el rumbo apunta hacia donde se mueve la cabeza", () => {
    const s = dimensionar(DEFINICIONES[0], ANCHO, ALTO);
    const t = 2.2;
    const ahora = posicion(s.trayectoria, t, ANCHO, ALTO);
    const luego = posicion(s.trayectoria, t + 0.01, ANCHO, ALTO);
    const observado = Math.atan2(luego.y - ahora.y, luego.x - ahora.x);
    const calculado = rumbo(s.trayectoria, t, ANCHO, ALTO);

    const d = Math.abs(
      Math.atan2(
        Math.sin(observado - calculado),
        Math.cos(observado - calculado),
      ),
    );
    expect(d).toBeLessThan(0.05);
  });
});

describe("el cuerpo", () => {
  it("separa los nodos exactamente la distancia configurada", () => {
    // LA prueba que faltaba en la primera version: la que habia comparaba
    // 60 Hz contra 120 Hz y las dos salian igual de mal, porque el acumulado
    // se reiniciaba en cada nodo y la separacion crecia como 1+2+3...
    const e = correr(8);
    for (const s of e.serpientes) {
      const cuerpo = segmentos(s);
      for (let i = 1; i < cuerpo.length; i++) {
        const d = Math.hypot(
          cuerpo[i].x - cuerpo[i - 1].x,
          cuerpo[i].y - cuerpo[i - 1].y,
        );
        expect(d).toBeCloseTo(AJUSTES.separacion, 1);
      }
    }
  });

  it("mide lo mismo a 60 y a 120 Hz", () => {
    const largo = (e: GameState) => {
      const c = segmentos(e.serpientes[0]);
      return Math.hypot(c[0].x - c[c.length - 1].x, c[0].y - c[c.length - 1].y);
    };
    expect(Math.abs(largo(correr(8, 60)) - largo(correr(8, 120)))).toBeLessThan(
      AJUSTES.separacion,
    );
  });

  it("es una LINEA: el grosor es el mismo de punta a punta", () => {
    // Lo pedido explicitamente. La version anterior lo estrechaba hacia el
    // final: le daba silueta de animal, y como cada tramo de grosor distinto
    // hay que trazarlo aparte, los mas finos se veian como puntos sueltos.
    for (const s of correr(8).serpientes) {
      const cuerpo = segmentos(s);
      for (const n of cuerpo) {
        expect(n.radio).toBe(s.grosor);
      }
    }
  });

  it("no revienta con un rastro de un solo punto", () => {
    // El primer frame, antes de que la cabeza haya dejado rastro.
    const e = estadoInicial(ANCHO, ALTO);
    for (const s of e.serpientes) {
      const cuerpo = segmentos(s);
      expect(cuerpo).toHaveLength(s.nodos);
      for (const n of cuerpo) {
        expect(Number.isFinite(n.x)).toBe(true);
        expect(Number.isFinite(n.y)).toBe(true);
      }
    }
  });

  it("no deja crecer el rastro sin limite", () => {
    // La fuga lenta: un punto por frame son 216 000 por hora en una pantalla
    // que la gente deja abierta.
    const e = correr(60);
    for (const s of e.serpientes) {
      expect(s.rastro.length).toBeLessThan(s.nodos * AJUSTES.separacion);
    }
  });
});

describe("dimensionar", () => {
  it("encoge la escena en un panel pequeno", () => {
    const grande = dimensionar(DEFINICIONES[0], 720, 900);
    const chico = dimensionar(DEFINICIONES[0], 390, 320);

    // Con las medidas fijas, en el panel del movil el cuerpo salia tan largo
    // como alto el panel y la cabeza ocupaba un tercio del ancho.
    expect(chico.cabeza).toBeLessThan(grande.cabeza);
    expect(chico.grosor).toBeLessThan(grande.grosor);
    expect(chico.nodos).toBeLessThan(grande.nodos);
  });

  it("recorta la amplitud para que la cabeza no se salga", () => {
    // Se veia cortada por abajo en la captura del movil.
    for (const [ancho, alto] of [
      [720, 900],
      [390, 320],
      [200, 140],
    ]) {
      for (const def of DEFINICIONES) {
        const s = dimensionar(def, ancho, alto);
        for (let t = 0; t < 40; t += 0.1) {
          const p = posicion(s.trayectoria, t, ancho, alto);
          expect(p.x - s.cabeza / 2).toBeGreaterThanOrEqual(0);
          expect(p.x + s.cabeza / 2).toBeLessThanOrEqual(ancho);
          expect(p.y - s.cabeza / 2).toBeGreaterThanOrEqual(0);
          expect(p.y + s.cabeza / 2).toBeLessThanOrEqual(alto);
        }
      }
    }
  });

  it("nunca deja una amplitud negativa, ni en un panel diminuto", () => {
    const s = dimensionar(DEFINICIONES[0], 30, 20);
    expect(s.trayectoria.rx).toBeGreaterThan(0);
    expect(s.trayectoria.ry).toBeGreaterThan(0);
    expect(s.nodos).toBeGreaterThan(0);
    expect(s.grosor).toBeGreaterThan(0);
  });
});
