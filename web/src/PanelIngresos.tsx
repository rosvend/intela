import { useEffect, useId, useMemo, useRef, useState } from "react";
import { api } from "./api";
import {
  formatearNeto,
  opcionesFiltro,
  rutaExplicar,
  rutaMisIngresos,
  type Explicacion,
  type FiltroIngresos,
  type Ingreso,
  type ListaIngresos,
} from "./ingresos";
import { useSesion } from "./sesion";
import EsqueletoFilas from "./titular/Esqueleto";
import HistoriaCifra from "./titular/HistoriaCifra";
import {
  nombreFuente,
  nombreFuentes,
  nombrePeriodo,
} from "./titular/presentacion";
import "./titular.css";

/**
 * Panel del titular (OE-6): ingresos netos por obra, fuente y periodo. Cada
 * fila se despliega en la historia de ExplicarCifra. El bruto no es cifra de
 * cabecera: solo aparece dentro de la historia.
 *
 * El fichero se llama PanelIngresos y no Ingresos.tsx porque en macOS
 * Ingresos.tsx e ingresos.ts son el mismo path.
 */
export function PanelIngresos() {
  const { usuario } = useSesion();
  const tecnico = !!usuario && usuario.rol !== "titular";
  const [catalogo, setCatalogo] = useState<Ingreso[]>([]);
  const [visibles, setVisibles] = useState<Ingreso[] | null>(null);
  const [filtro, setFiltro] = useState<FiltroIngresos>({
    obra: "",
    fuente: "",
    periodo: "",
  });
  const [error, setError] = useState("");
  const [cargando, setCargando] = useState(true);
  const filtroVacio = !filtro.obra && !filtro.fuente && !filtro.periodo;

  useEffect(() => {
    let vigente = true;
    setCargando(true);
    (
      api(
        rutaMisIngresos({ obra: "", fuente: "", periodo: "" }),
      ) as Promise<ListaIngresos>
    )
      .then((r) => {
        if (vigente) setCatalogo(r.ingresos);
      })
      .catch((e: Error) => vigente && setError(e.message))
      .finally(() => vigente && setCargando(false));
    return () => {
      vigente = false;
    };
  }, []);

  useEffect(() => {
    if (filtroVacio) {
      setVisibles(null);
      return;
    }
    let vigente = true;
    setCargando(true);
    (api(rutaMisIngresos(filtro)) as Promise<ListaIngresos>)
      .then((r) => {
        if (vigente) setVisibles(r.ingresos);
      })
      .catch((e: Error) => vigente && setError(e.message))
      .finally(() => vigente && setCargando(false));
    return () => {
      vigente = false;
    };
  }, [filtro, filtroVacio]);

  const filas = visibles ?? catalogo;
  const opciones = useMemo(() => opcionesFiltro(catalogo), [catalogo]);

  return (
    <section className="ingresos" aria-labelledby="panel-ingresos-titulo">
      <header className="panel-cabecera ingresos-cabecera">
        <div>
          <h2 id="panel-ingresos-titulo" className="panel-titulo">
            Mis ingresos
          </h2>
          <p className="muted ingresos-ayuda">
            Lo que recibiste por cada obra. Pulsa una fila para ver cómo se
            calculó.
          </p>
        </div>
      </header>
      {error ? (
        <p role="alert" className="titular-error">
          No se pudieron cargar tus ingresos: {error}
        </p>
      ) : (
        <>
          {catalogo.length > 0 && (
            <Filtros filtro={filtro} opciones={opciones} onChange={setFiltro} />
          )}
          {cargando ? (
            <EsqueletoFilas />
          ) : filas.length === 0 ? (
            <Vacio filtrando={!filtroVacio} />
          ) : (
            <TablaIngresos filas={filas} tecnico={tecnico} />
          )}
        </>
      )}
    </section>
  );
}

function Vacio({ filtrando }: { filtrando: boolean }) {
  return filtrando ? (
    <div className="vacio">
      <p className="vacio-titulo">Nada con esos filtros</p>
      <p className="muted">Prueba con otra obra, fuente o periodo.</p>
    </div>
  ) : (
    <div className="vacio">
      <p className="vacio-titulo">Aún no tienes ingresos liquidados</p>
      <p className="muted">
        Cuando REDES SGC reparta un periodo en el que se usaron tus obras, verás
        aquí cada pago y de dónde viene.
      </p>
    </div>
  );
}

