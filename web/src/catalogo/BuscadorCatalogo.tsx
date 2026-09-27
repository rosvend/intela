import {
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactElement,
} from "react";
import {
  CATEGORIAS,
  categoriaPorId,
  type CategoriaId,
} from "./categoriasDeBusqueda";

/**
 * La barra de busqueda del catalogo: selector de categoria pegado al campo y la
 * lupa a la derecha, con los filtros aplicados como chips debajo.
 *
 * NO es dueno de la URL: recibe lo que la URL dice (`filtros`) y avisa con
 * `onAplicar`, `onQuitar` y `onLimpiar`. Guarda solo estado de interfaz: la
 * categoria elegida (que no va en la URL: al cargar vuelve a Titulo) y el
 * texto de las categorias que se confirman con Enter.
 *
 * El selector, los chips y los iconos son locales y no se exportan: tienen un
 * solo consumidor, y se extraen el dia que otra pantalla los necesite.
 */
export default function BuscadorCatalogo({
  filtros,
  onAplicar,
  onQuitar,
  onLimpiar,
}: {
  filtros: Record<CategoriaId, string>;
  onAplicar: (categoria: CategoriaId, valor: string) => void;
  onQuitar: (categoria: CategoriaId) => void;
  onLimpiar: () => void;
}) {
  const [categoriaId, setCategoriaId] = useState<CategoriaId>("titulo");
  const categoria = categoriaPorId(categoriaId);
  const enVivo = categoria.modo === "vivo";

  // El texto de una categoria que se confirma con Enter es local: solo llega a
  // la URL al confirmar. `visto` es el ultimo valor de la URL para la categoria
  // elegida: si cambia por fuera del campo -boton atras, un enlace, "Limpiar
  // filtros"- el campo lo sigue. Titulo no lo necesita, es reflejo directo.
  const [texto, setTexto] = useState("");
  const [visto, setVisto] = useState("");
  const [aviso, setAviso] = useState<string | null>(null);
  if (!enVivo && filtros[categoriaId] !== visto) {
    setVisto(filtros[categoriaId]);
    setTexto(filtros[categoriaId]);
    setAviso(null);
  }
  const valorDelCampo = enVivo ? filtros.titulo : texto;

  const idAviso = useId();
  const campo = useRef<HTMLInputElement>(null);
  const botonesDeQuitar = useRef<
    Partial<Record<CategoriaId, HTMLButtonElement | null>>
  >({});

  const activos = CATEGORIAS.filter((c) => filtros[c.id] !== "");

  function elegirCategoria(id: CategoriaId) {
    setCategoriaId(id);
    // Al elegir una categoria que ya tiene valor, el campo lo precarga para
    // editarlo; si no lo tiene, queda vacio.
    setTexto(filtros[id]);
    setVisto(filtros[id]);
    setAviso(null);
  }

  function escribir(valor: string) {
    setAviso(null);
    if (enVivo) onAplicar("titulo", valor);
    else setTexto(valor);
  }

  function confirmar() {
    const valor = valorDelCampo.trim();
    // Con el campo vacio no se hace nada: borrar el texto no debe quitar un
    // filtro sin que quien busca lo vea. Se quita con la x del chip.
    if (valor === "") return;
    const mensaje = categoria.validar?.(valor) ?? null;
    if (mensaje !== null) {
      setAviso(mensaje);
      return;
    }
    setAviso(null);
    onAplicar(categoriaId, valor);
    if (!enVivo) {
      // El valor queda visible en su chip, como un filtro anadido. Se marca
      // como visto para que la URL que acaba de cambiar no lo devuelva al campo.
      setTexto("");
      setVisto(valor);
    }
  }

  function alTeclearEnElCampo(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key !== "Enter") return;
    e.preventDefault();
    confirmar();
  }

  function quitar(id: CategoriaId) {
    // El foco pasa al chip siguiente, o al campo si era el ultimo, para que no
    // se pierda cuando desaparece el boton que lo tenia.
    const posicion = activos.findIndex((c) => c.id === id);
    const siguiente = activos[posicion + 1];
    const destino = siguiente
      ? botonesDeQuitar.current[siguiente.id]
      : campo.current;
    destino?.focus();
    onQuitar(id);
  }

  const mostrarError = aviso !== null;

  return (
    <div className="buscador" role="search" aria-label="Filtros del catálogo">
      <div className="buscador-barra">
        <SelectorDeCategoria
          categoriaId={categoriaId}
          onElegir={elegirCategoria}
          alElegir={() => campo.current?.focus()}
        />
        <span className="buscador-divisor" aria-hidden="true" />
        <input
          ref={campo}
          type="text"
          className="buscador-campo"
          aria-label="Texto de búsqueda"
          autoComplete="off"
          placeholder={categoria.placeholder}
          inputMode={categoria.inputMode}
          value={valorDelCampo}
          onChange={(e) => escribir(e.target.value)}
          onKeyDown={alTeclearEnElCampo}
          aria-invalid={mostrarError ? "true" : undefined}
          aria-describedby={mostrarError ? idAviso : undefined}
        />
        <button
          type="button"
          className="buscador-lupa"
          aria-label="Buscar"
          onClick={confirmar}
        >
          <IconoLupa />
        </button>
      </div>

      {mostrarError && (
        <p id={idAviso} className="buscador-aviso">
          {aviso}
        </p>
      )}

      {activos.length > 0 && (
        <div className="buscador-aplicados">
          <FiltrosAplicados
            activos={activos.map((c) => ({
              id: c.id,
              etiqueta: c.etiqueta,
              valor: filtros[c.id],
            }))}
            registrarBoton={(id, boton) => {
              botonesDeQuitar.current[id] = boton;
            }}
            onQuitar={quitar}
          />
          <button
            type="button"
            className="catalogo-limpiar"
            onClick={onLimpiar}
          >
            Limpiar filtros
          </button>
        </div>
      )}
    </div>
  );
}

