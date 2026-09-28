/**
 * Citas del reglamento para el recibo de "mas detalles" de ExplicarCifra.
 *
 * El titular no tiene el Reglamento de Distribucion memorizado: una cita
 * como "RD 9.1.1" a secas no significa nada (feedback del PO tras probar el
 * panel). Esta tabla traduce cada cita que puede llegar de verdad en
 * `GET /explicar/{ref}` a un nombre humano y un extracto VERBATIM, copiado de
 * `docs/reglamentos/distribucion-v9/*.md` -- nunca parafraseado de memoria.
 *
 * Deliberadamente NO es un servidor generico de reglamento (YAGNI): solo
 * cubre las citas que pueden salir de una respuesta real --
 *   - `regla.reglamento` es un "+"-join de las citas de TODOS los parametros
 *     que pesaron en el snapshot (postgres/parametros.go), asi que trae una o
 *     mas de las modalidades de RD 9 (una por cada corrida, segun
 *     internal/dominio/reparto/doc.go) mas el marcador de siembra sintetica.
 *   - los tres conceptos de deduccion (internal/dominio/liquidacion/linea.go)
 *     no traen su propia cita en el JSON: la relacion concepto -> regla es
 *     fija y vive aqui.
 *   - `retenida: true` es siempre R-04 / RD 13.1.3.
 * Ampliar esta tabla cuando aparezca una cita nueva en produccion, no antes.
 */

export type CitaReglamento = {
  /** Nombre humano de la regla, para un titular sin el reglamento memorizado. */
  titulo: string;
  /** Extracto verbatim de la fuente. Vacio solo para una cita sin match (fallback). */
  texto: string;
  /** Ruta del .md fuente, para quien quiera verificar la cita completa. */
  fuente: string;
};

const CITAS: Record<string, CitaReglamento> = {
  "RD 9.1.1": {
    titulo: "Television abierta y radiodifundida — formula de valorizacion",
    texto:
      '"Total puntos por obra = Tipo de obra * Duracion * Rating (franja horaria)". ' +
      "Ponderacion por tipo de obra: Cinematografica 5.0, Unitario 2.8, Serie/Telenovela 1.3, Sketches 0.8.",
    fuente: "docs/reglamentos/distribucion-v9/09-metodologia-para-la-distribucion.md",
  },
  "RD 9.2": {
    titulo: "Exhibidores cinematograficos y salas de cine",
    texto:
      '"El importe correspondiente a cada obra cinematografica se determinara con base en los ' +
      "ingresos de taquilla de cada ejercicio economico que obtuvo cada obra basado en la " +
      'informacion suministrada por el usuario."',
    fuente: "docs/reglamentos/distribucion-v9/09-metodologia-para-la-distribucion.md",
  },
  "RD 9.3": {
    titulo: "Teatros",
    texto:
      '"El importe correspondiente a cada obra se determinara con base en los ingresos de ' +
      "taquilla de cada ejercicio economico que obtuvo cada obra, basado en la informacion " +
      'suministrada por el usuario."',
    fuente: "docs/reglamentos/distribucion-v9/09-metodologia-para-la-distribucion.md",
  },
  "RD 9.4": {
    titulo: "Medios de transporte publico",
    texto:
      '"REDES SGC recaudara los datos de las obras exhibidas durante el ejercicio y el numero ' +
      'de exhibiciones de cada una de ellas."',
    fuente: "docs/reglamentos/distribucion-v9/09-metodologia-para-la-distribucion.md",
  },
  "RD 9.5": {
    titulo: "Operadores de television por suscripcion",
    texto:
      '"El porcentaje que a cada grupo de canales le corresponde, se distribuira entre los ' +
      "escritores de las obras audiovisuales representadas por REDES SGC que se comuniquen en " +
      'dichos canales, aplicando la formula de valorizacion de la obra descrita en el numeral ' +
      '9.1.1 de este Reglamento." Grupos: privados nacionales 50%, regionales/locales/publicos ' +
      "20%, premium 10%, lideres en rating 10%, estandar 10%.",
    fuente: "docs/reglamentos/distribucion-v9/09-metodologia-para-la-distribucion.md",
  },
  "RD 9.6": {
    titulo: "Establecimientos hoteleros y otros abiertos al publico",
    texto:
      '"La distribucion se efectuara a partir del importe recaudado de los diferentes ' +
      "establecimientos, una vez practicadas las Deducciones Legales, utilizando las " +
      'disposiciones del numeral 9.5 del presente reglamento."',
    fuente: "docs/reglamentos/distribucion-v9/09-metodologia-para-la-distribucion.md",
  },
  "RD 9.7": {
    titulo: "Plataformas Over The Top (OTT) y nuevas tecnologias",
    texto:
      '"Pi = Cantidad de puntos asignados a la obra audiovisual. PB = Puntaje base para el ' +
      "tipo de obra. Du = Tiempo en minutos durante los cuales la obra es vista. V = " +
      'Representa la cantidad de veces que una obra ha sido vista."',
    fuente: "docs/reglamentos/distribucion-v9/09-metodologia-para-la-distribucion.md",
  },
  "RD-IX-seed-sintetico": {
    titulo: "Cifra provisional de siembra (sin acta de Asamblea)",
    texto:
      "Esta tasa todavia no tiene el acta de aprobacion de la Asamblea General (P-10): es un " +
      "valor de siembra para ambientes de prueba, no una cifra vigente del reglamento.",
    fuente: "internal/infraestructura/semilla/dataset.go",
  },
  "RD 13.1.3": {
    titulo: "Retencion por declaracion incompleta (R-04)",
    texto:
      '"Si al momento de realizar el reparto del recaudo correspondiente a la obra no se ' +
      "cuenta con la declaracion discriminada del 100% de los porcentajes a repartir, REDES " +
      "SGC mantendra en reserva el total del importe a distribuir respecto a dicha obra, hasta " +
      'tanto se declare el 100% de la misma."',
    fuente: "docs/reglamentos/distribucion-v9/13-sistema-de-distribucion.md",
  },
};

