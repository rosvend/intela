import { useEffect, useRef, useState } from "react";
import { ApiError, ErrorDeRed, api } from "../api";
import { Rol } from "../sesion";
import { esAusente } from "./ausente";
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
};

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

  return {
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
 * Si el path no cambio y ya habia datos, no se pinta "Cargando…" otra vez:
 * un parpadeo cada 15s haria ilegible el pipeline.
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
      !cambioDePath && actual.tipo === "listo" ? actual : { tipo: "cargando" },
    );

    (api(path) as Promise<T>)
      .then((datos) => {
        if (vigente) setRecurso({ tipo: "listo", datos });
      })
      .catch((error: unknown) => {
        if (!vigente) return;
        if (esAusente(error)) {
          setRecurso({ tipo: "ausente" });
          return;
        }
        const mensaje =
          error instanceof ApiError || error instanceof ErrorDeRed
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
