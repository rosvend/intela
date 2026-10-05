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
  trazo,
  type GameState,
} from "./snake";
import { puntoEn } from "./trazoLogo";

const ANCHO = 720;
const ALTO = 900;

function correr(segundos: number, hz = 60): GameState {
  let e = estadoInicial(ANCHO, ALTO);
  const dt = 1 / hz;
  for (let i = 0; i < segundos * hz; i++) e = avanzar(e, dt);
  return e;
}

describe("la escena", () => {
  it("trae varias serpientes repartidas por la misma ruta", () => {
    const e = estadoInicial(ANCHO, ALTO);
    expect(e.serpientes.length).toBeGreaterThan(1);
    // Desfases y velocidades distintas: dos trazos sincronizados se leen
    // como un error de repeticion.
    const fases = new Set(e.serpientes.map((s) => s.trayectoria.fase));
    const velocidades = new Set(
      e.serpientes.map((s) => s.trayectoria.velocidad),
    );
    expect(fases.size).toBe(e.serpientes.length);
    expect(velocidades.size).toBe(e.serpientes.length);
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

describe("las trayectorias siguen la marca", () => {
  it("no dependen de nada mas que del instante", () => {
    for (const def of DEFINICIONES) {
      const s = dimensionar(def, ANCHO, ALTO);
      expect(posicion(s.trayectoria, 3.5)).toEqual(
        posicion(s.trayectoria, 3.5),
      );
    }
  });

  it("dos corridas de la misma duracion acaban igual", () => {
    const a = correr(4);
    const b = correr(4);
    expect(a.serpientes.map((s) => s.t)).toEqual(b.serpientes.map((s) => s.t));
    expect(a.serpientes[0].rastro[0]).toEqual(b.serpientes[0].rastro[0]);
  });

  it("la cabeza esta sobre la ruta del logo, escalada al panel", () => {
    const s = dimensionar(DEFINICIONES[0], ANCHO, ALTO);
    const tr = s.trayectoria;
    for (const t of [0, 1.7, 9.3, 40]) {
      const p = posicion(tr, t);
      const q = puntoEn(tr.fase + tr.velocidad * t);
      expect(p.x).toBeCloseTo(tr.x0 + q.x * tr.lado, 6);
      expect(p.y).toBeCloseTo(tr.y0 + q.y * tr.lado, 6);
    }
  });

  it("la marca va grande, cuadrada y centrada en el panel", () => {
    const tr = dimensionar(DEFINICIONES[0], ANCHO, ALTO).trayectoria;
    expect(tr.lado).toBeGreaterThan(Math.min(ANCHO, ALTO) * 0.7);
    expect(tr.x0 + tr.lado / 2).toBeCloseTo(ANCHO / 2, 6);
    expect(tr.y0 + tr.lado / 2).toBeCloseTo(ALTO / 2, 6);
  });

  it("se quedan dentro del panel", () => {
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

  it("una vuelta completa vuelve al punto de partida", () => {
    const tr = dimensionar(DEFINICIONES[0], ANCHO, ALTO).trayectoria;
    const a = posicion(tr, 0);
    const b = posicion(tr, 1 / tr.velocidad);
    expect(b.x).toBeCloseTo(a.x, 6);
    expect(b.y).toBeCloseTo(a.y, 6);
  });

  it("el rumbo apunta hacia donde se mueve la cabeza", () => {
    const tr = dimensionar(DEFINICIONES[0], ANCHO, ALTO).trayectoria;
    for (const t of [2.2, 7.9, 13.1]) {
      const ahora = posicion(tr, t);
      const luego = posicion(tr, t + 0.05);
      const observado = Math.atan2(luego.y - ahora.y, luego.x - ahora.x);
      const calculado = rumbo(tr, t);
      const d = Math.abs(
        Math.atan2(
          Math.sin(observado - calculado),
          Math.cos(observado - calculado),
        ),
      );
      expect(d).toBeLessThan(0.15);
    }
  });

  it("el trazo de guia es la ruta entera dentro del panel", () => {
    const tr = dimensionar(DEFINICIONES[0], ANCHO, ALTO).trayectoria;
    const puntos = trazo(tr);
    expect(puntos.length).toBeGreaterThan(100);
    for (const p of puntos) {
      expect(p.x).toBeGreaterThanOrEqual(tr.x0 - 1e-9);
      expect(p.x).toBeLessThanOrEqual(tr.x0 + tr.lado + 1e-9);
    }
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

  it("la cabeza nunca se sale del panel", () => {
    // Se veia cortada por abajo en la captura del movil.
    for (const [ancho, alto] of [
      [720, 900],
      [390, 320],
      [200, 140],
    ]) {
      for (const def of DEFINICIONES) {
        const s = dimensionar(def, ancho, alto);
        for (let t = 0; t < 40; t += 0.1) {
          const p = posicion(s.trayectoria, t);
          expect(p.x - s.cabeza / 2).toBeGreaterThanOrEqual(0);
          expect(p.x + s.cabeza / 2).toBeLessThanOrEqual(ancho);
          expect(p.y - s.cabeza / 2).toBeGreaterThanOrEqual(0);
          expect(p.y + s.cabeza / 2).toBeLessThanOrEqual(alto);
        }
      }
    }
  });

  it("nunca deja un lado negativo, ni en un panel diminuto", () => {
    const s = dimensionar(DEFINICIONES[0], 30, 20);
    expect(s.trayectoria.lado).toBeGreaterThan(0);
    expect(s.nodos).toBeGreaterThan(0);
    expect(s.grosor).toBeGreaterThan(0);
  });
});
