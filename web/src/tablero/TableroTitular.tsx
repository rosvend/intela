import { ArrowRightIcon } from "@heroicons/react/20/solid";
import { Link } from "react-router-dom";
import { RUTA_MIS_LIQUIDACIONES, type Liquidacion } from "../Liquidaciones";
import { PanelIngresos } from "../PanelIngresos";
import { citaDeRetencion } from "../reglamento";
import { Usuario } from "../sesion";
import EstadoRecurso from "../titular/EstadoRecurso";
import {
  agruparPorObra,
  estadoDeObra,
  nombrePeriodo,
  primerNombre,
} from "../titular/presentacion";
import BarraApilada, { colorDe } from "../ui/BarraApilada";
import Cifra from "../ui/Cifra";
import Detalle from "../ui/Detalle";
import { formatearCOP } from "../ui/dinero";
import type { MisObras, UltimaLiquidacion } from "./tipos";
import { useDashboard, useRecurso } from "./useDashboard";
import "../ui/ui.css";
import "../titular.css";

/**
 * Inicio del titular: cuanto gano y como lo gano. Heroe con la ultima
 * liquidacion, reparto por obra, estado de cada declaracion y, debajo, el
 * panel de ingresos con la historia de cada cifra (#ingresos, M-5).
 */
export default function TableroTitular({ usuario }: { usuario: Usuario }) {
  const tablero = useDashboard(usuario.rol);
  const liquidaciones = useRecurso<Liquidacion>(RUTA_MIS_LIQUIDACIONES);
  const nombre = primerNombre(usuario.nombre);

  return (
    <section className="titular">
      <header className="titular-saludo">
        <h1>{nombre ? `Hola, ${nombre}` : "Hola"}</h1>
      </header>

      <div className="titular-rejilla">
        <article className="panel titular-heroe">
          <h2 className="titular-eyebrow">Tu última liquidación</h2>
          <EstadoRecurso
            recurso={tablero.ultimaLiquidacion}
            esqueleto={<EsqueletoCifra />}
            vacio={{
              titulo: "Tu primera liquidación aparecerá aquí",
              texto:
                "REDES SGC reparte por periodos. Cuando se liquide uno en el que se usaron tus obras, verás aquí lo que recibes.",
            }}
          >
            {(datos) => <Heroe datos={datos} />}
          </EstadoRecurso>
        </article>

        <article className="panel">
          <header className="panel-cabecera">
            <h2 className="panel-titulo">Ingresos por obra</h2>
          </header>
          <EstadoRecurso
            recurso={liquidaciones}
            esqueleto={<EsqueletoCifra />}
            vacio={{
              titulo: "Aún no hay ingresos por obra",
              texto:
                "Cuando recibas tu primer pago verás aquí cuánto aportó cada obra.",
            }}
          >
            {(datos) => <IngresosPorObra datos={datos} />}
          </EstadoRecurso>
        </article>

        <article className="panel">
          <header className="panel-cabecera">
            <h2 className="panel-titulo">Mis obras</h2>
          </header>
          <EstadoRecurso
            recurso={tablero.misObras}
            esqueleto={<EsqueletoCifra />}
            vacio={{
              titulo: "Aún no hay obras a tu nombre",
              texto:
                "Las obras aparecen aquí cuando su Declaración de Obra te incluye como escritor.",
            }}
          >
            {(datos) => <ListaObras datos={datos} />}
          </EstadoRecurso>
        </article>
      </div>

      <article className="panel" id="ingresos">
        <PanelIngresos />
      </article>
    </section>
  );
}

function EsqueletoCifra() {
  return (
    <div className="esqueleto-lista">
      <span className="esqueleto esqueleto-cifra" />
      <span className="esqueleto esqueleto-linea" />
    </div>
  );
}

function Heroe({ datos }: { datos: UltimaLiquidacion }) {
  return (
    <>
      <Cifra valor={datos.neto} />
      <ul className="titular-meta">
        <li className="chip">{nombrePeriodo(datos.periodo)}</li>
        {datos.obras > 0 && (
          <li className="chip">
            {datos.obras === 1 ? "1 obra" : `${datos.obras} obras`}
          </li>
        )}
      </ul>
    </>
  );
}

function IngresosPorObra({ datos }: { datos: Liquidacion }) {
  const obras = agruparPorObra(datos.lineas);
  if (obras.length === 0) {
    return (
      <div className="vacio">
        <p className="vacio-titulo">Aún no hay ingresos por obra</p>
        <p className="muted">
          Cuando recibas tu primer pago verás aquí cuánto aportó cada obra.
        </p>
      </div>
    );
  }
  return (
    <>
      <p className="titular-total">
        {formatearCOP(datos.totales.neto)}
        <span className="titular-total-sub">Recibido en total</span>
      </p>
      <BarraApilada
        etiqueta="Ingresos por obra"
        segmentos={obras.map((o) => ({
          id: o.obra_id,
          etiqueta: o.titulo,
          valor: o.neto,
        }))}
      />
      <ul className="leyenda">
        {obras.map((o, i) => (
          <li key={o.obra_id} className="leyenda-fila">
            <span
              className="leyenda-punto"
              style={{ "--color": colorDe(i) } as React.CSSProperties}
            />
            <span>{o.titulo}</span>
            <span className="leyenda-cifra">{formatearCOP(o.neto)}</span>
          </li>
        ))}
      </ul>
      <footer className="panel-pie">
        <span>
          {datos.lineas.length === 1
            ? "Basado en 1 liquidación"
            : `Basado en ${datos.lineas.length} liquidaciones`}
        </span>
        <Link to="/mis-liquidaciones" className="boton-secundario boton-cta">
          Ver mis liquidaciones
          <ArrowRightIcon aria-hidden="true" />
        </Link>
      </footer>
    </>
  );
}

function ListaObras({ datos }: { datos: MisObras }) {
  if (datos.obras.length === 0) {
    return (
      <div className="vacio">
        <p className="vacio-titulo">Aún no hay obras a tu nombre</p>
        <p className="muted">
          Las obras aparecen aquí cuando su Declaración de Obra te incluye como
          escritor.
        </p>
      </div>
    );
  }
  const cita = citaDeRetencion();
  return (
    <ul className="leyenda obras-lista">
      {datos.obras.map((o) => {
        const estado = estadoDeObra(o.estado);
        return (
          <li key={o.id} className="leyenda-fila">
            <span>{o.titulo}</span>
            {estado.tipo === "reserva" ? (
              <Detalle
                disparador={estado.etiqueta}
                titulo="¿Por qué está en reserva?"
                claseDisparador="chip chip-alerta chip-boton"
              >
                <span className="detalle-texto">
                  Los porcentajes de la Declaración de Obra deben sumar 100 %
                  entre todos sus escritores. Mientras no sea así, REDES SGC
                  guarda todo el pago de la obra y no reparte una parte.
                  Escríbele a REDES SGC para completarla.
                </span>
                <span className="detalle-texto">
                  <strong>RD 13.1.3 · {cita.titulo}.</strong> {cita.texto}
                </span>
              </Detalle>
            ) : (
              <span
                className={estado.tipo === "lista" ? "chip chip-ok" : "chip"}
              >
                {estado.etiqueta}
              </span>
            )}
          </li>
        );
      })}
    </ul>
  );
}