/**
 * Select-only combobox del patron WAI-ARIA APG: un boton con `role="combobox"`
 * que abre un listbox. El foco no sale del boton mientras el menu esta abierto;
 * la opcion resaltada se anuncia con `aria-activedescendant`.
 */
function SelectorDeCategoria({
  categoriaId,
  onElegir,
  alElegir,
}: {
  categoriaId: CategoriaId;
  onElegir: (id: CategoriaId) => void;
  /** Tras elegir con teclado o raton el foco pasa al campo. */
  alElegir: () => void;
}) {
  const categoria = categoriaPorId(categoriaId);
  const [abierto, setAbierto] = useState(false);
  const [resaltada, setResaltada] = useState(0);
  const raiz = useRef<HTMLDivElement>(null);
  const idLista = useId();
  const idOpcion = (id: CategoriaId) => `${idLista}-${id}`;
  const ultima = CATEGORIAS.length - 1;

  // Clic fuera cierra. El oyente solo existe mientras el menu esta abierto y
  // se retira en la limpieza.
  useEffect(() => {
    if (!abierto) return;
    function alPulsar(e: MouseEvent) {
      if (raiz.current && !raiz.current.contains(e.target as Node)) {
        setAbierto(false);
      }
    }
    document.addEventListener("mousedown", alPulsar);
    return () => document.removeEventListener("mousedown", alPulsar);
  }, [abierto]);

  function abrir() {
    setResaltada(CATEGORIAS.findIndex((c) => c.id === categoriaId));
    setAbierto(true);
  }

  function elegir(indice: number, conFoco: boolean) {
    onElegir(CATEGORIAS[indice].id);
    setAbierto(false);
    if (conFoco) alElegir();
  }

  function alTeclear(e: KeyboardEvent<HTMLButtonElement>) {
    if (!abierto) {
      if (["ArrowDown", "ArrowUp", "Enter", " "].includes(e.key)) {
        e.preventDefault();
        abrir();
      }
      return;
    }
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setResaltada((i) => Math.min(i + 1, ultima));
        break;
      case "ArrowUp":
        e.preventDefault();
        setResaltada((i) => Math.max(i - 1, 0));
        break;
      case "Home":
        e.preventDefault();
        setResaltada(0);
        break;
      case "End":
        e.preventDefault();
        setResaltada(ultima);
        break;
      case "Enter":
      case " ":
        e.preventDefault();
        elegir(resaltada, true);
        break;
      case "Escape":
        e.preventDefault();
        setAbierto(false);
        break;
      case "Tab":
        // Elige la resaltada y deja que el navegador mueva el foco -al campo,
        // que es el siguiente-.
        elegir(resaltada, false);
        break;
    }
  }

  return (
    <div className="buscador-selector" ref={raiz}>
      <button
        type="button"
        className="buscador-selector-boton"
        role="combobox"
        aria-haspopup="listbox"
        aria-expanded={abierto}
        aria-controls={idLista}
        aria-label="Buscar por"
        aria-activedescendant={
          abierto ? idOpcion(CATEGORIAS[resaltada].id) : undefined
        }
        onClick={() => (abierto ? setAbierto(false) : abrir())}
        onKeyDown={alTeclear}
        // Espacio activa el boton al SOLTAR la tecla; sin esto reabriria el
        // menu justo despues de elegir.
        onKeyUp={(e) => {
          if (e.key === " ") e.preventDefault();
        }}
      >
        <Icono id={categoria.id} />
        <span>{categoria.etiqueta}</span>
        <IconoFlecha />
      </button>
      <ul
        id={idLista}
        role="listbox"
        aria-label="Buscar por"
        className="buscador-menu"
        hidden={!abierto}
      >
        {CATEGORIAS.map((c, indice) => (
          <li
            key={c.id}
            id={idOpcion(c.id)}
            role="option"
            aria-selected={c.id === categoriaId}
            className={
              "buscador-opcion" +
              (indice === resaltada ? " buscador-opcion-resaltada" : "")
            }
            onClick={() => elegir(indice, true)}
          >
            <Icono id={c.id} />
            <span>{c.etiqueta}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/** Los filtros aplicados, uno por categoria y en el orden del menu. */
function FiltrosAplicados({
  activos,
  registrarBoton,
  onQuitar,
}: {
  activos: { id: CategoriaId; etiqueta: string; valor: string }[];
  registrarBoton: (id: CategoriaId, boton: HTMLButtonElement | null) => void;
  onQuitar: (id: CategoriaId) => void;
}) {
  return (
    <ul className="buscador-chips" aria-label="Filtros aplicados">
      {activos.map(({ id, etiqueta, valor }) => (
        <li key={id} className="buscador-chip">
          <span>{`${etiqueta}: ${valor}`}</span>
          <button
            type="button"
            aria-label={`Quitar el filtro ${etiqueta}`}
            ref={(boton) => registrarBoton(id, boton)}
            onClick={() => onQuitar(id)}
          >
            <IconoCerrar />
          </button>
        </li>
      ))}
    </ul>
  );
}

// Iconos en linea: sin libreria, heredan el color (`currentColor`) y no se
// anuncian, el texto que los acompana ya dice lo que son.
function Svg({ children }: { children: ReactElement | ReactElement[] }) {
  return (
    <svg
      viewBox="0 0 24 24"
      width="18"
      height="18"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {children}
    </svg>
  );
}

function Icono({ id }: { id: CategoriaId }) {
  switch (id) {
    case "titulo":
      // Claqueta
      return (
        <Svg>
          <rect x="3" y="9" width="18" height="12" rx="2" />
          <path d="M3 9l2.5-5 4 1.5L7 9M11.5 5.5l4 1.5-2.5 2M17 7l3-2" />
        </Svg>
      );
    case "genero":
      // Etiqueta
      return (
        <Svg>
          <path d="M3 12V4a1 1 0 0 1 1-1h8l9 9-9 9-9-9z" />
          <circle cx="8" cy="8" r="1.5" />
        </Svg>
      );
    case "ipi":
      // Persona
      return (
        <Svg>
          <circle cx="12" cy="8" r="4" />
          <path d="M4 21a8 8 0 0 1 16 0" />
        </Svg>
      );
    case "anio":
      // Calendario
      return (
        <Svg>
          <rect x="3" y="5" width="18" height="16" rx="2" />
          <path d="M3 10h18M8 3v4M16 3v4" />
        </Svg>
      );
  }
}

function IconoLupa() {
  return (
    <Svg>
      <circle cx="11" cy="11" r="7" />
      <path d="M20 20l-4-4" />
    </Svg>
  );
}

function IconoFlecha() {
  return (
    <Svg>
      <path d="M6 9l6 6 6-6" />
    </Svg>
  );
}

function IconoCerrar() {
  return (
    <Svg>
      <path d="M6 6l12 12M18 6L6 18" />
    </Svg>
  );
}
