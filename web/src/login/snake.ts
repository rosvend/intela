/**
 * Logica del adorno de la pantalla de acceso. Sin lienzo y sin DOM.
 *
 * Cada serpiente recorre SU hebra de la marca de Intela (`trazoLogo`): juntas
 * van dibujando el nudo. No se dirigen, asi que no hay teclas que quitarle al formulario.
 *
 * `avanzar` es pura: se prueba sin `CanvasRenderingContext2D` y sin frames.
 */

import { MUESTRAS, puntoEn, RUTAS } from "./trazoLogo";

export interface Point {
  x: number;
  y: number;
}

/** Un nodo del cuerpo, ya colocado sobre el rastro que dejo la cabeza. */
export interface SnakeSegment extends Point {
  /**
   * Grosor en px. CONSTANTE a lo largo del cuerpo: esto es una linea, no una
   * cola.
   *
   * Hubo una version que lo estrechaba hacia el final. Dos motivos para
   * quitarlo: le daba silueta de animal, que no es lo que se pide; y como cada
   * tramo de grosor distinto hay que trazarlo aparte, los ultimos salian tan
   * finos que se veian como puntos suelos en vez de una linea que termina. Con
   * grosor constante el cuerpo es UN trazo con la punta redonda.
   */
  radio: number;
}

/** La ruta de la marca colocada en el panel: caja cuadrada en px. */
export interface Trayectoria {
  /** Esquina superior izquierda de la caja, en px. */
  x0: number;
  y0: number;
  /** Lado de la caja, en px. */
  lado: number;
  /** Hebra de la marca, indice en `RUTAS`. */
  ruta: number;
  /** Desfase sobre el lazo, en fraccion de vuelta. */
  fase: number;
  /** Vueltas por segundo. */
  velocidad: number;
}

export interface Serpiente {
  trayectoria: Trayectoria;
  /** Segundos transcurridos sobre la trayectoria. */
  t: number;
  /**
   * Rastro de posiciones de la cabeza, de la mas reciente a la mas antigua.
   *
   * El cuerpo se coloca sobre este rastro midiendo ARCO, asi la separacion
   * no depende de la tasa de frames.
   */
  rastro: Point[];
  /** Nodos del cuerpo. */
  nodos: number;
  /** Grosor de la linea, en px. */
  grosor: number;
  /** Lado del logo de la cabeza, en px. */
  cabeza: number;
  /** Opacidad, para dar profundidad entre serpientes. */
  alfa: number;
}

export interface GameState {
  serpientes: Serpiente[];
  ancho: number;
  alto: number;
}

export const AJUSTES = {
  /** Separacion entre nodos, en px. Fina: el cuerpo es una linea. */
  separacion: 7,
} as const;

/** Definicion relativa de cada serpiente, antes de ajustarla al panel. */
export interface DefinicionSerpiente {
  /** Hebra de la marca que recorre: indice en `RUTAS`. */
  ruta: number;
  fase: number;
  velocidad: number;
  nodos: number;
  grosor: number;
  cabeza: number;
  alfa: number;
}

/** Fraccion del lado menor del panel que ocupa la marca. */
const OCUPACION = 0.86;

/**
 * Una serpiente por hebra. Misma fase, mismo paso y mismo sentido: las hebras
 * son la misma elipse girada un cuarto de vuelta, asi que las cuatro cabezas
 * se mueven como una figura que gira y la marca se lee entera, no como cuatro
 * trazos sueltos.
 */
export const DEFINICIONES: readonly DefinicionSerpiente[] = RUTAS.map(
  (_, ruta) => ({
    ruta,
    fase: 0,
    velocidad: 0.2,
    nodos: 56,
    grosor: 7,
    cabeza: 36,
    alfa: 0.9,
  }),
);

/**
 * Ajusta una definicion al panel: escala de grosor y cabeza por la diagonal,
 * y la marca grande, cuadrada y centrada sin que la cabeza se salga.
 */
export function dimensionar(
  def: DefinicionSerpiente,
  ancho: number,
  alto: number,
): Omit<Serpiente, "t" | "rastro"> {
  // Diagonal y no lado menor: en el panel del movil (390x320) el lado corto
  // dejaba lineas de 2 px; el suelo de 0.6 mantiene presencia.
  const escala = Math.min(
    1.1,
    Math.max(0.6, Math.hypot(ancho, alto) / Math.hypot(720, 900)),
  );
  const cabeza = def.cabeza * escala;
  const margen = cabeza / 2 + 10;
  const menor = Math.min(ancho, alto);
  const lado = Math.max(1, Math.min(menor * OCUPACION, menor - 2 * margen));

  return {
    trayectoria: {
      x0: (ancho - lado) / 2,
      y0: (alto - lado) / 2,
      lado,
      ruta: def.ruta,
      fase: def.fase,
      velocidad: def.velocidad,
    },
    nodos: Math.max(12, Math.round(def.nodos * escala)),
    grosor: Math.max(2, def.grosor * escala),
    cabeza,
    alfa: def.alfa,
  };
}