function Filtros({
  filtro,
  opciones,
  onChange,
}: {
  filtro: FiltroIngresos;
  opciones: ReturnType<typeof opcionesFiltro>;
  onChange: (f: FiltroIngresos) => void;
}) {
  return (
    <div className="filtros-pastilla">
      <select
        className="pastilla-select"
        aria-label="Filtrar por obra"
        value={filtro.obra}
        onChange={(e) => onChange({ ...filtro, obra: e.target.value })}
      >
        <option value="">Todas las obras</option>
        {opciones.obras.map((o) => (
          <option key={o.id} value={o.id}>
            {o.titulo}
          </option>
        ))}
      </select>
      <select
        className="pastilla-select"
        aria-label="Filtrar por fuente"
        value={filtro.fuente}
        onChange={(e) => onChange({ ...filtro, fuente: e.target.value })}
      >
        <option value="">Todas las fuentes</option>
        {opciones.fuentes.map((f) => (
          <option key={f} value={f}>
            {nombreFuente(f)}
          </option>
        ))}
      </select>
      <select
        className="pastilla-select"
        aria-label="Filtrar por periodo"
        value={filtro.periodo}
        onChange={(e) => onChange({ ...filtro, periodo: e.target.value })}
      >
        <option value="">Todos los periodos</option>
        {opciones.periodos.map((p) => (
          <option key={p} value={p}>
            {nombrePeriodo(p)}
          </option>
        ))}
      </select>
    </div>
  );
}

export function TablaIngresos({
  filas,
  tecnico = false,
}: {
  filas: Ingreso[];
  tecnico?: boolean;
}) {
  const [abierta, setAbierta] = useState<string>("");
  const [explicacion, setExplicacion] = useState<Explicacion | null>(null);
  const [errorExplicar, setErrorExplicar] = useState("");
  const [cargandoExplicar, setCargandoExplicar] = useState(false);
  const pedido = useRef(0);

  function pedirExplicacion(ref: string) {
    const id = ++pedido.current;
    if (abierta === ref) {
      setAbierta("");
      setExplicacion(null);
      setErrorExplicar("");
      setCargandoExplicar(false);
      return;
    }
    setAbierta(ref);
    setExplicacion(null);
    setErrorExplicar("");
    setCargandoExplicar(true);
    (api(rutaExplicar(ref)) as Promise<Explicacion>)
      .then((r) => {
        if (pedido.current === id) setExplicacion(r);
      })
      .catch((e: Error) => {
        if (pedido.current === id) setErrorExplicar(e.message);
      })
      .finally(() => {
        if (pedido.current === id) setCargandoExplicar(false);
      });
  }

  return (
    <ul className="ingresos-lista">
      {filas.map((fila, i) => (
        <li
          key={fila.ref}
          className="ingresos-item"
          style={{ "--indice": i } as React.CSSProperties}
        >
          <FilaIngreso
            fila={fila}
            abierta={abierta === fila.ref}
            explicacion={abierta === fila.ref ? explicacion : null}
            error={abierta === fila.ref ? errorExplicar : ""}
            cargando={abierta === fila.ref && cargandoExplicar}
            tecnico={tecnico}
            onExplicar={() => pedirExplicacion(fila.ref)}
          />
        </li>
      ))}
    </ul>
  );
}

export function FilaIngreso({
  fila,
  abierta,
  explicacion,
  error,
  cargando,
  tecnico = false,
  onExplicar,
}: {
  fila: Ingreso;
  abierta: boolean;
  explicacion: Explicacion | null;
  error: string;
  cargando: boolean;
  tecnico?: boolean;
  onExplicar: () => void;
}) {
  const id = useId();
  return (
    <>
      <button
        type="button"
        className="ingreso-fila"
        aria-expanded={abierta}
        aria-controls={abierta ? id : undefined}
        onClick={onExplicar}
      >
        <span className="ingreso-obra">
          <span className="ingreso-titulo">{fila.obra}</span>
          <span className="ingreso-sub">
            {nombreFuentes(fila.fuente)} · {nombrePeriodo(fila.periodo)}
          </span>
        </span>
        <span className="ingreso-monto">{formatearNeto(fila.neto)}</span>
        <span className="ingreso-chevron" aria-hidden="true" />
      </button>
      {abierta && (
        <div id={id} className="desplegable">
          <div className="desplegable-interior">
            {cargando && <EsqueletoFilas filas={2} />}
            {error && (
              <p role="alert" className="titular-error">
                No pudimos explicar esta cifra: {error}
              </p>
            )}
            {explicacion && (
              <PanelExplicacion cifra={explicacion} tecnico={tecnico} />
            )}
          </div>
        </div>
      )}
    </>
  );
}

export function PanelExplicacion({
  cifra,
  tecnico = false,
}: {
  cifra: Explicacion;
  tecnico?: boolean;
}) {
  return <HistoriaCifra cifra={cifra} tecnico={tecnico} />;
}
