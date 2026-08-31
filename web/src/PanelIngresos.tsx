import { useEffect, useMemo, useState } from "react";
import { api } from "./api";
import {
  filtrarIngresos,
  formatearNeto,
  opcionesFiltro,
  rutaExplicar,
  rutaMisIngresos,
  type Explicacion,
  type FiltroIngresos,
  type Ingreso,
  type ListaIngresos,
} from "./ingresos";

/**
 * Panel del titular (OE-6): ingresos netos por obra, fuente y periodo.
 * Cada fila abre el linaje de ExplicarCifra. El bruto no es cifra de
 * cabecera: solo aparece dentro de la explicacion.
 *
 * El fichero se llama PanelIngresos y no Ingresos.tsx porque en macOS
 * Ingresos.tsx e ingresos.ts son el mismo path.
 */
export function PanelIngresos() {
  const [filas, setFilas] = useState<Ingreso[]>([]);
  const [filtro, setFiltro] = useState<FiltroIngresos>({
    obra: "",
    fuente: "",
    periodo: "",
  });
  const [error, setError] = useState("");
  const [cargando, setCargando] = useState(true);

  useEffect(() => {
    let vigente = true;
    setCargando(true);
    api(rutaMisIngresos({ obra: "", fuente: "", periodo: "" }))
      .then((r: ListaIngresos) => {
        if (vigente) setFilas(r.ingresos);
      })
      .catch((e: Error) => vigente && setError(e.message))
      .finally(() => vigente && setCargando(false));
    return () => {
      vigente = false;
    };
  }, []);

  const visibles = useMemo(
    () => filtrarIngresos(filas, filtro),
    [filas, filtro],
  );
  const opciones = useMemo(() => opcionesFiltro(filas), [filas]);

  if (error) {
    return (
      <section>
        <h1>Mis ingresos</h1>
        <p role="alert">No se pudieron cargar los ingresos: {error}</p>
      </section>
    );
  }

  return (
    <section>
      <h1>Mis ingresos</h1>
      <p className="muted">
        Montos netos despues de deducciones. El origen de cada cifra — fuente,
        reporte y regla — se abre con una pulsacion.
      </p>
      <Filtros filtro={filtro} opciones={opciones} onChange={setFiltro} />
      {cargando ? (
        <p>Consultando...</p>
      ) : visibles.length === 0 ? (
        <p className="muted">No hay ingresos con esos filtros.</p>
      ) : (
        <TablaIngresos filas={visibles} />
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

  function pedirExplicacion(ref: string) {
    if (abierta === ref) {
      setAbierta("");
      setExplicacion(null);
      setErrorExplicar("");
      return;
    }
    setAbierta(ref);
    setExplicacion(null);
    setErrorExplicar("");
    setCargandoExplicar(true);
    api(rutaExplicar(ref))
      .then((r: Explicacion) => {
        setExplicacion(r);
      })
      .catch((e: Error) => setErrorExplicar(e.message))
      .finally(() => setCargandoExplicar(false));
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
          {cifra.split.porcentaje}% · IPI {cifra.split.ipi} · declaracion v
          {cifra.split.version}
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
    </section>
  );
}