function enPanel(tr: Trayectoria, p: { x: number; y: number }): Point {
  return { x: tr.x0 + p.x * tr.lado, y: tr.y0 + p.y * tr.lado };
}

/** Donde esta la cabeza en el instante `t`. */
export function posicion(tr: Trayectoria, t: number): Point {
  return enPanel(tr, puntoEn(tr.ruta, tr.fase + tr.velocidad * t));
}

/** Rumbo de la cabeza: diferencia entre las muestras vecinas de la ruta. */
export function rumbo(tr: Trayectoria, t: number): number {
  const s = tr.fase + tr.velocidad * t;
  const h = 1 / MUESTRAS;
  const a = puntoEn(tr.ruta, s - h);
  const b = puntoEn(tr.ruta, s + h);
  return Math.atan2(b.y - a.y, b.x - a.x);
}

/** La hebra completa en px, para pintar la guia tenue de la marca. */
export function trazo(tr: Trayectoria): Point[] {
  return RUTAS[tr.ruta].map((p) => enPanel(tr, p));
}

export function estadoInicial(ancho: number, alto: number): GameState {
  return {
    ancho,
    alto,
    serpientes: DEFINICIONES.map((def) => {
      const s = dimensionar(def, ancho, alto);
      return { ...s, t: 0, rastro: [posicion(s.trayectoria, 0)] };
    }),
  };
}

/** Cuanto rastro hace falta guardar para vestir el cuerpo. */
function rastroNecesario(s: Serpiente): number {
  return s.nodos * AJUSTES.separacion + AJUSTES.separacion * 2;
}

/**
 * Un paso de simulacion.
 *
 * `dt` en segundos, y quien llama lo acota (ver `SnakeCanvas`): al volver de
 * una pestana en segundo plano el primer dt vale varios segundos, y la cabeza
 * daria un salto que parte el rastro.
 */
export function avanzar(estado: GameState, dt: number): GameState {
  return {
    ...estado,
    serpientes: estado.serpientes.map((s) => {
      const t = s.t + dt;
      const cabeza = posicion(s.trayectoria, t);
      return {
        ...s,
        t,
        rastro: recortar([cabeza, ...s.rastro], rastroNecesario(s)),
      };
    }),
  };
}

/**
 * Recorta el rastro a la longitud que viste el cuerpo.
 *
 * Sin esto el array crece un punto por frame mientras la pestana este abierta
 * -- 216 000 puntos por hora a 60 Hz --, que es una fuga lenta en una pantalla
 * que la gente deja abierta.
 */
function recortar(rastro: Point[], necesario: number): Point[] {
  let acumulado = 0;
  for (let i = 1; i < rastro.length; i++) {
    acumulado += Math.hypot(
      rastro[i].x - rastro[i - 1].x,
      rastro[i].y - rastro[i - 1].y,
    );
    if (acumulado >= necesario) return rastro.slice(0, i + 1);
  }
  return rastro;
}

/**
 * Coloca los nodos del cuerpo sobre el rastro, separados por ARCO.
 *
 * Por arco y no "un nodo cada N puntos del array" por dos razones: el array
 * crece un punto por frame, asi que a 120 Hz los puntos estan a la mitad de
 * distancia que a 60 Hz.
 *
 * `acumulado` NO se reinicia entre nodos. Reiniciarlo era el defecto de la
 * primera version: cada nodo volvia a recorrer `objetivo` desde donde lo dejo
 * el anterior, asi que la separacion crecia como 1+2+3... y el cuerpo parecia
 * un collar de cuentas suelto. Se vio en una captura, no en las pruebas.
 */
export function segmentos(s: Serpiente): SnakeSegment[] {
  const salida: SnakeSegment[] = [];
  let indice = 1;
  let acumulado = 0;

  for (let n = 0; n < s.nodos; n++) {
    const objetivo = n * AJUSTES.separacion;

    while (indice < s.rastro.length) {
      const a = s.rastro[indice - 1];
      const b = s.rastro[indice];
      const tramo = Math.hypot(b.x - a.x, b.y - a.y);
      if (acumulado + tramo >= objetivo) break;
      acumulado += tramo;
      indice++;
    }

    let punto: Point;
    if (indice < s.rastro.length) {
      const a = s.rastro[indice - 1];
      const b = s.rastro[indice];
      const tramo = Math.hypot(b.x - a.x, b.y - a.y);
      const t = tramo === 0 ? 0 : (objetivo - acumulado) / tramo;
      punto = { x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t };
    } else {
      // El rastro se acabo: el resto se apila en la cola. Pasa en los primeros
      // segundos, y es lo que hace que el cuerpo se despliegue detras de la
      // cabeza en vez de aparecer estirado de golpe.
      punto = s.rastro[s.rastro.length - 1];
    }

    salida.push({ ...punto, radio: s.grosor });
  }

  return salida;
}
