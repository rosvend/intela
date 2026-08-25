import { Link, Route, Routes } from "react-router-dom";
import { useEffect, useState } from "react";
import { api } from "./api";

/**
 * Shell del tablero.
 *
 * Es andamiaje: enrutado, cliente de API y una comprobacion de que el backend
 * responde. Las pantallas operativas -obras, carga, bandeja ONI, procesos,
 * parametros, alertas, liquidaciones y asientos- entran con su PR, junto a
 * los endpoints que consumen.
 *
 * La navegacion usa <Link>, no <a href>. Con <a href> cada clic recarga la
 * pagina entera y pierde el estado; y sin `try_files` en nginx -que hasta
 * ahora tampoco estaba- devuelve 404 directamente.
 */
export default function App() {
  return (
    <div className="shell">
      <header>
        <strong>Intela</strong>
        <nav>
          <Link to="/">Inicio</Link>
          <Link to="/estado">Estado</Link>
        </nav>
      </header>
      <main>
        <Routes>
          <Route path="/" element={<Inicio />} />
          <Route path="/estado" element={<Estado />} />
          <Route path="*" element={<NoEncontrado />} />
        </Routes>
      </main>
    </div>
  );
}

function Inicio() {
  return (
    <section>
      <h1>Tablero de administracion</h1>
      <p>
        Reconocimiento de obras y distribucion de ingresos por propiedad
        intelectual para REDES SGC.
      </p>
      <p>
        Esto es el andamiaje. Las pantallas operativas llegan con los PRs de
        cada modulo.
      </p>
    </section>
  );
}

type EstadoBackend = { estado: string } | null;

function Estado() {
  const [salud, setSalud] = useState<EstadoBackend>(null);
  const [error, setError] = useState<string>("");

  useEffect(() => {
    let vigente = true;
    api("/api/ready")
      .then((r) => vigente && setSalud(r))
      .catch((e: Error) => vigente && setError(e.message));
    // Evita escribir estado si el componente se desmonto mientras tanto.
    return () => {
      vigente = false;
    };
  }, []);

  if (error) {
    return (
      <section>
        <h1>Estado</h1>
        <p role="alert">El backend no responde: {error}</p>
      </section>
    );
  }

  return (
    <section>
      <h1>Estado</h1>
      <p>{salud ? `Backend: ${salud.estado}` : "Consultando..."}</p>
    </section>
  );
}

function NoEncontrado() {
  return (
    <section>
      <h1>404</h1>
      <p>
        Esa pagina no existe. <Link to="/">Volver al inicio</Link>.
      </p>
    </section>
  );
}
