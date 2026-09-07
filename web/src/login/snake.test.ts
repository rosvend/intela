import { describe, expect, it } from "vitest";
import {
  AJUSTES,
  avanzar,
  comidaNueva,
  diferenciaAngular,
  estadoInicial,
  nodosDelCuerpo,
  segmentos,
  type Entrada,
  type GameState,
} from "./snake";

/** Azar fijo: la comida siempre cae en el mismo sitio. */
const fijo = (v: number) => () => v;

/** Estado sin comida, para probar el movimiento sin que crezca por el camino. */
function sinComida(ancho = 400, alto = 400): GameState {
  return { ...estadoInicial(ancho, alto, fijo(0.5)), comida: [] };
}

describe("diferenciaAngular", () => {
  it("toma el camino corto por encima de PI", () => {
    // De 170 a -170 grados son 20 grados por el lado corto, no 340 por el largo.
    const d = diferenciaAngular((170 * Math.PI) / 180, (-170 * Math.PI) / 180);
    expect(d).toBeGreaterThan(0);
    expect(Math.abs(d)).toBeLessThan(Math.PI / 4);
  });

  it("es cero contra si mismo", () => {
    expect(diferenciaAngular(1.2, 1.2)).toBeCloseTo(0);
  });
});

describe("avanzar", () => {
  it("mueve la cabeza segun el rumbo y el tiempo", () => {
    const e = sinComida();
    const d = avanzar(e, 0.5, { tipo: "ninguna" }, fijo(0.5));
    // angulo 0 apunta a la derecha, asi que solo cambia x.
    expect(d.cabeza.x).toBeCloseTo(e.cabeza.x + AJUSTES.velocidad * 0.5);
    expect(d.cabeza.y).toBeCloseTo(e.cabeza.y);
  });

  it("gira hacia el puntero, pero no mas de lo que permite el giro maximo", () => {
    const e = sinComida();
    // Objetivo justo detras: pide media vuelta, que es mas de lo que cabe en
    // un frame. Ahi esta la suavidad de la curva.
    const entrada: Entrada = {
      tipo: "puntero",
      objetivo: { x: e.cabeza.x - 100, y: e.cabeza.y },
    };
    const dt = 0.1;
    const d = avanzar(e, dt, entrada, fijo(0.5));

    expect(Math.abs(d.angulo)).toBeLessThanOrEqual(
      AJUSTES.giroMaximo * dt + 1e-9,
    );
    expect(Math.abs(d.angulo)).toBeGreaterThan(0);
  });

  it("ignora un puntero encima de la cabeza en vez de tirar a la derecha", () => {
    // atan2(0,0) devuelve 0: sin la guarda, poner el raton sobre la cabeza
    // enderezaria la serpiente hacia la derecha de golpe.
    const e = { ...sinComida(), angulo: Math.PI / 2 };
    const d = avanzar(
      e,
      0.1,
      { tipo: "puntero", objetivo: { ...e.cabeza } },
      fijo(0.5),
    );
    expect(d.angulo).toBeCloseTo(Math.PI / 2);
  });

  it("cruza los bordes en vez de detenerse", () => {
    const e: GameState = { ...sinComida(200, 200), cabeza: { x: 195, y: 100 } };
    const d = avanzar(e, 0.2, { tipo: "ninguna" }, fijo(0.5));
    expect(d.cabeza.x).toBeGreaterThanOrEqual(0);
    expect(d.cabeza.x).toBeLessThan(200);
    // Y reaparece por la izquierda, no se queda pegado al borde derecho.
    expect(d.cabeza.x).toBeLessThan(195);
  });

  it("corta el camino al cruzar, para no dibujar una raya de lado a lado", () => {
    const e: GameState = {
      ...sinComida(200, 200),
      cabeza: { x: 195, y: 100 },
      camino: [
        { x: 195, y: 100 },
        { x: 180, y: 100 },
        { x: 165, y: 100 },
      ],
    };
    const d = avanzar(e, 0.2, { tipo: "ninguna" }, fijo(0.5));
    expect(d.camino).toHaveLength(1);
  });

  it("suma un punto y crece al comer", () => {
    const e = sinComida();
    const conComida: GameState = {
      ...e,
      comida: [
        { x: e.cabeza.x + 4, y: e.cabeza.y, radio: AJUSTES.radioComida },
      ],
    };
    const antes = nodosDelCuerpo(conComida.puntos);

    const d = avanzar(conComida, 0.016, { tipo: "ninguna" }, fijo(0.5));

    expect(d.puntos).toBe(1);
    expect(nodosDelCuerpo(d.puntos)).toBeGreaterThan(antes);
    // La comida no desaparece: se repone, para que el lienzo no se vacie.
    expect(d.comida).toHaveLength(1);
  });

  it("no crece sin limite", () => {
    expect(nodosDelCuerpo(10_000)).toBe(AJUSTES.nodosMaximos);
  });

  it("no deja crecer el camino sin limite", () => {
    // La fuga lenta: la pantalla de acceso se queda abierta, y un punto por
    // frame a 60 Hz son 216 000 puntos por hora si nadie los recorta.
    let e = sinComida();
    for (let i = 0; i < 3000; i++) {
      e = avanzar(e, 1 / 60, { tipo: "ninguna" }, fijo(0.5));
    }
    const tope = nodosDelCuerpo(e.puntos) * AJUSTES.separacion;
    // Con el recorte, el camino guardado cubre el largo del cuerpo y poco mas.
    expect(e.camino.length).toBeLessThan(tope);
  });
});

