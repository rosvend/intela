import { useEffect, useId, useState, type ReactElement } from "react";
import { Link } from "react-router-dom";
import Cargando from "../Cargando";
import { useApi } from "../useApi";
import Dialogo from "./Dialogo";
import PanelResolucion, { type ModoResolucion } from "./PanelResolucion";
import { useFijarPendientes } from "./pendientes";
import {
  LIMITE_BANDEJA,
  RUTAS_IDENTIFICACION,
  esPaginaDeCasos,
  etiquetaDeModalidad,
  formatearPuntaje,
  idsDeFuente,
  type CandidatoIdentificacion,
  type CasoIdentificacion,
} from "./tipos";

/** El caso sobre el que se abrio el panel, y en que modo. */
type PanelAbierto = { caso: CasoIdentificacion; modo: ModoResolucion };

/** El aviso de exito, `role="status"` durante 4 segundos (plano seccion 5). */
type Aviso = { detalle: string };

const DURACION_AVISO_MS = 4000;

/**
 * Bandeja de identificacion (`/identificacion`): los casos ONI pendientes,
 * uno por tarjeta, con sus obras candidatas.
 *
 * Es solo el envoltorio que fuerza la recarga completa (D2, D3): `useApi` no
 * vuelve a pedir el mismo path, asi que "Intentar de nuevo", "Cargar los
 * siguientes casos" y "Recargar caso" incrementan `recarga`, que via `key`
 * remonta `Contenido` entero -useApi pide de nuevo, y el estado local de la
 * lista (`fuera`, el panel abierto, el aviso) arranca limpio, que es
 * exactamente lo que "recargar desde el principio de la cola" quiere decir.
 */
export default function BandejaIdentificacion(): ReactElement {
  const [recarga, setRecarga] = useState(0);
  return (
    <Contenido key={recarga} onRecargar={() => setRecarga((r) => r + 1)} />
  );
}

