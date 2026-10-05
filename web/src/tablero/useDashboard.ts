import { useEffect, useRef, useState } from "react";
import {
  api,
  esErrorDeApi,
  esRespuestaSinJson,
  MENSAJE_RESPUESTA_SIN_JSON,
} from "../api";
import { Asiento, RUTAS_AUDITORIA } from "../auditoria/tipos";
import { puedeVer } from "../navegacion";
import { Proceso, ResumenDeAlertas, RUTAS_REPARTO } from "../reparto/tipos";
import { Rol } from "../sesion";
import { esAusente } from "./ausente";
import { Bolsa } from "./recaudo";
import {
  Conteo,
  MisObras,
  Recurso,
  RUTAS_TABLERO,
  UltimaCorrida,
  UltimaLiquidacion,
} from "./tipos";

export type Tablero = {
  cargasPendientes: Recurso<Conteo>;
  obrasEnReserva: Recurso<Conteo>;
  oni: Recurso<Conteo>;
  ultimaCorrida: Recurso<UltimaCorrida>;
  misObras: Recurso<MisObras>;
  ultimaLiquidacion: Recurso<UltimaLiquidacion>;
  procesos: Recurso<Proceso[]>;
  bolsas: Recurso<Bolsa[]>;
  resumenAlertas: Recurso<ResumenDeAlertas>;
  asientos: Recurso<Asiento[]>;
  /** Periodo de la corrida mas reciente; contexto del panel de staff. */
  periodo?: string;
};

/** Actividad reciente del panel: la bitacora es larga, el tablero no. */
export const RUTA_ACTIVIDAD = `${RUTAS_AUDITORIA.asientos}?limite=5`;

/**
 * Hook de datos del tablero. Un recurso por widget, en paralelo, para que
 * cuando aterrice un endpoint (ONI, corridas, liquidaciones) solo se
 * encienda esa tarjeta. `habilitado` evita pedir conteos de administrador
 * con sesion de titular, y al reves.
 *
 * Vive aqui y no en `useApi` porque el tablero tiene una semantica propia:
 * un 404 no es un error, es "todavia no hay backend".
 */
export function useDashboard(rol: Rol): Tablero {
  const esTitular = rol === "titular";

  const cargasPendientes = useRecurso<Conteo>(
    RUTAS_TABLERO.cargasPendientes,
    !esTitular,
  );
  const obrasEnReserva = useRecurso<Conteo>(
    RUTAS_TABLERO.obrasEnReserva,
    !esTitular,
  );
  const oni = useRecurso<Conteo>(RUTAS_TABLERO.oni, !esTitular);
  const ultimaCorrida = useRecurso<UltimaCorrida>(
    RUTAS_TABLERO.ultimaCorrida,
    !esTitular,
  );
  const misObras = useRecurso<MisObras>(RUTAS_TABLERO.misObras, esTitular);
  const ultimaLiquidacion = useRecurso<UltimaLiquidacion>(
    RUTAS_TABLERO.ultimaLiquidacion,
    esTitular,
  );

  const procesos = useRecurso<Proceso[]>(RUTAS_REPARTO.procesos, !esTitular);
  const bolsas = useRecurso<Bolsa[]>("/api/bolsas", !esTitular);
  const asientos = useRecurso<Asiento[]>(
    RUTA_ACTIVIDAD,
    puedeVer(rol, "/auditoria"),
  );
  // /api/procesos lista la mas reciente primero (PanelCorridas toma [0]).
  const periodo =
    procesos.tipo === "listo" && Array.isArray(procesos.datos)
      ? procesos.datos[0]?.periodo
      : undefined;
  const resumenAlertas = useRecurso<ResumenDeAlertas>(
    RUTAS_REPARTO.resumenAlertas(periodo ?? ""),
    !esTitular && Boolean(periodo) && puedeVer(rol, "/anomalias"),
  );

  return {
    procesos,
    bolsas,
    asientos,
    resumenAlertas,
    periodo,
    cargasPendientes,
    obrasEnReserva,
    oni,
    ultimaCorrida,
    misObras,
    ultimaLiquidacion,
  };
}

/**
 * `recarga` fuerza un refetch (sondeo del panel de corridas, o tras firmar).
 * Si el path no cambio y el recurso ya estaba resuelto, no se pinta
 * "Cargando…" otra vez: un parpadeo cada 15s haria ilegible el pipeline.
 * Resuelto incluye `ausente` y `error`, que hoy son los estados que el
 * usuario ve mientras el backend no exista; remontar su mensaje cada 15s
 * parpadea igual y ademas re-anuncia el `role="alert"` en lectores.
 */
export function useRecurso<T>(
  path: string,
  habilitado = true,
  recarga = 0,
): Recurso<T> {
  const [recurso, setRecurso] = useState<Recurso<T>>(
    habilitado ? { tipo: "cargando" } : { tipo: "inactivo" },
  );
  const pathAnterior = useRef(path);

  useEffect(() => {
    if (!habilitado) {
      setRecurso({ tipo: "inactivo" });
      return;
    }

    const cambioDePath = pathAnterior.current !== path;
    pathAnterior.current = path;

    let vigente = true;
    setRecurso((actual) =>
      !cambioDePath && esResuelto(actual) ? actual : { tipo: "cargando" },
    );

    (api(path) as Promise<T>)
      .then((datos) => {
        if (!vigente) return;
        // El mismo corte que `useApi`: un 2xx sin JSON llega como `Response`
        // crudo, y entregarlo como `datos` es lo que tumbaba el tablero del
        // titular (`datos.obras.length` sobre un `Response`). `T` no se
        // comprueba; esta frontera si distingue la respuesta sin leer.
        if (esRespuestaSinJson(datos)) {
          setRecurso({ tipo: "error", mensaje: MENSAJE_RESPUESTA_SIN_JSON });
          return;
        }
        setRecurso({ tipo: "listo", datos });
      })
      .catch((error: unknown) => {
        if (!vigente) return;
        if (esAusente(error, path)) {
          setRecurso({ tipo: "ausente" });
          return;
        }
        // Los mismos errores tipados que en `useApi` -incluido el 2xx con el
        // cuerpo ilegible, cuyo mensaje dice mas que el generico de abajo-, y
        // decididos por el mismo predicado de `api.ts`: la lista se escribe una
        // vez, donde estan las clases. Ver D-016.
        const mensaje = esErrorDeApi(error)
          ? error.message
          : "no se pudo cargar este indicador";
        setRecurso({ tipo: "error", mensaje });
      });

    return () => {
      vigente = false;
    };
  }, [path, habilitado, recarga]);

  return recurso;
}

/**
 * `inactivo` no cuenta: viene de `habilitado: false` y al encenderse hay que
 * pintar la carga, no el vacio que dejo el recurso apagado.
 */
function esResuelto<T>(recurso: Recurso<T>): boolean {
  return (
    recurso.tipo === "listo" ||
    recurso.tipo === "ausente" ||
    recurso.tipo === "error"
  );
}