const CITAS_POR_CONCEPTO: Record<string, CitaReglamento> = {
  gastos_administrativos: {
    titulo: "Gastos administrativos (R-06)",
    texto:
      '"...establece que de las sumas recaudadas, las sociedades de gestion colectiva podran ' +
      "deducir hasta un 20% para gastos administrativos y hasta un 30% por los dos primeros " +
      'anos de existencia de la sociedad." (Art. 21 Ley 44 de 1993, modificado por Art. 23 Ley ' +
      "1493 de 2011)",
    fuente: "docs/reglamentos/distribucion-v9/03-definiciones.md",
  },
  bienestar_social: {
    titulo: "Bienestar social (R-06)",
    texto:
      '"...establece que las sociedades de gestion colectiva podran destinar hasta un 10% de ' +
      'los dineros recaudados para ser destinados a programas de inversion social." (Art. 21 ' +
      "Ley 44 de 1993)",
    fuente: "docs/reglamentos/distribucion-v9/03-definiciones.md",
  },
  reserva_errores_tecnicos: {
    titulo: "Reserva para correccion de errores tecnicos (R-07)",
    texto:
      '"REDES SGC podra mantener en reserva hasta un 5% del recaudo nacional del porcentaje ' +
      "asignado a REDES SGC para distribucion, para corregir dichos errores y/o solucionar " +
      'adecuadamente los reclamos de los titulares." (RD 14.1)',
    fuente: "docs/reglamentos/distribucion-v9/14-reserva-para-correccion-de-errores-tecnicos.md",
  },
};

const NOMBRES_CONCEPTO: Record<string, string> = {
  gastos_administrativos: "Gastos administrativos",
  bienestar_social: "Bienestar social",
  reserva_errores_tecnicos: "Reserva para errores tecnicos",
};

/** Nombre en lenguaje llano de un concepto de deduccion; el propio codigo si no se conoce. */
export function nombreConcepto(concepto: string): string {
  return NOMBRES_CONCEPTO[concepto] ?? concepto;
}

/** La cita R-06/R-07 fija de un concepto de deduccion, si esta en la tabla curada. */
export function citaDeConcepto(concepto: string): CitaReglamento | undefined {
  return CITAS_POR_CONCEPTO[concepto];
}

/** La cita fija de R-04 (retencion por declaracion incompleta). */
export function citaDeRetencion(): CitaReglamento {
  return CITAS["RD 13.1.3"];
}

/**
 * `regla.reglamento` llega como un "+"-join de todas las citas que pesaron en
 * el snapshot (p.ej. "RD 9.1.1+RD-IX-seed-sintetico"). Cada token se busca por
 * separado y en el orden en que llego; uno sin cita conocida se devuelve tal
 * cual con `texto` vacio, nunca inventado.
 */
export function citarReglamento(
  reglamento: string,
): (CitaReglamento & { token: string })[] {
  return reglamento
    .split("+")
    .map((token) => token.trim())
    .filter(Boolean)
    .map((token) => {
      const cita = CITAS[token];
      return cita ? { ...cita, token } : { token, titulo: token, texto: "", fuente: "" };
    });
}
