import { Link } from "react-router-dom";

export default function NoEncontrado() {
  return (
    <section>
      <h1>404</h1>
      <p>
        Esa pagina no existe. <Link to="/">Volver al inicio</Link>.
      </p>
    </section>
  );
}
