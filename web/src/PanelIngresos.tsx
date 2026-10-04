import { useEffect, useMemo, useRef, useState } from "react";
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
import {
  citarReglamento,
  citaDeConcepto,
  citaDeRetencion,
  nombreConcepto,
} from "./reglamento";
import { lineaDeValorizacion, USOS_VISIBLES } from "./valorizacion";

/**
 * Panel del titular (OE-6): ingresos netos por obra, fuente y periodo.
 * Cada fila abre el linaje de ExplicarCifra. El bruto no es cifra de
 * cabecera: solo aparece dentro de la explicacion.
 *
 * El fichero se llama PanelIngresos y no Ingresos.tsx porque en macOS
 * Ingresos.tsx e ingresos.ts son el mismo path.
 */
export function PanelIngresos() {
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

  if (error) {
    return (
      <section aria-labelledby="panel-ingresos-titulo">
        <h2 id="panel-ingresos-titulo" className="tarjeta-etiqueta">
          Mis ingresos
        </h2>
        <p role="alert">No se pudieron cargar los ingresos: {error}</p>
      </section>
    );
  }

  return (
    <section aria-labelledby="panel-ingresos-titulo">
      <h2 id="panel-ingresos-titulo" className="tarjeta-etiqueta">
        Mis ingresos
      </h2>
      <p className="muted">
        Montos netos despues de deducciones. El origen de cada cifra — fuente,
        reporte y regla — se abre con una pulsacion.
      </p>
      <Filtros filtro={filtro} opciones={opciones} onChange={setFiltro} />
      {cargando ? (
        <p>Consultando...</p>
      ) : filas.length === 0 ? (
        <p className="muted">No hay ingresos con esos filtros.</p>
      ) : (
        <TablaIngresos filas={filas} />
      )}
    </section>
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
    <div className="filtros">
      <label>
        Obra
        <select
          aria-label="Filtrar por obra"
          value={filtro.obra}
          onChange={(e) => onChange({ ...filtro, obra: e.target.value })}
        >
          <option value="">Todas</option>
          {opciones.obras.map((o) => (
            <option key={o.id} value={o.id}>
              {o.titulo}
            </option>
          ))}
        </select>
      </label>
      <label>
        Fuente
        <select
          aria-label="Filtrar por fuente"
          value={filtro.fuente}
          onChange={(e) => onChange({ ...filtro, fuente: e.target.value })}
        >
          <option value="">Todas</option>
          {opciones.fuentes.map((f) => (
            <option key={f} value={f}>
              {f}
            </option>
          ))}
        </select>
      </label>
      <label>
        Periodo
        <select
          aria-label="Filtrar por periodo"
          value={filtro.periodo}
          onChange={(e) => onChange({ ...filtro, periodo: e.target.value })}
        >
          <option value="">Todos</option>
          {opciones.periodos.map((p) => (
            <option key={p} value={p}>
              {p}
            </option>
          ))}
        </select>
      </label>
    </div>
  );
}

export function TablaIngresos({ filas }: { filas: Ingreso[] }) {
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
        if (pedido.current !== id) return;
        setExplicacion(r);
      })
      .catch((e: Error) => {
        if (pedido.current !== id) return;
        setErrorExplicar(e.message);
      })
      .finally(() => {
        if (pedido.current !== id) return;
        setCargandoExplicar(false);
      });
  }

  return (
    <table>
      <thead>
        <tr>
          <th>Obra</th>
          <th>Fuente</th>
          <th>Periodo</th>
          <th>Neto</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        {filas.map((fila) => (
          <FilaIngreso
            key={fila.ref}
            fila={fila}
            abierta={abierta === fila.ref}
            explicacion={abierta === fila.ref ? explicacion : null}
            error={abierta === fila.ref ? errorExplicar : ""}
            cargando={abierta === fila.ref && cargandoExplicar}
            onExplicar={() => pedirExplicacion(fila.ref)}
          />
        ))}
      </tbody>
    </table>
  );
}

