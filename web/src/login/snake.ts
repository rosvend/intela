/**
 * Logica del juego de la pantalla de acceso. Sin lienzo y sin DOM.
 *
 * Vive aparte de `SnakeCanvas` a proposito: `avanzar` es una funcion pura, asi
 * que se puede probar el movimiento, el giro y el crecimiento sin un
 * `CanvasRenderingContext2D` -que jsdom no implementa- y sin esperar un frame.
 * Lo que queda en el componente es pintar y escuchar eventos.
 *
 * No hay estado de derrota, y es una decision de producto: esto adorna un
 * formulario de acceso. Una pantalla que le dice "perdiste" a quien viene a
 * entrar a trabajar no ayuda a nadie, asi que la serpiente no choca contra si
 * misma y los bordes se cruzan de un lado al otro.
 */

export interface Point {
  x: number;
  y: number;
}

/** Un nodo del cuerpo, ya colocado sobre el camino que dejo la cabeza. */
export interface SnakeSegment extends Point {
  /** Radio en px. Se estrecha hacia la cola. */
  radio: number;
}

export interface Comida extends Point {
  radio: number;
}

/** Como se esta dirigiendo la serpiente en este momento. */
export type Entrada =
  | { tipo: "puntero"; objetivo: Point }
  | { tipo: "rumbo"; angulo: number }
  | { tipo: "ninguna" };

export interface GameState {
  /** Posicion de la cabeza. */
  cabeza: Point;
  /** Rumbo en radianes. 0 apunta a la derecha. */
  angulo: number;
  /**
   * Rastro de posiciones por las que paso la cabeza, de la mas reciente a la
   * mas antigua. El cuerpo se coloca sobre este camino, no se simula: es lo
   * que da el movimiento continuo de slither.io en vez de una rejilla.
   */
  camino: Point[];
  comida: Comida[];
  puntos: number;
  ancho: number;
  alto: number;
}

/** Numeros del juego, juntos para poder afinarlos de un vistazo. */
export const AJUSTES = {
  /** px por segundo. */
  velocidad: 190,
  /** radianes por segundo. Limita el giro para que la curva sea suave. */
  giroMaximo: 3.6,
  radioCabeza: 17,
  radioCola: 6,
  /**
   * Nodos del cuerpo sin comer nada.
   *
   * 18 * 9 px daba un cuerpo de 153 px, que en un panel de 720 px de ancho se
   * veia rechoncho. 26 lo llevan a 225 px, con presencia en escritorio y sin
   * pasarse en el panel de 390 px del movil.
   */
  nodosBase: 26,
  /** Nodos que anade cada comida. */
  nodosPorComida: 3,
  nodosMaximos: 60,
  /**
   * Separacion entre nodos a lo largo del camino, en px.
   *
   * Menor que el diametro del nodo mas fino (radioCola * 2 = 12) a proposito:
   * el cuerpo se pinta como circulos, y con separacion mayor se veria como un
   * collar de cuentas suelto en vez de un cuerpo continuo.
   */
  separacion: 9,
  radioComida: 6,
  comidaEnPantalla: 5,
  /** Margen para no generar comida pegada al borde. */
  margenComida: 34,
} as const;

/** Cuantos nodos tiene el cuerpo con la puntuacion actual. */
export function nodosDelCuerpo(puntos: number): number {
  return Math.min(
    AJUSTES.nodosMaximos,
    AJUSTES.nodosBase + puntos * AJUSTES.nodosPorComida,
  );
}

/**
 * Longitud de camino que hace falta guardar.
 *
 * Se recorta a esto en cada paso: sin el recorte el array crece sin limite
 * mientras la pestana este abierta, que es una fuga de memoria lenta en una
 * pantalla que la gente deja abierta.
 */
function caminoNecesario(puntos: number): number {
  return nodosDelCuerpo(puntos) * AJUSTES.separacion + AJUSTES.separacion * 2;
}

