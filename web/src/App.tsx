import { Link, Route, Routes, useNavigate } from "react-router-dom";
import { FormEvent, useEffect, useState } from "react";
import { api, setToken } from "./api";
import { PanelIngresos } from "./PanelIngresos";

/**
 * Shell del tablero.
 *
 * Enrutado, cliente de API, panel del titular (OE-6) y una comprobacion
 * de que el backend responde. El resto de pantallas operativas entran
 * con su PR.
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
          <Link to="/ingresos">Mis ingresos</Link>
          <Link to="/estado">Estado</Link>
        </nav>
      </header>
      <main>
        <Routes>
          <Route path="/" element={<Inicio />} />
          <Route path="/login" element={<Login />} />
          <Route path="/ingresos" element={<PanelIngresos />} />
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
      <h1>Tablero</h1>
      <p>
        Reconocimiento de obras y distribucion de ingresos por propiedad
        intelectual para REDES SGC.
      </p>
      <p>
        El panel del titular consulta ingresos netos por obra, fuente y periodo,
        y explica cada cifra hasta su origen.
      </p>
    </section>
  );
}

type SesionRespuesta = {
  token: string;
  usuario: { nombre: string; rol: string };
};

function Login() {
  const navigate = useNavigate();
  const [email, setEmail] = useState("");
  const [clave, setClave] = useState("");
  const [error, setError] = useState("");

  async function enviar(e: FormEvent) {
    e.preventDefault();
    setError("");
    try {
      const sesion = (await api("/api/auth/session", {
        method: "POST",
        body: JSON.stringify({ email, clave }),
      })) as SesionRespuesta;
      setToken(sesion.token);
      navigate("/ingresos", { replace: true });
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "no se pudo iniciar sesion",
      );
    }
  }

  return (
    <section className="login">
      <h1>Iniciar sesion</h1>
      <p className="muted">Panel del titular. La sesion recorta lo que ve.</p>
      <form onSubmit={enviar}>
        <label>
          Correo
          <input
            type="email"
            name="email"
            autoComplete="username"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
          />
        </label>
        <label>
          Clave
          <input
            type="password"
            name="clave"
            autoComplete="current-password"
            value={clave}
            onChange={(e) => setClave(e.target.value)}
            required
          />
        </label>
        {error && <p role="alert">{error}</p>}
        <button type="submit">Entrar</button>
      </form>
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