function Contenido({ onRecargar }: { onRecargar: () => void }): ReactElement {
  const fijarPendientes = useFijarPendientes();
  const lectura = useApi<unknown>(
    RUTAS_IDENTIFICACION.casos({
      estado: "pendiente",
      limite: LIMITE_BANDEJA,
    }),
  );

  // Remocion optimista (D3): los ids que ya salieron de la vista, sea porque
  // el servidor confirmo la resolucion o porque quedaron fuera mientras se
  // reintenta. `Set`, no filtrar el array de `useApi`: la pagina que trajo
  // `useApi` no cambia con la recarga optimista, solo lo que se PINTA de ella.
  const [fuera, setFuera] = useState<Set<string>>(new Set());
  const [panel, setPanel] = useState<PanelAbierto | null>(null);
  const [bloqueado, setBloqueado] = useState(false);
  const [aviso, setAviso] = useState<Aviso | null>(null);
  const idTituloPanel = useId();

  const pagina =
    !lectura.cargando && !lectura.error && esPaginaDeCasos(lectura.datos)
      ? lectura.datos
      : null;
  const casosVisibles = pagina
    ? pagina.casos.filter((c) => !fuera.has(c.id))
    : [];
  // `pagina.pendientes` es el conteo del servidor al momento de la carga;
  // restarle `fuera.size` lo mantiene en vivo sin pedir de nuevo el recurso
  // -el mismo criterio que `usePendientesDeIdentificacion` aplica del lado
  // del badge, aqui del lado de quien lo alimenta.
  const conteo = pagina
    ? Math.max(pagina.pendientes - fuera.size, 0)
    : undefined;

  useEffect(() => {
    if (conteo !== undefined) fijarPendientes(conteo);
    // `fijarPendientes` es estable (viene de `useFijarPendientes`, que la
    // memoiza); solo `conteo` decide cuando volver a empujar.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [conteo]);

  useEffect(() => {
    if (!aviso) return;
    const temporizador = setTimeout(() => setAviso(null), DURACION_AVISO_MS);
    return () => clearTimeout(temporizador);
  }, [aviso]);

  function marcarFuera(id: string) {
    setFuera((previos) => {
      const siguientes = new Set(previos);
      siguientes.add(id);
      return siguientes;
    });
  }

  function devolverALaLista(id: string) {
    setFuera((previos) => {
      const siguientes = new Set(previos);
      siguientes.delete(id);
      return siguientes;
    });
  }

  function cerrarPanel() {
    if (bloqueado) return;
    setPanel(null);
  }

  return (
    <section className="bandeja" aria-label="Bandeja de identificación">
      <header className="bandeja-cabecera">
        <div className="bandeja-titulo-fila">
          <h1>Bandeja de identificación</h1>
          {conteo !== undefined && (
            <span className="pastilla-conteo">
              {conteo === 1 ? "1 caso pendiente" : `${conteo} casos pendientes`}
            </span>
          )}
        </div>
        <p className="bandeja-intro">
          Revisa la evidencia y decide la obra correcta. Intela nunca asigna un
          registro a ciegas.
        </p>
        <Link to="/lista-oni" className="boton-secundario">
          Ver lista ONI
        </Link>
      </header>

      {aviso && (
        <p className="bandeja-aviso" role="status">
          <strong>Decisión registrada</strong> {aviso.detalle}
        </p>
      )}

      {lectura.cargando && <Cargando texto="Cargando los casos pendientes…" />}

      {!lectura.cargando && lectura.error && (
        <div className="bandeja-error" role="alert">
          <p>No pudimos cargar los casos: {lectura.error.message}</p>
          <button
            type="button"
            className="boton-secundario"
            onClick={onRecargar}
          >
            Intentar de nuevo
          </button>
        </div>
      )}

      {!lectura.cargando && !lectura.error && pagina === null && (
        <p className="bandeja-error" role="alert">
          La bandeja no llegó como una lista de casos legibles.
        </p>
      )}

      {pagina !== null && (
        <>
          {casosVisibles.length === 0 && (conteo ?? 0) === 0 && (
            <div className="bandeja-vacia">
              <p>No hay casos pendientes</p>
              <p className="muted">
                {fuera.size > 0
                  ? "Todas las entradas fueron asignadas o descartadas con trazabilidad."
                  : "La cascada de identificación procesó todos los registros disponibles."}
              </p>
            </div>
          )}

          {casosVisibles.length > 0 && (
            <ul className="bandeja-lista">
              {casosVisibles.map((caso) => (
                <TarjetaCaso
                  key={caso.id}
                  caso={caso}
                  onAsignarCandidata={(candidato) =>
                    setPanel({
                      caso,
                      modo: {
                        tipo: "candidata",
                        obraId: candidato.obra_id,
                        obraTitulo: candidato.titulo,
                        obraAnio: candidato.anio,
                        obraGenero: candidato.genero,
                        puntaje: candidato.puntaje,
                      },
                    })
                  }
                  onBuscarOtraObra={() =>
                    setPanel({ caso, modo: { tipo: "busqueda" } })
                  }
                  onDescartar={() =>
                    setPanel({ caso, modo: { tipo: "descarte" } })
                  }
                />
              ))}
            </ul>
          )}

          {conteo !== undefined && conteo > casosVisibles.length && (
            <div className="bandeja-mas">
              <p>
                Se muestran {casosVisibles.length} de {conteo} casos pendientes,
                en orden de llegada del reporte.
              </p>
              <button
                type="button"
                className="boton-secundario"
                onClick={onRecargar}
              >
                Cargar los siguientes casos
              </button>
            </div>
          )}
        </>
      )}

      <Dialogo
        abierto={panel !== null}
        variante="lateral"
        idTitulo={idTituloPanel}
        bloqueado={bloqueado}
        onCerrar={cerrarPanel}
      >
        {panel && (
          <PanelResolucion
            idTitulo={idTituloPanel}
            caso={panel.caso}
            modo={panel.modo}
            onCancelar={cerrarPanel}
            onEnviandoCambia={setBloqueado}
            onEnviarInicio={() => marcarFuera(panel.caso.id)}
            onFalla={(volvioALaLista) => {
              if (volvioALaLista) devolverALaLista(panel.caso.id);
            }}
            onExito={({ obraTitulo, esDescarte }) => {
              setPanel(null);
              setAviso({
                detalle: esDescarte
                  ? `Registro “${panel.caso.titulo}” descartado`
                  : `Registro asignado a “${obraTitulo}”`,
              });
            }}
            onRecargarTodo={onRecargar}
          />
        )}
      </Dialogo>
    </section>
  );
}

function TarjetaCaso({
  caso,
  onAsignarCandidata,
  onBuscarOtraObra,
  onDescartar,
}: {
  caso: CasoIdentificacion;
  onAsignarCandidata: (candidato: CandidatoIdentificacion) => void;
  onBuscarOtraObra: () => void;
  onDescartar: () => void;
}): ReactElement {
  // Expandida por defecto (plano seccion 5): nadie tiene que abrir la
  // tarjeta para ver por que un registro esta en la bandeja.
  const [expandida, setExpandida] = useState(true);
  const idCuerpo = useId();

  return (
    <li className="bandeja-caso">
      <button
        type="button"
        className="bandeja-caso-cabecera"
        aria-expanded={expandida}
        aria-controls={idCuerpo}
        onClick={() => setExpandida((v) => !v)}
      >
        <div className="bandeja-caso-titulo-fila">
          <h2>{caso.titulo}</h2>
          <span className="etiqueta-modalidad">
            {etiquetaDeModalidad(caso.modalidad)}
          </span>
          <span className="etiqueta-id">{caso.id}</span>
        </div>
        <p className="muted">
          {caso.fuente} · {caso.periodo} · Entrega {caso.reporte_id}
        </p>
      </button>

      {expandida && (
        <div id={idCuerpo} className="bandeja-caso-cuerpo">
          <div className="bandeja-caso-datos">
            <section className="bandeja-caso-entrada">
              <h3>Entrada del reporte</h3>
              <dl>
                <div>
                  <dt>Título emitido</dt>
                  <dd>{caso.titulo}</dd>
                </div>
                <div>
                  <dt>Título original</dt>
                  <dd>{caso.titulo_original || "—"}</dd>
                </div>
                <div>
                  <dt>Fuente</dt>
                  <dd>{caso.fuente}</dd>
                </div>
              </dl>
            </section>
            <section className="bandeja-caso-evidencia">
              <h3>Evidencia del sistema</h3>
              <ul className="bandeja-fichas">
                {idsDeFuente(caso.ids_fuente).map((id) => (
                  <li key={id} className="ficha">
                    {id}
                  </li>
                ))}
              </ul>
              <p>{caso.evidencia}</p>
            </section>
          </div>

          <section className="bandeja-candidatas">
            <div className="bandeja-candidatas-cabecera">
              <div>
                <h3>Obras candidatas</h3>
                <p className="muted">
                  Ordenadas por puntaje. Ninguna se asigna automáticamente.
                </p>
              </div>
              <span className="muted">
                {caso.candidatos.length} coincidencias
              </span>
            </div>

            {caso.candidatos.length === 0 ? (
              <div className="bandeja-sin-candidatas">
                <p>No encontramos obras candidatas</p>
                <p className="muted">
                  Busca manualmente en el catálogo o descarta el registro.
                </p>
              </div>
            ) : (
              <ul className="bandeja-lista-candidatas">
                {caso.candidatos.map((candidato) => (
                  <li key={candidato.obra_id} className="bandeja-candidato">
                    <div className="bandeja-candidato-cabecera">
                      <span className="bandeja-candidato-titulo">
                        {candidato.titulo}
                      </span>
                      <span className="pastilla-puntaje">
                        {formatearPuntaje(candidato.puntaje)}
                      </span>
                    </div>
                    <p className="muted">
                      {candidato.anio} · {candidato.genero} · Comparado con “
                      {candidato.titulo_consultado}”
                    </p>
                    <button
                      type="button"
                      className="boton-primario"
                      aria-label={`Asignar a esta obra: ${candidato.titulo}`}
                      onClick={() => onAsignarCandidata(candidato)}
                    >
                      Asignar a esta obra
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </section>

          <div className="bandeja-caso-pie">
            <button
              type="button"
              className="boton-secundario"
              onClick={onBuscarOtraObra}
            >
              Buscar otra obra
            </button>
            <button
              type="button"
              className="boton-secundario"
              onClick={onDescartar}
            >
              Descartar registro
            </button>
          </div>
        </div>
      )}
    </li>
  );
}