/**
 * `aleatorio` entra por parametro en vez de llamar a Math.random aqui dentro.
 *
 * Es lo mismo que hace el nucleo de Go con `PuertoReloj`: con la fuente de
 * azar inyectada, una prueba puede fijar donde aparece la comida y afirmar que
 * la serpiente crecio al comerla, en vez de perseguir un objetivo que se mueve.
 */
export type Aleatorio = () => number;

export function comidaNueva(
  ancho: number,
  alto: number,
  aleatorio: Aleatorio,
): Comida {
  const m = AJUSTES.margenComida;
  return {
    x: m + aleatorio() * Math.max(1, ancho - m * 2),
    y: m + aleatorio() * Math.max(1, alto - m * 2),
    radio: AJUSTES.radioComida,
  };
}

export function estadoInicial(
  ancho: number,
  alto: number,
  aleatorio: Aleatorio,
): GameState {
  const cabeza: Point = { x: ancho / 2, y: alto / 2 };
  return {
    cabeza,
    angulo: 0,
    // El camino arranca con un solo punto: el cuerpo se despliega detras de la
    // cabeza en los primeros frames, sin aparecer estirado de golpe.
    camino: [cabeza],
    comida: Array.from({ length: AJUSTES.comidaEnPantalla }, () =>
      comidaNueva(ancho, alto, aleatorio),
    ),
    puntos: 0,
    ancho,
    alto,
  };
}

/** Diferencia angular normalizada a (-PI, PI]. */
export function diferenciaAngular(desde: number, hasta: number): number {
  let d = (hasta - desde) % (Math.PI * 2);
  if (d > Math.PI) d -= Math.PI * 2;
  if (d <= -Math.PI) d += Math.PI * 2;
  return d;
}

/** Rumbo que pide la entrada, o null si no pide ninguno. */
function rumboPedido(estado: GameState, entrada: Entrada): number | null {
  switch (entrada.tipo) {
    case "puntero": {
      const dx = entrada.objetivo.x - estado.cabeza.x;
      const dy = entrada.objetivo.y - estado.cabeza.y;
      // Con el puntero encima de la cabeza no hay direccion que calcular, y
      // atan2(0,0) daria 0 -o sea, un tiron hacia la derecha.
      if (Math.hypot(dx, dy) < 1) return null;
      return Math.atan2(dy, dx);
    }
    case "rumbo":
      return entrada.angulo;
    case "ninguna":
      return null;
  }
}

/** Envuelve una coordenada por los bordes. */
function envolver(v: number, limite: number): number {
  if (limite <= 0) return v;
  return ((v % limite) + limite) % limite;
}

/**
 * Un paso de simulacion.
 *
 * `dt` en segundos. Se acota antes de entrar (ver `SnakeCanvas`): con la
 * pestana en segundo plano `requestAnimationFrame` deja de llamarse, y al
 * volver el primer dt vale varios segundos, que teletransportaria la cabeza al
 * otro lado del lienzo.
 */
export function avanzar(
  estado: GameState,
  dt: number,
  entrada: Entrada,
  aleatorio: Aleatorio,
): GameState {
  const pedido = rumboPedido(estado, entrada);

  let angulo = estado.angulo;
  if (pedido !== null) {
    const d = diferenciaAngular(angulo, pedido);
    const maximo = AJUSTES.giroMaximo * dt;
    angulo += Math.max(-maximo, Math.min(maximo, d));
  }

  const avance = AJUSTES.velocidad * dt;
  const cabeza: Point = {
    x: envolver(estado.cabeza.x + Math.cos(angulo) * avance, estado.ancho),
    y: envolver(estado.cabeza.y + Math.sin(angulo) * avance, estado.alto),
  };

  // Al cruzar un borde se corta el camino en vez de continuarlo: si no, el
  // cuerpo se dibujaria como una raya recta de un extremo al otro del lienzo.
  const cruzo =
    Math.abs(cabeza.x - estado.cabeza.x) > estado.ancho / 2 ||
    Math.abs(cabeza.y - estado.cabeza.y) > estado.alto / 2;

  const camino = cruzo ? [cabeza] : [cabeza, ...estado.camino];

  let puntos = estado.puntos;
  const comida: Comida[] = [];
  for (const c of estado.comida) {
    const alcanzada =
      Math.hypot(c.x - cabeza.x, c.y - cabeza.y) <
      c.radio + AJUSTES.radioCabeza * 0.7;
    if (alcanzada) {
      puntos += 1;
      comida.push(comidaNueva(estado.ancho, estado.alto, aleatorio));
    } else {
      comida.push(c);
    }
  }

  return {
    ...estado,
    cabeza,
    angulo,
    camino: recortarCamino(camino, puntos),
    comida,
    puntos,
  };
}

