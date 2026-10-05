/**
 * La ruta de la marca de Intela: un lazo cerrado que recorre el nudo entero.
 *
 * Calcado a mano de `docs/intela-logo.png`. El nudo tiene simetria central, asi
 * que solo se escribe media vuelta y la otra mitad es su giro de 180 grados.
 * Media vuelta: aro superior izquierdo, barra inferior del centro, arco grande
 * de arriba a la derecha y barra izquierda del centro hasta abajo.
 */

export interface Punto {
  x: number;
  y: number;
}

/** Muestras de la tabla. */
export const MUESTRAS = 480;

// Media vuelta en pixeles del PNG (marca ~330x315, centro en 195,172).
const MEDIA_VUELTA: readonly [number, number][] = [
  [185, 25],
  [145, 21],
  [105, 27.5],
  [70, 47.5],
  [50, 80],
  [49, 112.5],
  [62.5, 150],
  [90, 180],
  [125, 210],
  [160, 226],
  [195, 233],
  [230, 231],
  [260, 224],
  [285, 211],
  [310, 194],
  [332.5, 172.5],
  [342.5, 140],
  [342.5, 105],
  [332.5, 72.5],
  [310, 45],
  [277.5, 27.5],
  [242.5, 24],
  [207.5, 32.5],
  [176, 52.5],
  [152.5, 82.5],
  [137.5, 122.5],
  [132.5, 165],
  [139, 210],
  [155, 250],
  [180, 287.5],
];

const CENTRO = { x: 195, y: 172 };

function controles(): Punto[] {
  const a = MEDIA_VUELTA.map(([x, y]) => ({ x, y }));
  const b = a.map((p) => ({ x: 2 * CENTRO.x - p.x, y: 2 * CENTRO.y - p.y }));
  return [...a, ...b];
}

/** Catmull-Rom uniforme y cerrada: pasa por cada control sin esquinas. */
function curva(c: Punto[], porTramo: number): Punto[] {
  const n = c.length;
  const salida: Punto[] = [];
  for (let i = 0; i < n; i++) {
    const p0 = c[(i - 1 + n) % n];
    const p1 = c[i];
    const p2 = c[(i + 1) % n];
    const p3 = c[(i + 2) % n];
    for (let k = 0; k < porTramo; k++) {
      const t = k / porTramo;
      const t2 = t * t;
      const t3 = t2 * t;
      const f = (a: number, b: number, cc: number, d: number) =>
        0.5 *
        (2 * b +
          (-a + cc) * t +
          (2 * a - 5 * b + 4 * cc - d) * t2 +
          (-a + 3 * b - 3 * cc + d) * t3);
      salida.push({
        x: f(p0.x, p1.x, p2.x, p3.x),
        y: f(p0.y, p1.y, p2.y, p3.y),
      });
    }
  }
  return salida;
}

/** Remuestrea por arco: `n` puntos a la misma distancia sobre el lazo. */
function porArco(densa: Punto[], n: number): Punto[] {
  const lazo = [...densa, densa[0]];
  const acum = [0];
  for (let i = 1; i < lazo.length; i++) {
    acum.push(
      acum[i - 1] +
        Math.hypot(lazo[i].x - lazo[i - 1].x, lazo[i].y - lazo[i - 1].y),
    );
  }
  const total = acum[acum.length - 1];
  const salida: Punto[] = [];
  let j = 1;
  for (let k = 0; k < n; k++) {
    const objetivo = (k / n) * total;
    while (acum[j] < objetivo) j++;
    const tramo = acum[j] - acum[j - 1];
    const t = tramo === 0 ? 0 : (objetivo - acum[j - 1]) / tramo;
    salida.push({
      x: lazo[j - 1].x + (lazo[j].x - lazo[j - 1].x) * t,
      y: lazo[j - 1].y + (lazo[j].y - lazo[j - 1].y) * t,
    });
  }
  return salida;
}

/** Lleva a la caja [0,1] conservando el aspecto, centrado. */
function normalizar(p: Punto[]): Punto[] {
  const xs = p.map((q) => q.x);
  const ys = p.map((q) => q.y);
  const [x0, x1] = [Math.min(...xs), Math.max(...xs)];
  const [y0, y1] = [Math.min(...ys), Math.max(...ys)];
  const lado = Math.max(x1 - x0, y1 - y0);
  const dx = (lado - (x1 - x0)) / 2;
  const dy = (lado - (y1 - y0)) / 2;
  return p.map((q) => ({
    x: (q.x - x0 + dx) / lado,
    y: (q.y - y0 + dy) / lado,
  }));
}

/** La ruta muestreada una sola vez, equidistante por arco, en [0,1]. */
export const TABLA: readonly Punto[] = normalizar(
  porArco(curva(controles(), 40), MUESTRAS),
);

/** Punto de la ruta en la fraccion `s` del lazo; envuelve fuera de [0,1). */
export function puntoEn(s: number): Punto {
  const u = (((s % 1) + 1) % 1) * MUESTRAS;
  const i = Math.floor(u) % MUESTRAS;
  const t = u - Math.floor(u);
  const a = TABLA[i];
  const b = TABLA[(i + 1) % MUESTRAS];
  return { x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t };
}