describe("segmentos", () => {
  it("separa los nodos por distancia recorrida, no por indice del array", () => {
    // Es lo que hace que el cuerpo mida lo mismo a 60 y a 120 Hz.
    const lento = (() => {
      let e = sinComida();
      for (let i = 0; i < 400; i++)
        e = avanzar(e, 1 / 60, { tipo: "ninguna" }, fijo(0.5));
      return segmentos(e);
    })();
    const rapido = (() => {
      let e = sinComida();
      for (let i = 0; i < 800; i++)
        e = avanzar(e, 1 / 120, { tipo: "ninguna" }, fijo(0.5));
      return segmentos(e);
    })();

    expect(lento).toHaveLength(rapido.length);

    const largo = (s: typeof lento) =>
      Math.hypot(s[0].x - s[s.length - 1].x, s[0].y - s[s.length - 1].y);
    // Mismo largo fisico con el doble de frames: la diferencia queda en el
    // ruido de un paso, no en la mitad.
    expect(Math.abs(largo(lento) - largo(rapido))).toBeLessThan(
      AJUSTES.separacion,
    );
  });

  it("separa los nodos exactamente la distancia configurada", () => {
    // LA prueba que faltaba. La version anterior comparaba 60 Hz contra 120 Hz,
    // y las dos salian igual de mal: la separacion crecia como 1+2+3... porque
    // el acumulado se reiniciaba en cada nodo. Una comparacion relativa no ve
    // un defecto que afecta a los dos lados por igual; hay que fijar el valor.
    let e = sinComida(2000, 2000);
    // En linea recta, para que la distancia entre nodos sea la del arco.
    for (let i = 0; i < 300; i++) {
      e = avanzar(e, 1 / 60, { tipo: "ninguna" }, fijo(0.5));
    }
    const s = segmentos(e);

    for (let i = 1; i < s.length; i++) {
      const d = Math.hypot(s[i].x - s[i - 1].x, s[i].y - s[i - 1].y);
      expect(d).toBeCloseTo(AJUSTES.separacion, 1);
    }
  });

  it("mantiene el cuerpo mas corto que el camino guardado", () => {
    // Con la separacion desbocada el cuerpo se salia del camino y los ultimos
    // nodos se apilaban en la cola: el sintoma visible era un cuerpo corto con
    // un amasijo al final.
    let e = sinComida(2000, 2000);
    for (let i = 0; i < 300; i++) {
      e = avanzar(e, 1 / 60, { tipo: "ninguna" }, fijo(0.5));
    }
    const s = segmentos(e);
    const largoCuerpo = Math.hypot(
      s[0].x - s[s.length - 1].x,
      s[0].y - s[s.length - 1].y,
    );
    expect(largoCuerpo).toBeCloseTo(
      (nodosDelCuerpo(e.puntos) - 1) * AJUSTES.separacion,
      0,
    );
  });

  it("se estrecha de la cabeza a la cola", () => {
    let e = sinComida();
    for (let i = 0; i < 200; i++)
      e = avanzar(e, 1 / 60, { tipo: "ninguna" }, fijo(0.5));
    const s = segmentos(e);

    expect(s[0].radio).toBeCloseTo(AJUSTES.radioCabeza);
    expect(s[s.length - 1].radio).toBeCloseTo(AJUSTES.radioCola);
    for (let i = 1; i < s.length; i++) {
      expect(s[i].radio).toBeLessThanOrEqual(s[i - 1].radio + 1e-9);
    }
  });

  it("no revienta con un camino de un solo punto", () => {
    // Es el primer frame, antes de que la cabeza haya dejado rastro.
    const s = segmentos(sinComida());
    expect(s).toHaveLength(AJUSTES.nodosBase);
    for (const nodo of s) {
      expect(Number.isFinite(nodo.x)).toBe(true);
      expect(Number.isFinite(nodo.y)).toBe(true);
    }
  });
});

describe("comidaNueva", () => {
  it("deja margen con el borde", () => {
    const c = comidaNueva(300, 300, fijo(0));
    expect(c.x).toBeGreaterThanOrEqual(AJUSTES.margenComida);
    expect(c.y).toBeGreaterThanOrEqual(AJUSTES.margenComida);
  });

  it("no sale del lienzo con el azar al maximo", () => {
    const c = comidaNueva(300, 300, fijo(1));
    expect(c.x).toBeLessThanOrEqual(300 - AJUSTES.margenComida);
    expect(c.y).toBeLessThanOrEqual(300 - AJUSTES.margenComida);
  });

  it("aguanta un lienzo mas estrecho que los margenes", () => {
    // El panel colapsado en un movil muy pequeno: no debe dar NaN.
    const c = comidaNueva(20, 20, fijo(0.5));
    expect(Number.isFinite(c.x)).toBe(true);
    expect(Number.isFinite(c.y)).toBe(true);
  });
});
