import { useCallback, useMemo, useState } from "react";
import { useOutletContext } from "react-router-dom";
import { useRecurso } from "../tablero/useDashboard";
import { RUTA_CONTEO_PENDIENTES } from "./tipos";

/**
 * El conteo de pendientes, compartido entre el badge del sidebar (`Layout`) y
 * la bandeja por `useOutletContext` de react-router (D1 del plano de #39): sin
 * react-query, porque el unico dato que se comparte es un numero y los dos
 * consumidores viven en la misma rama del arbol (el shell y su Outlet).
 *
 * `fijarPendientes` es lo que la bandeja llama para empujar su propio conteo
 * optimista -tras remover un caso de la lista- sin esperar a que el badge
 * vuelva a pedir el recurso.
 */
export type ContextoIdentificacion = {
  pendientes: number | undefined;
  fijarPendientes: (valor: number) => void;
};

/**
 * `true` si el valor sin tipar trae un campo `pendientes` con forma de
 * conteo: un entero mayor o igual a cero.
 *
 * A proposito NO valida el resto de la forma de `PaginaCasosIdentificacion`
 * (eso es `esPaginaDeCasos`, en `tipos.ts`): el badge pide `limite=1` solo
 * para tener el conteo, y un `caso` mal formado en esa unica fila no es motivo
 * para esconder el numero que si vino bien.
 */
function esConteoValido(valor: unknown): valor is { pendientes: number } {
  if (typeof valor !== "object" || valor === null) return false;
  const pendientes = (valor as Record<string, unknown>)["pendientes"];
  return Number.isInteger(pendientes) && (pendientes as number) >= 0;
}

/**
 * El conteo de casos pendientes para el badge del sidebar.
 *
 * `habilitado` es `puedeVer(rol, "/identificacion")`: quien no es
 * administrador no hace la peticion. Cualquier desenlace que no sea "un
 * entero valido en el cuerpo" -404, error de red, 503, una forma que no
 * calza- se lee como "sin badge", nunca como un error visible: el contador es
 * un adorno, no una pantalla de error mas.
 *
 * `fijados`, cuando esta puesto, manda sobre el valor leido del servidor: es
 * el mecanismo que le deja a la bandeja empujar su conteo optimista tras
 * resolver un caso, sin depender de un segundo round-trip a este mismo
 * endpoint.
 */
export function usePendientesDeIdentificacion(habilitado: boolean): {
  pendientes: number | undefined;
  contexto: ContextoIdentificacion;
} {
  const recurso = useRecurso<unknown>(RUTA_CONTEO_PENDIENTES, habilitado);
  const [fijados, setFijados] = useState<number | undefined>(undefined);

  const leido =
    recurso.tipo === "listo" && esConteoValido(recurso.datos)
      ? recurso.datos.pendientes
      : undefined;

  const pendientes = fijados ?? leido;

  const fijarPendientes = useCallback((valor: number) => {
    setFijados(valor);
  }, []);

  // Estable via `useMemo`: un Outlet que recibe un objeto nuevo en cada
  // render de `Layout` (por ejemplo, por el menu del perfil) desmontaria y
  // remontaria sus rutas hijas si algo aguas abajo dependiera de la
  // identidad de este contexto.
  const contexto = useMemo<ContextoIdentificacion>(
    () => ({ pendientes, fijarPendientes }),
    [pendientes, fijarPendientes],
  );

  return { pendientes, contexto };
}

/**
 * Para que una ruta hija del shell (la bandeja) empuje su propio conteo de
 * pendientes al badge, sin esperar a que este vuelva a pedir el recurso.
 *
 * Sin `Outlet` de por medio -fuera del shell, o en un test que monta el
 * componente solo- `useOutletContext` no tiene nada que devolver: la funcion
 * resultante no hace nada, en vez de reventar.
 */
export function useFijarPendientes(): (valor: number) => void {
  const contexto = useOutletContext<ContextoIdentificacion | undefined>();
  return contexto?.fijarPendientes ?? (() => {});
}
