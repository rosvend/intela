import { useEffect, useId, useRef, useState, type ReactElement } from "react";
import { Link } from "react-router-dom";
import "../revision.css";
import Detalle from "../ui/Detalle";
import { useApi } from "../useApi";
import DetalleTecnico from "./DetalleTecnico";
import Dialogo from "./Dialogo";
import {
  IconoCheck,
  IconoChispa,
  IconoDescartar,
  IconoInfo,
  IconoLupa,
  IconoPantalla,
  IconoPregunta,
  IconoTodoEnOrden,
} from "./iconos";
import MedidorConfianza from "./MedidorConfianza";
import PanelResolucion, { type ModoResolucion } from "./PanelResolucion";
import { useFijarPendientes } from "./pendientes";
import {
  LIMITE_BANDEJA,
  RUTAS_IDENTIFICACION,
  esPaginaDeCasos,
  etiquetaDeModalidad,
  type CandidatoIdentificacion,
  type CasoIdentificacion,
  type SugerenciaIdentificacion,
} from "./tipos";
import {
  explicarCandidata,
  formatearPeriodo,
  nombreDeFuente,
  prefiereQuieto,
} from "./presentacion";

/** El caso sobre el que se abrio el panel, y en que modo. */
type PanelAbierto = { caso: CasoIdentificacion; modo: ModoResolucion };

/** El aviso de exito, `role="status"` durante 4 segundos (plano seccion 5). */
type Aviso = { detalle: string };

const DURACION_AVISO_MS = 4000;

