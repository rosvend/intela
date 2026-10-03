import { Component, Fragment, type ErrorInfo, type ReactNode } from "react";
import { useNavigate } from "react-router-dom";

type Estado = {
  error: Error | null;
  clave: number;
};

type Props = {
  children: ReactNode;
};

/**
 * Ultima red del marco de la app (#130).
 *
 * Un fallo de render (un 2xx con forma inesperada que llega a pintarse, un
 * objeto donde React espera texto) desmonta el arbol y deja la pantalla en
 * blanco. Este limite lo convierte en un estado visible y con salida:
 * reintentar remonta el arbol, y volver al inicio cambia de pantalla.
 *
 * La forma de cada elemento se sigue comprobando donde ya se comprueba (las
 * listas de ingesta) y en las fronteras compartidas (`useApi`, `useRecurso`),
 * que rechazan el `Response` crudo. Repetir esa comprobacion en cada pantalla
 * duplicaria el contrato. Lo que ninguna guarda alcanzo a ver lo dice esta
 * pantalla.
 */
export default class ErrorBoundary extends Component<Props, Estado> {
  state: Estado = { error: null, clave: 0 };

  static getDerivedStateFromError(error: Error): Pick<Estado, "error"> {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error(error, info.componentStack);
  }

  reintentar = () => {
    this.setState((previo) => ({
      error: null,
      clave: previo.clave + 1,
    }));
  };

  render() {
    if (this.state.error) {
      return (
        <PantallaDeFallo
          error={this.state.error}
          alReintentar={this.reintentar}
        />
      );
    }
    return <Fragment key={this.state.clave}>{this.props.children}</Fragment>;
  }
}

function PantallaDeFallo({
  error,
  alReintentar,
}: {
  error: Error;
  alReintentar: () => void;
}) {
  const navigate = useNavigate();

  function irAlInicio() {
    navigate("/");
    alReintentar();
  }

  return (
    <section className="fallo-de-render" role="alert">
      <h1>No se pudo mostrar esta pantalla</h1>
      <p>
        Falló al pintarla. El fallo queda escrito abajo: se puede reintentar, o
        volver al inicio.
      </p>
      <p className="fallo-de-render-detalle">{error.message}</p>
      <div className="acciones">
        <button type="button" className="boton-primario" onClick={alReintentar}>
          Reintentar
        </button>
        <button type="button" className="boton-secundario" onClick={irAlInicio}>
          Volver al inicio
        </button>
      </div>
    </section>
  );
}
