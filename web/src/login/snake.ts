/**
 * Logica del adorno de la pantalla de acceso. Sin lienzo y sin DOM.
 *
 * Varias serpientes recorren trayectorias FIJAS. No se dirigen: no hay puntero
 * ni teclado, asi que tampoco hay teclas que quitarle al formulario -- el
 * riesgo que antes habia que vigilar desaparece por construccion, no por una
 * guarda.
 *
 * Vive aparte de `SnakeCanvas` porque `avanzar` es una funcion pura: el
 * movimiento y la colocacion del cuerpo se prueban sin un
 * `CanvasRenderingContext2D` -- que jsdom no implementa -- y sin esperar un
 * frame.
 *
 * # Por que trayectorias cerradas y no giros guionizados
 *
 * Una lista de giros con duraciones tambien seria "movimiento fijo", pero
 * deriva: a los pocos ciclos las serpientes se salen del panel y hay que
 * envolverlas por los bordes, que corta el cuerpo y se ve como una raya suelta.
 * Una curva de Lissajous cerrada se queda dentro por definicion, no repite el
 * mismo trazo de forma obvia, y con frecuencias distintas cada serpiente lleva
 * su propio dibujo sin escribir ninguno a mano.
 */

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

/**
 * Una trayectoria de Lissajous, en fraccion del panel.
 *
 * `cx`/`cy` son el centro y `rx`/`ry` la amplitud, las cuatro entre 0 y 1, asi
 * que la misma trayectoria vale para un panel de 720 px y para uno de 390 sin
 * recalcular nada.
 */
export interface Trayectoria {
  cx: number;
  cy: number;
  rx: number;
  ry: number;
  /** Frecuencias. Su relacion es la que dibuja la figura. */
  fx: number;
  fy: number;
  /** Desfase, en radianes. Separa dos serpientes con la misma figura. */
  fase: number;
  /** Velocidad, en radianes por segundo. */
  omega: number;
}

