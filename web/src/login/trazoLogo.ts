/**
 * Las rutas de la marca de Intela: una por hebra del nudo.
 *
 * La marca son cuatro hebras iguales, cada una la anterior girada un cuarto de
 * vuelta sobre el centro. Cada hebra es (casi) una elipse: se ajusto por
 * minimos cuadrados al eje de los trazos de `docs/intela-logo.png`, con la
 * simetria impuesta, y queda a ~2 px de media del eje (el trazo mide ~28 px).
 * En el PNG cada hebra es un arco abierto; la ruta cierra su elipse, asi que
 * la serpiente sigue la vuelta sin codos.
 */

export interface Punto {
  x: number;
  y: number;
}

/** Muestras de cada tabla. */
export const MUESTRAS = 480;

/** Elipse base, en px del PNG, relativa al centro de la marca (195.8, 172.6). */
const ELIPSE = {
  dx: -11.549,
  dy: -42.393,
  a: 137.198,
  b: 105.212,
  giro: 0.233,
};

const DENSIDAD = 4096;

/**
 * La hebra `k`: la base girada k cuartos de vuelta. El parametro crece en
 * sentido horario en pantalla (y hacia abajo), igual en las cuatro.
 */
function hebra(k: number): Punto[] {
  const g = (k * Math.PI) / 2;
  const [cg, sg] = [Math.cos(g), Math.sin(g)];
  const cx = ELIPSE.dx * cg - ELIPSE.dy * sg;
  const cy = ELIPSE.dx * sg + ELIPSE.dy * cg;
  const th = ELIPSE.giro + g;
  const [ct, st] = [Math.cos(th), Math.sin(th)];
  return Array.from({ length: DENSIDAD }, (_, i) => {
    const s = (i / DENSIDAD) * 2 * Math.PI;
    const u = ELIPSE.a * Math.cos(s);
    const v = ELIPSE.b * Math.sin(s);
    return { x: cx + u * ct - v * st, y: cy + u * st + v * ct };
  });
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

/**
 * Lleva todas las rutas a la MISMA caja [0,1], cuadrada y centrada: con una
 * caja por ruta la marca se desarmaria.
 */
function normalizar(rutas: Punto[][]): Punto[][] {
  const todos = rutas.flat();
  const xs = todos.map((q) => q.x);
  const ys = todos.map((q) => q.y);
  const [x0, x1] = [Math.min(...xs), Math.max(...xs)];
  const [y0, y1] = [Math.min(...ys), Math.max(...ys)];
  const lado = Math.max(x1 - x0, y1 - y0);
  const dx = (lado - (x1 - x0)) / 2;
  const dy = (lado - (y1 - y0)) / 2;
  return rutas.map((r) =>
    r.map((q) => ({ x: (q.x - x0 + dx) / lado, y: (q.y - y0 + dy) / lado })),
  );
}

/** Las cuatro hebras, muestreadas una vez y equidistantes por arco, en [0,1]. */
export const RUTAS: readonly (readonly Punto[])[] = normalizar(
  [0, 1, 2, 3].map((k) => porArco(hebra(k), MUESTRAS)),
);

/** Punto de la ruta `ruta` en la fraccion `s` del lazo; envuelve fuera de [0,1). */
export function puntoEn(ruta: number, s: number): Punto {
  const tabla = RUTAS[ruta];
  const u = (((s % 1) + 1) % 1) * MUESTRAS;
  const i = Math.floor(u) % MUESTRAS;
  const t = u - Math.floor(u);
  const a = tabla[i];
  const b = tabla[(i + 1) % MUESTRAS];
  return { x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t };
}
