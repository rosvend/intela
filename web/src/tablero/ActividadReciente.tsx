import { CSSProperties } from "react";
import { Link } from "react-router-dom";
import { Asiento, esAsiento, familiaDeHecho } from "../auditoria/tipos";
import { describirAsiento } from "./actividad";
import { formatearInstante, tiempoRelativo } from "./formato";
import { Panel } from "./Panel";
import { Recurso } from "./tipos";

export function ActividadReciente({
  asientos,
}: {
  asientos: Recurso<Asiento[]>;
}) {
  const ahora = new Date();
  return (
    <Panel
      titulo="Actividad reciente"
      className="staff-actividad"
      recurso={asientos}
    >
      {(lista) => {
        const validos = (Array.isArray(lista) ? lista : [])
          .filter(esAsiento)
          .slice(0, 5);
        return (
          <>
            {validos.length === 0 ? (
              <p className="muted panel-vacio">Todavía no hay movimientos.</p>
            ) : (
              <ol className="staff-actividad-lista">
                {validos.map((a, i) => (
                  <li
                    key={a.id}
                    style={{ animationDelay: `${i * 50}ms` } as CSSProperties}
                  >
                    <span
                      className={`staff-actividad-punto staff-familia-${familiaDeHecho(a.hecho)}`}
                      aria-hidden="true"
                    />
                    <span className="staff-actividad-texto">
                      {describirAsiento(a)}
                      <span className="leyenda-sub">
                        <time
                          dateTime={a.cuando}
                          title={formatearInstante(a.cuando)}
                        >
                          {tiempoRelativo(a.cuando, ahora)}
                        </time>{" "}
                        · {a.actor}
                      </span>
                    </span>
                  </li>
                ))}
              </ol>
            )}
            <footer className="panel-pie">
              <span>Cada cifra se rastrea hasta su origen</span>
              <Link className="tarjeta-enlace" to="/auditoria">
                Ver bitácora
              </Link>
            </footer>
          </>
        );
      }}
    </Panel>
  );
}