export interface Serpiente {
  trayectoria: Trayectoria;
  /** Segundos transcurridos sobre la trayectoria. */
  t: number;
  /**
   * Rastro de posiciones de la cabeza, de la mas reciente a la mas antigua.
   *
   * El cuerpo se coloca sobre este rastro y no se calcula de la formula: sobre
   * una Lissajous la velocidad no es constante, asi que muestrear la formula a
   * intervalos de tiempo iguales daria nodos mas juntos en las curvas y mas
   * separados en las rectas. Sobre el rastro se mide ARCO, y la separacion sale
   * igual en todo el recorrido.
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

/**
 * Definicion relativa de cada serpiente, antes de ajustarla al panel.
 *
 * Todas centradas en 0.5/0.5: asi el recorte de amplitud que mantiene la cabeza
 * dentro del panel es simetrico y se calcula con una resta. Lo que las
 * distingue es la FIGURA (`fx`/`fy`), no el sitio.
 */
export interface DefinicionSerpiente {
  trayectoria: Omit<Trayectoria, "cx" | "cy">;
  nodos: number;
  grosor: number;
  cabeza: number;
  alfa: number;
}

/**
 * Las cinco serpientes de la escena.
 *
 * Cada una con su FIGURA (`fx`:`fy` -- 3:2, 3:4, 2:3, 4:3, 5:3) y no la misma
 * desfasada: dos trazos iguales corriendo en paralelo se leen como un error de
 * repeticion. Y `omega` distinto en cada una, para que no se sincronicen.
 *
 * La lista va de mas a menos presencia, y el grosor y la opacidad bajan con
 * ella: es lo que da profundidad. Se pintan en orden inverso, asi que la
 * primera -- la mas gruesa y opaca -- queda encima.
 */
export const DEFINICIONES: readonly DefinicionSerpiente[] = [
  {
    trayectoria: { rx: 0.34, ry: 0.32, fx: 3, fy: 2, fase: 0, omega: 0.4 },
    nodos: 44,
    grosor: 9,
    cabeza: 46,
    alfa: 1,
  },
  {
    trayectoria: {
      rx: 0.32,
      ry: 0.28,
      fx: 3,
      fy: 4,
      fase: Math.PI / 5,
      omega: 0.35,
    },
    nodos: 40,
    grosor: 7,
    cabeza: 38,
    alfa: 0.68,
  },
  {
    trayectoria: {
      rx: 0.38,
      ry: 0.26,
      fx: 2,
      fy: 3,
      fase: Math.PI / 3,
      omega: 0.29,
    },
    nodos: 36,
    grosor: 6,
    cabeza: 32,
    alfa: 0.5,
  },
  {
    trayectoria: {
      rx: 0.26,
      ry: 0.36,
      fx: 4,
      fy: 3,
      fase: Math.PI / 1.4,
      omega: 0.23,
    },
    nodos: 28,
    grosor: 4,
    cabeza: 24,
    alfa: 0.28,
  },
  {
    trayectoria: {
      rx: 0.3,
      ry: 0.3,
      fx: 5,
      fy: 3,
      fase: Math.PI / 2.2,
      omega: 0.19,
    },
    nodos: 24,
    grosor: 3,
    cabeza: 19,
    alfa: 0.18,
  },
];

/**
 * Ajusta una definicion al panel.
 *
 * Dos cosas que el panel decide y la definicion no puede:
 *
 *  - **La escala.** El panel del movil mide 390x320 y el del escritorio
 *    720x900. Con las medidas fijas, en movil el cuerpo salia tan largo como
 *    alto el panel y la cabeza ocupaba un tercio del ancho.
 *  - **El recorte de amplitud.** La cabeza es una imagen de lado `cabeza`, asi
 *    que una amplitud de 0.34 la dejaba colgando fuera del borde -- se veia
 *    cortada por abajo en la captura del movil. Se recorta para que el centro
 *    de la cabeza nunca pase de medio logo mas un margen.
 */
export function dimensionar(
  def: DefinicionSerpiente,
  ancho: number,
  alto: number,
): Omit<Serpiente, "t" | "rastro"> {
  // Sobre la DIAGONAL y no sobre el lado menor: el panel del movil es
  // 390x320, y medir por el lado corto lo castigaba dos veces -- salian lineas
  // de 2 px y cabezas de 20, que se leian como garabatos y no como un elemento
  // de diseno. El suelo de 0.6 es lo que mantiene presencia ahi.
  const escala = Math.min(
    1.1,
    Math.max(0.6, Math.hypot(ancho, alto) / Math.hypot(720, 900)),
  );
  const cabeza = def.cabeza * escala;
  const margen = cabeza / 2 + 10;

  return {
    trayectoria: {
      ...def.trayectoria,
      cx: 0.5,
      cy: 0.5,
      rx: Math.max(0.05, Math.min(def.trayectoria.rx, 0.5 - margen / ancho)),
      ry: Math.max(0.05, Math.min(def.trayectoria.ry, 0.5 - margen / alto)),
    },
    nodos: Math.max(12, Math.round(def.nodos * escala)),
    grosor: Math.max(2, def.grosor * escala),
    cabeza,
    alfa: def.alfa,
  };
}

/** Donde esta la cabeza en el instante `t`. */
export function posicion(
  tr: Trayectoria,
  t: number,
  ancho: number,
  alto: number,
): Point {
  return {
    x: ancho * (tr.cx + tr.rx * Math.sin(tr.fx * tr.omega * t + tr.fase)),
    y: alto * (tr.cy + tr.ry * Math.sin(tr.fy * tr.omega * t)),
  };
}

/** Rumbo de la cabeza, para girar el logo hacia donde va. */
export function rumbo(
  tr: Trayectoria,
  t: number,
  ancho: number,
  alto: number,
): number {
  // Derivada analitica y no la diferencia con el frame anterior: en el primer
  // frame no hay anterior, y a dt pequeno la diferencia es ruido numerico que
  // haria temblar el logo.
  const dx =
    ancho * tr.rx * tr.fx * tr.omega * Math.cos(tr.fx * tr.omega * t + tr.fase);
  const dy = alto * tr.ry * tr.fy * tr.omega * Math.cos(tr.fy * tr.omega * t);
  return Math.atan2(dy, dx);
}

export function estadoInicial(ancho: number, alto: number): GameState {
  return {
    ancho,
    alto,
    serpientes: DEFINICIONES.map((def) => {
      const s = dimensionar(def, ancho, alto);
      return { ...s, t: 0, rastro: [posicion(s.trayectoria, 0, ancho, alto)] };
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
      const cabeza = posicion(s.trayectoria, t, estado.ancho, estado.alto);
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
 * distancia que a 60 Hz; y sobre una Lissajous la velocidad no es constante,
 * asi que ni siquiera a tasa fija los puntos estan repartidos.
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
