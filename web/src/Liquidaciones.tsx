import { useCallback, useEffect, useState } from "react";
import { api, descargar } from "./api";
import EsqueletoFilas from "./titular/Esqueleto";
import { nombrePeriodo } from "./titular/presentacion";
import Cifra from "./ui/Cifra";
import { formatearCOP } from "./ui/dinero";
import "./ui/ui.css";
import "./titular.css";

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

export const RUTA_MIS_LIQUIDACIONES = "/api/mis-liquidaciones/obras";

function queryPeriodo(periodo: string) {
  const p = periodo.trim();
  return p ? `periodo=${encodeURIComponent(p)}` : "";
}

/** "Mis liquidaciones" (titular): neto por obra, descuentos de ley y exportacion PDF/XLSX. */
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
      `${RUTA_MIS_LIQUIDACIONES}${q ? `?${q}` : ""}`,
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

  const hayLineas = !!liq && liq.lineas.length > 0;

  return (
    <section className="titular">
      <header className="titular-saludo">
        <h1>Mis liquidaciones</h1>
        <p>
          Lo que recibiste por cada obra y los descuentos de ley. El archivo
          descargado lleva las mismas cifras.
        </p>
      </header>

      <div className="liquidaciones-barra">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            setFiltro(periodo.trim());
          }}
        >
          <input
            className="pastilla-campo"
            aria-label="Periodo"
            name="periodo"
            placeholder="AAAA-MM"
            value={periodo}
            onChange={(e) => setPeriodo(e.target.value)}
          />
          <button type="submit" className="boton-secundario">
            Filtrar
          </button>
        </form>
        {hayLineas && (
          <div className="liquidaciones-exportar">
            <button
              type="button"
              className="boton-secundario"
              onClick={() => exportar("pdf")}
              disabled={!!exportando}
            >
              Descargar PDF
            </button>
            <button
              type="button"
              className="boton-secundario"
              onClick={() => exportar("xlsx")}
              disabled={!!exportando}
            >
              Descargar Excel
            </button>
          </div>
        )}
      </div>

      {error && (
        <p role="alert" className="titular-error">
          {error}
        </p>
      )}

      {!liq ? (
        error ? null : (
          <article className="panel">
            <EsqueletoFilas />
          </article>
        )
      ) : !hayLineas ? (
        <article className="panel">
          <div className="vacio">
            <p className="vacio-titulo">Todavía no tienes liquidaciones</p>
            <p className="muted">
              {filtro
                ? `No hay pagos en ${nombrePeriodo(filtro)}. Prueba con otro periodo.`
                : "Cuando REDES SGC reparta un periodo en el que se usaron tus obras, aparecerá aquí."}
            </p>
          </div>
        </article>
      ) : (
        <article className="panel">
          <header className="panel-cabecera">
            <h2 className="panel-titulo">
              {filtro
                ? `Recibiste en ${nombrePeriodo(filtro)}`
                : "Recibiste en total"}
            </h2>
          </header>
          <Cifra valor={liq.totales.neto} />
          <p className="liquidacion-desglose">
            <span>
              De {formatearCOP(liq.totales.bruto)} antes de descuentos
            </span>
          </p>
          <ul className="ingresos-lista liquidaciones-lista">
            {liq.lineas.map((l, i) => (
              <li
                key={`${l.periodo}-${l.obra_id}`}
                className="liquidacion-fila"
                style={{ "--indice": i } as React.CSSProperties}
              >
                <span className="ingreso-obra">
                  <span className="ingreso-titulo">{l.titulo}</span>
                  <span className="ingreso-sub">
                    {nombrePeriodo(l.periodo)}
                  </span>
                </span>
                <span className="ingreso-monto">
                  {formatearCOP(l.neto)}
                  <span className="ingreso-sub ingreso-sub-bloque">
                    de {formatearCOP(l.bruto)}
                  </span>
                </span>
                <p className="liquidacion-desglose">
                  <span>Gastos administrativos {formatearCOP(l.admin)}</span>
                  <span>Bienestar social {formatearCOP(l.social)}</span>
                  <span>
                    Reserva para errores técnicos {formatearCOP(l.reserva)}
                  </span>
                </p>
              </li>
            ))}
          </ul>
        </article>
      )}
    </section>
  );
}