function recortarCamino(camino: Point[], puntos: number): Point[] {
  const necesario = caminoNecesario(puntos);
  let acumulado = 0;
  for (let i = 1; i < camino.length; i++) {
    acumulado += Math.hypot(
      camino[i].x - camino[i - 1].x,
      camino[i].y - camino[i - 1].y,
    );
    if (acumulado >= necesario) return camino.slice(0, i + 1);
  }
  return camino;
}

/**
 * Coloca los nodos del cuerpo sobre el camino, separados por arco y no por
 * indice.
 *
 * Por arco y no "un nodo cada N puntos del array" porque el array crece un
 * punto por frame: a 120 Hz los puntos estan a la mitad de distancia que a
 * 60 Hz, y el cuerpo saldria con la mitad de largo en un monitor rapido.
 */
export function segmentos(estado: GameState): SnakeSegment[] {
  const total = nodosDelCuerpo(estado.puntos);
  const salida: SnakeSegment[] = [];

  // `acumulado` es la distancia de la cabeza hasta camino[indice - 1], y NO se
  // reinicia entre nodos. Reiniciarlo era el defecto de la primera version:
  // cada nodo volvia a recorrer `objetivo` desde donde lo dejo el anterior, asi
  // que la separacion crecia como 1+2+3... y a los cinco nodos ya iban a 50 px
  // en vez de a 9. Se vio en una captura -- el cuerpo parecia un collar de
  // cuentas suelto -- y no en las pruebas, porque la que habia comparaba 60 Hz
  // contra 120 Hz: las dos salian igual de mal y la comparacion pasaba.
  let indice = 1;
  let acumulado = 0;

  for (let n = 0; n < total; n++) {
    const objetivo = n * AJUSTES.separacion;

    while (indice < estado.camino.length) {
      const a = estado.camino[indice - 1];
      const b = estado.camino[indice];
      const tramo = Math.hypot(b.x - a.x, b.y - a.y);
      if (acumulado + tramo >= objetivo) break;
      acumulado += tramo;
      indice++;
    }

    let punto: Point;
    if (indice < estado.camino.length) {
      const a = estado.camino[indice - 1];
      const b = estado.camino[indice];
      const tramo = Math.hypot(b.x - a.x, b.y - a.y);
      const t = tramo === 0 ? 0 : (objetivo - acumulado) / tramo;
      punto = { x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t };
    } else {
      // El camino se acabo: el resto de nodos se apila en la cola. Pasa en los
      // primeros frames y justo despues de cruzar un borde, y es lo que hace
      // que el cuerpo se despliegue en vez de aparecer estirado de golpe.
      punto = estado.camino[estado.camino.length - 1] ?? estado.cabeza;
    }

    // Se estrecha hacia la cola con una curva, no lineal: la lineal deja el
    // cuerpo con pinta de cono y no de animal.
    const t = total === 1 ? 0 : n / (total - 1);
    const radio =
      AJUSTES.radioCola +
      (AJUSTES.radioCabeza - AJUSTES.radioCola) * Math.pow(1 - t, 0.65);

    salida.push({ ...punto, radio });
  }

  return salida;
}
