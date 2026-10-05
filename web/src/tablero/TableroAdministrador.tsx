import { Link } from "react-router-dom";
import {
  ArrowsRightLeftIcon,
  DocumentMagnifyingGlassIcon,
  InboxArrowDownIcon,
  LockClosedIcon,
  PlusIcon,
} from "@heroicons/react/24/outline";
import { puedeVer } from "../navegacion";
import { ETIQUETA_ETAPA } from "../reparto/etapas";
import { Usuario } from "../sesion";
import { ActividadReciente } from "./ActividadReciente";
import { AlertasDelPeriodo } from "./AlertasDelPeriodo";
import { DistribucionEnCurso } from "./DistribucionEnCurso";
import { RecaudoPorFuente } from "./RecaudoPorFuente";
import { Tarjeta } from "./Tarjeta";
import { formatearEntero } from "./formato";
import { useDashboard } from "./useDashboard";
import "../staff.css";

const HOY = new Intl.DateTimeFormat("es-CO", {
  weekday: "long",
  day: "numeric",
  month: "long",
});

function primerNombre(nombre: string): string {
  return nombre.trim().split(/\s+/)[0] ?? "";
}

function etiquetaDeEtapa(etapa: string): string {
  return (ETIQUETA_ETAPA as Record<string, string>)[etapa] ?? etapa;
}

export default function TableroAdministrador({
  usuario,
}: {
  usuario: Usuario;
}) {
  const tablero = useDashboard(usuario.rol);
  const ve = (ruta: string) => puedeVer(usuario.rol, ruta);
  const nombre = primerNombre(usuario.nombre);
  const hoy = HOY.format(new Date());

  return (
    <section className="tablero staff">
      <header className="tablero-cabecera staff-cabecera">
        <div>
          <h1>{nombre ? `Hola, ${nombre}` : "Hola"}</h1>
          <p className="muted staff-contexto">
            {tablero.periodo
              ? `Periodo ${tablero.periodo} · ${hoy}`
              : `Sin distribución abierta · ${hoy}`}
          </p>
        </div>
        {ve("/ingesta") && (
          <Link className="boton-primario staff-pill" to="/ingesta">
            <PlusIcon className="boton-primario-icono" aria-hidden="true" />
            Nueva ingesta
          </Link>
        )}
      </header>

      <div className="tablero-kpis staff-kpis">
        <Tarjeta
          titulo="Cargas por procesar"
          icono={<InboxArrowDownIcon />}
          to={ve("/ingesta") ? "/ingesta" : undefined}
          etiquetaEnlace="Ir a ingesta"
          recurso={tablero.cargasPendientes}
        >
          {(d) => <p className="tarjeta-valor">{formatearEntero(d.total)}</p>}
        </Tarjeta>

        <Tarjeta
          titulo="Obras en reserva"
          icono={<LockClosedIcon />}
          ayuda="Los porcentajes de la declaración de obra no suman 100%. Mientras no se corrija, no se reparte nada de esa obra: el total queda en reserva (RD 13.1.3)."
          to={ve("/catalogo") ? "/catalogo" : undefined}
          etiquetaEnlace="Ir al catálogo"
          recurso={tablero.obrasEnReserva}
        >
          {(d) => <p className="tarjeta-valor">{formatearEntero(d.total)}</p>}
        </Tarjeta>

        <Tarjeta
          titulo="Obras sin identificar"
          icono={<DocumentMagnifyingGlassIcon />}
          to={ve("/anomalias") ? "/anomalias" : undefined}
          etiquetaEnlace="Ir a anomalías"
          recurso={tablero.oni}
        >
          {(d) => <p className="tarjeta-valor">{formatearEntero(d.total)}</p>}
        </Tarjeta>

        <Tarjeta
          titulo="Última distribución"
          icono={<ArrowsRightLeftIcon />}
          to={ve("/distribucion") ? "/distribucion" : undefined}
          etiquetaEnlace="Ir a distribución"
          recurso={tablero.ultimaCorrida}
        >
          {(d) => (
            <p className="tarjeta-valor tarjeta-valor-texto">
              {etiquetaDeEtapa(d.etapa)}
            </p>
          )}
        </Tarjeta>
      </div>

      <div className="staff-rejilla">
        <RecaudoPorFuente bolsas={tablero.bolsas} periodo={tablero.periodo} />
        <DistribucionEnCurso
          procesos={tablero.procesos}
          enlace={ve("/distribucion")}
        />
        {ve("/anomalias") && (
          <AlertasDelPeriodo
            resumen={tablero.resumenAlertas}
            periodo={tablero.periodo}
          />
        )}
        {ve("/auditoria") && <ActividadReciente asientos={tablero.asientos} />}
      </div>
    </section>
  );
}