/** Lo que dura la salida animada de una tarjeta (revision.css, `caso-salir`). */
const DURACION_SALIDA_MS = 220;

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
  // Las tarjetas que se estan yendo: siguen pintadas mientras dura la salida.
  const [saliendo, setSaliendo] = useState<Set<string>>(new Set());
  const temporizadores = useRef(new Map<string, number>());
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
  // restarle lo que ya salio lo mantiene en vivo sin pedir de nuevo.
  const conteo = pagina
    ? Math.max(pagina.pendientes - fuera.size - saliendo.size, 0)
    : undefined;

  useEffect(() => {
    if (conteo !== undefined) fijarPendientes(conteo);
    // `fijarPendientes` es estable (memoizada en `useFijarPendientes`).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [conteo]);

  useEffect(() => {
    if (!aviso) return;
    const temporizador = setTimeout(() => setAviso(null), DURACION_AVISO_MS);
    return () => clearTimeout(temporizador);
  }, [aviso]);

  useEffect(() => {
    const pendientes = temporizadores.current;
    return () => pendientes.forEach((t) => window.clearTimeout(t));
  }, []);

  function conId(previos: Set<string>, id: string, poner: boolean) {
    const siguientes = new Set(previos);
    if (poner) siguientes.add(id);
    else siguientes.delete(id);
    return siguientes;
  }

  function marcarFuera(id: string) {
    if (prefiereQuieto()) {
      setFuera((p) => conId(p, id, true));
      return;
    }
    setSaliendo((p) => conId(p, id, true));
    const t = window.setTimeout(() => {
      temporizadores.current.delete(id);
      setSaliendo((p) => conId(p, id, false));
      setFuera((p) => conId(p, id, true));
    }, DURACION_SALIDA_MS);
    temporizadores.current.set(id, t);
  }

  function devolverALaLista(id: string) {
    window.clearTimeout(temporizadores.current.get(id));
    temporizadores.current.delete(id);
    setSaliendo((p) => conId(p, id, false));
    setFuera((p) => conId(p, id, false));
  }

  function cerrarPanel() {
    if (bloqueado) return;
    setPanel(null);
  }

  return (
    <section
      className="bandeja revision"
      aria-label="Bandeja de identificación"
    >
      <header className="revision-cabecera">
        <div className="revision-titulo-fila">
          <h1>Bandeja de identificación</h1>
          {conteo !== undefined && conteo > 0 && (
            <span className="chip chip-marca">{conteo} por revisar</span>
          )}
          <Link to="/lista-oni" className="revision-enlace">
            Ver lista ONI
          </Link>
        </div>
        <p className="revision-intro">
          Usos reportados que no pudimos asociar con seguridad a una obra del
          catálogo. Elige la obra correcta o descártalo.
        </p>
      </header>

      {aviso && (
        <div className="revision-toast" role="status">
          <span className="revision-toast-icono">
            <IconoCheck tamano={16} />
          </span>
          <strong>Decisión registrada</strong>
          <span>{aviso.detalle}</span>
        </div>
      )}

      {lectura.cargando && (
        <EsqueletoDeCasos etiqueta="Cargando los casos pendientes" />
      )}

      {!lectura.cargando && lectura.error && (
        <div className="revision-aviso-error" role="alert">
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
        <p className="revision-aviso-error" role="alert">
          La bandeja no llegó como una lista de casos legibles.
        </p>
      )}

      {pagina !== null && (
        <>
          {casosVisibles.length === 0 && (conteo ?? 0) === 0 && (
            <EstadoVacio
              titulo="Todo en orden: no hay usos pendientes por identificar"
              texto={
                fuera.size > 0
                  ? "Cada decisión quedó registrada con su nota."
                  : "Cuando llegue un uso dudoso, aparecerá aquí."
              }
            />
          )}

          {casosVisibles.length > 0 && (
            <ul className="casos">
              {casosVisibles.map((caso, i) => (
                <TarjetaCaso
                  key={caso.id}
                  caso={caso}
                  indice={i}
                  saliendo={saliendo.has(caso.id)}
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
            <div className="revision-mas">
              <p>
                Se muestran {casosVisibles.length} de {conteo}, en orden de
                llegada.
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

/** Esqueleto de carga: la forma de las tarjetas, sin texto que leer. */
export function EsqueletoDeCasos({
  etiqueta,
}: {
  etiqueta: string;
}): ReactElement {
  return (
    <div
      className="esqueletos"
      role="status"
      aria-label={etiqueta}
      aria-busy="true"
    >
      {[0, 1, 2].map((i) => (
        <div key={i} className="esqueleto-caso" aria-hidden="true">
          <span className="esqueleto esqueleto-titulo" />
          <span className="esqueleto esqueleto-linea" />
          <span className="esqueleto esqueleto-barra" />
        </div>
      ))}
    </div>
  );
}

export function EstadoVacio({
  titulo,
  texto,
}: {
  titulo: string;
  texto?: string;
}): ReactElement {
  return (
    <div className="revision-vacio">
      <span className="revision-vacio-icono">
        <IconoTodoEnOrden />
      </span>
      <p className="revision-vacio-titulo">{titulo}</p>
      {texto && <p className="revision-vacio-texto">{texto}</p>}
    </div>
  );
}

function TarjetaCaso({
  caso,
  indice,
  saliendo,
  onAsignarCandidata,
  onBuscarOtraObra,
  onDescartar,
}: {
  caso: CasoIdentificacion;
  indice: number;
  saliendo: boolean;
  onAsignarCandidata: (candidato: CandidatoIdentificacion) => void;
  onBuscarOtraObra: () => void;
  onDescartar: () => void;
}): ReactElement {
  const sugerenciaTexto = textoDeSugerencia(caso.sugerencia);

  return (
    <li
      className={`caso${saliendo ? " caso-saliendo" : ""}`}
      style={{ "--indice": Math.min(indice, 8) } as React.CSSProperties}
      aria-hidden={saliendo || undefined}
    >
      <header className="caso-cabecera">
        <span className="caso-icono">
          <IconoPantalla />
        </span>
        <div className="caso-reportado">
          <h2 className="caso-titulo">{caso.titulo}</h2>
          <p className="caso-meta">
            {nombreDeFuente(caso.fuente)} · {formatearPeriodo(caso.periodo)}
          </p>
        </div>
        <span className="chip">{etiquetaDeModalidad(caso.modalidad)}</span>
        <Detalle
          titulo="Detalle técnico"
          etiquetaDisparador="Detalle técnico"
          claseDisparador="revision-icono-boton"
          disparador={<IconoInfo />}
        >
          <DetalleTecnico caso={caso} />
        </Detalle>
      </header>

      {sugerenciaTexto && (
        <p className="caso-sugerencia">
          <IconoChispa />
          <span>{sugerenciaTexto}</span>
        </p>
      )}

      {caso.candidatos.length === 0 ? (
        <p className="caso-sin-candidatas">
          No encontramos obras parecidas en el catálogo. Búscala a mano o
          descarta el uso.
        </p>
      ) : (
        <ul className="candidatas" aria-label="Obras posibles">
          {caso.candidatos.map((candidato) => (
            <li key={candidato.obra_id} className="candidata">
              <div className="candidata-obra">
                <span className="candidata-titulo">{candidato.titulo}</span>
                <span className="candidata-meta">
                  {candidato.anio} · {candidato.genero}
                </span>
              </div>
              <MedidorConfianza puntaje={candidato.puntaje} />
              <Detalle
                titulo={`Por qué ${candidato.titulo} es candidata`}
                etiquetaDisparador={`Por qué ${candidato.titulo} es candidata`}
                claseDisparador="revision-icono-boton"
                disparador={<IconoPregunta />}
              >
                <span className="detalle-texto">
                  {explicarCandidata(candidato)}
                </span>
              </Detalle>
              <button
                type="button"
                className="boton-una"
                aria-label={`Es esta obra: ${candidato.titulo}`}
                onClick={() => onAsignarCandidata(candidato)}
              >
                <IconoCheck tamano={16} />
                Es esta obra
              </button>
            </li>
          ))}
        </ul>
      )}

      <footer className="caso-pie">
        <button
          type="button"
          className="boton-fantasma"
          onClick={onBuscarOtraObra}
        >
          <IconoLupa />
          Buscar otra obra
        </button>
        <button type="button" className="boton-fantasma" onClick={onDescartar}>
          <IconoDescartar />
          Descartar
        </button>
      </footer>
    </li>
  );
}

/** Texto de la propuesta. `null` si no hay con que sugerir: no se pinta un aviso vacio. */
function textoDeSugerencia(
  sugerencia: SugerenciaIdentificacion,
): string | null {
  if (sugerencia.decision === "ninguna") return null;
  if (sugerencia.decision === "descartar") {
    return "Sugerencia: descartar este registro.";
  }
  const nombre = sugerencia.titulo || sugerencia.obra_id;
  return `Sugerencia: asignar a ${nombre}.`;
}
