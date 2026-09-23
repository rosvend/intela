import { useId, useState } from "react";
import { Link } from "react-router-dom";
import Cargando from "../Cargando";
import { useLista } from "../catalogo/useLista";
import LineaTiempo from "./LineaTiempo";
import {
  ETIQUETA_FAMILIA,
  FILTROS_VACIOS,
  RUTAS_AUDITORIA,
  esAsiento,
  filtrarAsientos,
  type FamiliaHecho,
  type Filtros,
} from "./tipos";

const LIMITE_TIMELINE = 100;

const FAMILIAS: readonly (FamiliaHecho | "todas")[] = [
  "todas",
  "catalogo",
  "splits",
  "distribucion",
  "recaudo",
  "identificacion",
  "otro",
];

/**
 * Portal de Auditoria (OE-7): la linea de tiempo de los asientos, filtrable
 * por tipo, fecha, actor y obra.
 *
 * Solo lectura: ni un control de edicion ni uno de borrado en toda la
 * pantalla. La bitacora es append-only por construccion -la tabla rechaza
 * UPDATE y DELETE por trigger (`asientos_inmutables`, ADR 0006)- y la vista
 * lo dice en vez de prometerlo con codigo.
 *
 * Los filtros se aplican en el cliente sobre la pagina que trajo el
 * servidor: la API no filtra (alcance minimo de este PR). La pagina son los
 * 100 asientos mas recientes.
 */
export default function Auditoria() {
  const [filtros, setFiltros] = useState<Filtros>(FILTROS_VACIOS);
  const [obraParaHistoria, setObraParaHistoria] = useState("");

  const idFamilia = useId();
  const idDesde = useId();
  const idHasta = useId();
  const idActor = useId();
  const idObra = useId();
  const idHistoria = useId();

  const lista = useLista(
    `${RUTAS_AUDITORIA.asientos}?limite=${LIMITE_TIMELINE}`,
    esAsiento,
  );

  function aplicar<K extends keyof Filtros>(campo: K, valor: Filtros[K]) {
    setFiltros((anteriores) => ({ ...anteriores, [campo]: valor }));
  }

  function limpiarFiltros() {
    setFiltros(FILTROS_VACIOS);
  }

  return (
    <section className="auditoria">
      <header className="auditoria-cabecera">
        <div>
          <h1>Auditoría</h1>
          <p className="muted">
            Historia de cambios del catálogo, los repartos entre autores y las
            distribuciones, con la evidencia de origen de cada cifra. Los{" "}
            {LIMITE_TIMELINE} asientos más recientes.
          </p>
        </div>
      </header>

      <p className="auditoria-nota">
        Libro append-only: los asientos no se pueden modificar ni eliminar
        (trigger{" "}
        <span className="detalle-identificador">asientos_inmutables</span>, ADR
        0006). Corregir es asentar de nuevo, nunca reescribir.
      </p>

      <form
        className="auditoria-filtros"
        onSubmit={(evento) => evento.preventDefault()}
      >
        <div className="auditoria-campos">
          <div className="auditoria-campo">
            <label htmlFor={idFamilia}>Tipo</label>
            <select
              id={idFamilia}
              value={filtros.familia}
              onChange={(evento) =>
                aplicar("familia", evento.target.value as Filtros["familia"])
              }
            >
              {FAMILIAS.map((familia) => (
                <option key={familia} value={familia}>
                  {familia === "todas" ? "Todos" : ETIQUETA_FAMILIA[familia]}
                </option>
              ))}
            </select>
          </div>
          <div className="auditoria-campo">
            <label htmlFor={idDesde}>Desde</label>
            <input
              id={idDesde}
              type="date"
              value={filtros.desde}
              onChange={(evento) => aplicar("desde", evento.target.value)}
            />
          </div>
          <div className="auditoria-campo">
            <label htmlFor={idHasta}>Hasta</label>
            <input
              id={idHasta}
              type="date"
              value={filtros.hasta}
              onChange={(evento) => aplicar("hasta", evento.target.value)}
            />
          </div>
          <div className="auditoria-campo">
            <label htmlFor={idActor}>Actor</label>
            <input
              id={idActor}
              type="text"
              placeholder="usr-admin"
              value={filtros.actor}
              onChange={(evento) => aplicar("actor", evento.target.value)}
            />
          </div>
          <div className="auditoria-campo">
            <label htmlFor={idObra}>Obra</label>
            <input
              id={idObra}
              type="text"
              placeholder="obra-1"
              value={filtros.obra}
              onChange={(evento) => aplicar("obra", evento.target.value)}
            />
          </div>
        </div>
        <button
          type="button"
          className="catalogo-limpiar"
          onClick={limpiarFiltros}
        >
          Limpiar filtros
        </button>
      </form>

      <form
        className="auditoria-historia"
        onSubmit={(evento) => evento.preventDefault()}
      >
        <label htmlFor={idHistoria}>Historia de una obra concreta</label>
        <div className="auditoria-historia-fila">
          <input
            id={idHistoria}
            type="text"
            placeholder="obra-1"
            value={obraParaHistoria}
            onChange={(evento) => setObraParaHistoria(evento.target.value)}
          />
          {obraParaHistoria.trim() !== "" && (
            <Link
              to={`/auditoria/obra/${encodeURIComponent(obraParaHistoria.trim())}`}
            >
              Ver historia
            </Link>
          )}
        </div>
      </form>

      {lista.estado === "cargando" && <Cargando texto="Cargando asientos…" />}
      {lista.estado === "error" && (
        <p className="catalogo-error" role="alert">
          {lista.mensaje}
        </p>
      )}
      {lista.estado === "ilegible" && (
        <p className="catalogo-error" role="alert">
          La bitácora no llegó en un formato legible.
        </p>
      )}
      {lista.estado === "ok" && (
        <Contenido
          asientos={filtrarAsientos(lista.elementos, filtros)}
          total={lista.elementos.length}
        />
      )}
    </section>
  );
}

function Contenido({
  asientos,
  total,
}: {
  asientos: ReturnType<typeof filtrarAsientos>;
  total: number;
}) {
  if (total === 0) {
    return (
      <p className="muted">
        Sin asientos todavía. La bitácora se llena a medida que el sistema
        registra recaudo, declaraciones y repartos.
      </p>
    );
  }
  if (asientos.length === 0) {
    return <p className="muted">Ningún asiento cuadra con esos filtros.</p>;
  }
  return (
    <>
      <p className="catalogo-resumen" role="status">
        {asientos.length} de {total} asientos.
      </p>
      <LineaTiempo asientos={asientos} />
    </>
  );
}
