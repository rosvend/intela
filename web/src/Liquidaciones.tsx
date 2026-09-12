import { useCallback, useEffect, useState } from "react";
import { api, descargar } from "./api";

export type TotalesLiquidacion = {
  bruto: string;
  admin: string;
  social: string;
  reserva: string;
  neto: string;
};

export type LineaLiquidacion = TotalesLiquidacion & {
  periodo: string;
  obra_id: string;
  titulo: string;
};

export type Liquidacion = {
  titular_id: string;
  periodo: string;
  lineas: LineaLiquidacion[];
  totales: TotalesLiquidacion;
};

function queryPeriodo(periodo: string) {
  const p = periodo.trim();
  return p ? `periodo=${encodeURIComponent(p)}` : "";
}

export default function Liquidaciones() {
  const [periodo, setPeriodo] = useState("");
  const [filtro, setFiltro] = useState("");
  const [liq, setLiq] = useState<Liquidacion | null>(null);
  const [error, setError] = useState("");
  const [exportando, setExportando] = useState("");

  const cargar = useCallback(async (p: string) => {
    setError("");
    const q = queryPeriodo(p);
    const data = (await api(
      `/api/mis-liquidaciones${q ? `?${q}` : ""}`,
    )) as Liquidacion;
    setLiq(data);
  }, []);

  useEffect(() => {
    let vigente = true;
    cargar(filtro).catch((e: Error) => vigente && setError(e.message));
    return () => {
      vigente = false;
    };
  }, [cargar, filtro]);

  async function exportar(formato: "pdf" | "xlsx") {
    setExportando(formato);
    setError("");
    const partes = [`formato=${formato}`];
    const q = queryPeriodo(filtro);
    if (q) partes.push(q);
    try {
      await descargar(`/api/mis-liquidaciones/export?${partes.join("&")}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "no se pudo exportar");
    } finally {
      setExportando("");
    }
  }

  return (
    <section>
      <h1>Mis liquidaciones</h1>
      <p className="muted">
        Bruto, deducciones y neto por obra. El archivo exportado lleva las
        mismas cifras y se puede abrir sin conexion.
      </p>

      <form
        className="card"
        onSubmit={(e) => {
          e.preventDefault();
          setFiltro(periodo.trim());
        }}
      >
        <label htmlFor="periodo">Periodo</label>{" "}
        <input
          id="periodo"
          name="periodo"
          placeholder="2026-01"
          value={periodo}
          onChange={(e) => setPeriodo(e.target.value)}
        />
        <button type="submit">Filtrar</button>{" "}
        <button
          type="button"
          onClick={() => exportar("pdf")}
          disabled={!!exportando}
        >
          Exportar PDF
        </button>{" "}
        <button
          type="button"
          onClick={() => exportar("xlsx")}
          disabled={!!exportando}
        >
          Exportar Excel
        </button>
      </form>

      {error ? <p role="alert">{error}</p> : null}

      {liq ? (
        <div className="card">
          <table>
            <thead>
              <tr>
                <th>Periodo</th>
                <th>Obra</th>
                <th>Bruto</th>
                <th>Admin</th>
                <th>Social</th>
                <th>Reserva</th>
                <th>Neto</th>
              </tr>
            </thead>
            <tbody>
              {liq.lineas.map((l) => (
                <tr key={`${l.periodo}-${l.obra_id}`}>
                  <td>{l.periodo}</td>
                  <td>{l.titulo}</td>
                  <td>{l.bruto}</td>
                  <td>{l.admin}</td>
                  <td>{l.social}</td>
                  <td>{l.reserva}</td>
                  <td>{l.neto}</td>
                </tr>
              ))}
            </tbody>
            <tfoot>
              <tr>
                <th colSpan={2}>Totales</th>
                <th>{liq.totales.bruto}</th>
                <th>{liq.totales.admin}</th>
                <th>{liq.totales.social}</th>
                <th>{liq.totales.reserva}</th>
                <th>{liq.totales.neto}</th>
              </tr>
            </tfoot>
          </table>
        </div>
      ) : error ? null : (
        <p>Consultando...</p>
      )}
    </section>
  );
}
