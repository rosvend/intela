import { ArrowRightIcon } from "@heroicons/react/20/solid";
import { Fragment, type CSSProperties, type ReactElement } from "react";
import "./fusion.css";
import { IconoCheck } from "./iconos";
import {
  compararTitulos,
  filasDeComparacion,
  nombreDeFuente,
  type FilaDeComparacion,
  type ObraComparada,
} from "./presentacion";
import type { CasoIdentificacion } from "./tipos";

/**
 * Las dos tarjetas de la union, lado a lado: lo reportado contra la obra del
 * catalogo. Es una sola rejilla de filas (no dos tarjetas sueltas) para que
 * cada campo quede a la misma altura en los dos lados aunque un valor ocupe
 * dos lineas; las tarjetas son solo el fondo de cada columna.
 */
export default function ComparacionFusion({
  caso,
  obra,
  unida,
}: {
  caso: CasoIdentificacion;
  obra: ObraComparada | null;
  unida: boolean;
}): ReactElement {
  const filas = filasDeComparacion(caso, obra);
  return (
    <div
      className={`fusion-comparacion${unida ? " fusion-unida" : ""}`}
      role="group"
      aria-label="Comparación campo por campo"
      style={{ "--filas": filas.length } as CSSProperties}
    >
      <div className="fusion-cabecera fusion-cabecera-reportado">
        Reportado por {nombreDeFuente(caso.fuente)}
      </div>
      <div className="fusion-flecha" aria-hidden="true">
        <span className="fusion-flecha-circulo">
          {unida ? <IconoCheck tamano={16} /> : <ArrowRightIcon />}
        </span>
      </div>
      <div className="fusion-cabecera fusion-cabecera-catalogo">
        Obra del catálogo
      </div>

      {filas.map((fila, i) => (
        <div className="fusion-fila" key={fila.campo}>
          <CeldaReportada
            fila={fila}
            caso={caso}
            obra={obra}
            indice={i}
            ultima={i === filas.length - 1}
          />
          {obra && (
            <CeldaCatalogo
              // La clave cambia con la obra: la celda se monta de nuevo y
              // entra animada al cambiar de candidata.
              key={`${obra.id}-${fila.campo}`}
              fila={fila}
              indice={i}
              ultima={i === filas.length - 1}
            />
          )}
        </div>
      ))}

      {!obra && (
        <div className="fusion-catalogo-vacio">
          <p>Elige una candidata o busca la obra para compararla.</p>
        </div>
      )}
    </div>
  );
}

function Valor({ texto }: { texto: string | null }): ReactElement {
  if (texto === null) {
    return <span className="fusion-valor fusion-valor-falta">—</span>;
  }
  return <span className="fusion-valor">{texto}</span>;
}

function CeldaReportada({
  fila,
  caso,
  obra,
  indice,
  ultima,
}: {
  fila: FilaDeComparacion;
  caso: CasoIdentificacion;
  obra: ObraComparada | null;
  indice: number;
  ultima: boolean;
}): ReactElement {
  const distinta = fila.estado === "distinto";
  const clases = [
    "fusion-celda",
    "fusion-celda-reportado",
    distinta && "fusion-distinta",
    ultima && "fusion-celda-ultima",
  ]
    .filter(Boolean)
    .join(" ");
  return (
    <div
      className={clases}
      data-campo={fila.campo}
      data-estado={fila.estado ?? undefined}
      // Fila 1: las cabeceras. Explicita para que las dos columnas casen.
      style={{ gridRow: indice + 2 }}
    >
      <span className="fusion-etiqueta">
        {fila.etiqueta}
        {distinta && <span className="solo-lector">, Distinto</span>}
      </span>
      {fila.campo === "titulo" ? (
        <>
          <h2 className="fusion-titulo">
            <TituloComparado texto={caso.titulo} contra={obra?.titulo} />
          </h2>
          {caso.titulo_original !== "" &&
            caso.titulo_original !== caso.titulo && (
              <span className="fusion-original">
                <TituloComparado
                  texto={caso.titulo_original}
                  contra={obra?.titulo}
                />
              </span>
            )}
        </>
      ) : (
        <Valor texto={fila.reportado} />
      )}
    </div>
  );
}

function CeldaCatalogo({
  fila,
  indice,
  ultima,
}: {
  fila: FilaDeComparacion;
  indice: number;
  ultima: boolean;
}): ReactElement {
  return (
    <div
      className={`fusion-celda fusion-celda-catalogo${ultima ? " fusion-celda-ultima" : ""}`}
      data-campo={fila.campo}
      data-estado={fila.estado ?? undefined}
      style={{ "--indice": indice, gridRow: indice + 2 } as CSSProperties}
    >
      <span className="fusion-etiqueta">
        {fila.etiqueta}
        {fila.estado === "coincide" && (
          <span className="fusion-coincide">
            <IconoCheck tamano={12} />
            <span className="solo-lector">Coincide</span>
          </span>
        )}
      </span>
      {fila.campo === "titulo" ? (
        <p className="fusion-titulo">{fila.catalogo}</p>
      ) : (
        <Valor texto={fila.catalogo} />
      )}
    </div>
  );
}

/** Un titulo con las palabras que no estan en `contra` resaltadas. */
function TituloComparado({
  texto,
  contra,
}: {
  texto: string;
  contra: string | undefined;
}): ReactElement {
  if (contra === undefined) return <>{texto}</>;
  return (
    <>
      {compararTitulos(texto, contra).map((s, i) => {
        if (!s.distinto) return <Fragment key={i}>{s.texto}</Fragment>;
        const espacio = /^\s*/.exec(s.texto)?.[0] ?? "";
        return (
          <Fragment key={i}>
            {espacio}
            <mark className="diferencia">{s.texto.slice(espacio.length)}</mark>
          </Fragment>
        );
      })}
    </>
  );
}