export function FilaIngreso({
  fila,
  abierta,
  explicacion,
  error,
  cargando,
  onExplicar,
}: {
  fila: Ingreso;
  abierta: boolean;
  explicacion: Explicacion | null;
  error: string;
  cargando: boolean;
  onExplicar: () => void;
}) {
  return (
    <>
      <tr>
        <td>{fila.obra}</td>
        <td>{fila.fuente}</td>
        <td>{fila.periodo}</td>
        <td className="neto">{formatearNeto(fila.neto)}</td>
        <td>
          <button type="button" onClick={onExplicar}>
            {abierta ? "Ocultar" : "Explicar esta cifra"}
          </button>
        </td>
      </tr>
      {abierta && (
        <tr>
          <td colSpan={5}>
            {cargando && <p>Cargando linaje...</p>}
            {error && (
              <p role="alert">No se pudo explicar esta cifra: {error}</p>
            )}
            {explicacion && <PanelExplicacion cifra={explicacion} />}
          </td>
        </tr>
      )}
    </>
  );
}

export function PanelExplicacion({ cifra }: { cifra: Explicacion }) {
  const [detalles, setDetalles] = useState(false);
  return (
    <section
      className="explicacion card"
      role="region"
      aria-label="Explicacion de la cifra"
    >
      <p className="badge">Linaje de ExplicarCifra</p>
      <dl>
        <dt>Neto</dt>
        <dd className="neto">{formatearNeto(cifra.neto)}</dd>
        <dt>Bruto</dt>
        <dd>{formatearNeto(cifra.bruto)}</dd>
        <dt>Corrida</dt>
        <dd>
          {cifra.corrida.proceso_id} · {cifra.corrida.periodo} ·{" "}
          {cifra.corrida.circuito}
        </dd>
        <dt>Reporte de origen</dt>
        <dd>
          {cifra.reporte.fuente || "—"} · {cifra.reporte.id || "—"}
          {cifra.reporte.sha256 ? ` · ${cifra.reporte.sha256}` : ""}
        </dd>
        <dt>Obra y match</dt>
        <dd>
          {cifra.obra.titulo} · escalon {cifra.obra.escalon || "—"} · puntaje{" "}
          {cifra.obra.puntaje}
        </dd>
        <dt>Regla y snapshot</dt>
        <dd>
          {cifra.regla.reglamento} · snapshot {cifra.regla.snapshot_id}
        </dd>
        <dt>Split</dt>
        <dd>
          {cifra.split
            ? `${cifra.split.porcentaje}% · IPI ${cifra.split.ipi}${
                typeof cifra.split.version === "number"
                  ? ` · declaracion v${cifra.split.version}`
                  : ""
              }`
            : "—"}
        </dd>
      </dl>
      <h2>Deducciones (bruto a neto)</h2>
      {cifra.deducciones.length === 0 ? (
        <p className="muted">Sin deducciones registradas.</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>Concepto</th>
              <th>Porcentaje</th>
              <th>Monto</th>
            </tr>
          </thead>
          <tbody>
            {cifra.deducciones.map((d) => (
              <tr key={d.concepto}>
                <td>{d.concepto}</td>
                <td>{d.porcentaje}%</td>
                <td>{formatearNeto(d.monto)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <button
        type="button"
        className="boton-detalles"
        aria-expanded={detalles}
        onClick={() => setDetalles((d) => !d)}
      >
        {detalles ? "Ocultar mas detalles" : "Mas detalles"}
      </button>
      {detalles && <Recibo cifra={cifra} />}
    </section>
  );
}

/**
 * El "recibo": la misma Explicacion ya cargada, en prosa de bruto a neto y
 * con la cita de reglamento traducida (feedback del PO: nadie tiene el
 * reglamento memorizado). Tambien itemiza los puntos de cada uso (#187). No
 * pide datos nuevos -- ver web/src/reglamento.ts.
 */
function Recibo({ cifra }: { cifra: Explicacion }) {
  const [todos, setTodos] = useState(false);
  const citasRegla = citarReglamento(cifra.regla.reglamento);
  const citaRetencion = citaDeRetencion();
  return (
    <div className="recibo" aria-label="Recibo en lenguaje sencillo">
      <h3>Recibo</h3>
      <div className="recibo-puntos">
        <h4>Puntos de la obra</h4>
        {cifra.valorizacion.length === 0 ? (
          <p className="muted">
            Esta cifra se calculo antes de que se guardara el desglose por
            factor; se conserva solo el total: {cifra.obra.puntos} puntos.
          </p>
        ) : (
          <>
            <ul>
              {(todos
                ? cifra.valorizacion
                : cifra.valorizacion.slice(0, USOS_VISIBLES)
              ).map((v) => (
                <li key={v.uso_id}>{lineaDeValorizacion(v)}</li>
              ))}
            </ul>
            {cifra.valorizacion.length > USOS_VISIBLES && (
              <button
                type="button"
                className="boton-detalles"
                aria-expanded={todos}
                onClick={() => setTodos((t) => !t)}
              >
                {todos
                  ? "Ver menos"
                  : `Ver los ${cifra.valorizacion.length} usos`}
              </button>
            )}
            <p>
              Total de la obra: <strong>{cifra.obra.puntos} puntos</strong>
            </p>
          </>
        )}
      </div>
      <ol className="recibo-lineas">
        <li className="recibo-linea recibo-bruto">
          <span>Bruto</span>
          <span className="neto">{formatearNeto(cifra.bruto)}</span>
        </li>
        {cifra.deducciones.map((d) => {
          const cita = citaDeConcepto(d.concepto);
          return (
            <li key={d.concepto} className="recibo-linea recibo-deduccion">
              <p>
                {nombreConcepto(d.concepto)} ({d.concepto}): {d.porcentaje}% ={" "}
                {formatearNeto(d.monto)}
              </p>
              {cita && (
                <p className="muted recibo-cita">
                  <strong>{cita.titulo}.</strong> {cita.texto}
                </p>
              )}
            </li>
          );
        })}
        <li className="recibo-linea recibo-neto">
          <span>Neto</span>
          <span className="neto">{formatearNeto(cifra.neto)}</span>
        </li>
      </ol>

      {cifra.split && (
        <p className="recibo-split">
          Tu parte declarada: <strong>{cifra.split.porcentaje}%</strong> (IPI{" "}
          {cifra.split.ipi}
          {typeof cifra.split.version === "number"
            ? `, declaracion v${cifra.split.version}`
            : ""}
          )
        </p>
      )}

      {cifra.retenida && (
        <div className="recibo-retencion alerta">
          <p>
            Esta obra se retuvo por completo
            {cifra.motivo ? `: ${cifra.motivo}` : "."}
          </p>
          {citaRetencion && (
            <p className="recibo-cita">
              <strong>{citaRetencion.titulo}.</strong> {citaRetencion.texto}
            </p>
          )}
        </div>
      )}

      {citasRegla.length > 0 && (
        <div className="recibo-reglamento">
          <h4>Regla aplicada</h4>
          {citasRegla.map((c) => (
            <p key={c.token} className="recibo-cita">
              <strong>{c.titulo}</strong>
              {c.texto ? <> — {c.texto}</> : null}
            </p>
          ))}
        </div>
      )}

      {cifra.firmas.length > 0 && (
        <div className="recibo-firmas">
          <h4>Firmas</h4>
          <ul>
            {cifra.firmas.map((f) => (
              <li key={`${f.rol}-${f.etapa}-${f.sobre_revision}-${f.cuando}`}>
                {f.rol} · {f.actor_id} · {f.etapa} · {f.cuando.slice(0, 10)}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
