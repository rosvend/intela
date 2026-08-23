import { Navigate, Route, Routes, useNavigate } from "react-router-dom";
import { api, setToken, token } from "./api";
import { useEffect, useState } from "react";

type Usuario = { id: string; email: string; nombre: string; rol: string };

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/*" element={<Shell />} />
    </Routes>
  );
}

function Login() {
  const nav = useNavigate();
  const [email, setEmail] = useState("admin@intela.local");
  const [clave, setClave] = useState("intela");
  const [err, setErr] = useState("");
  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    try {
      const r = await api("/api/sesiones", { method: "POST", body: JSON.stringify({ email, clave }) });
      setToken(r.token);
      nav("/");
    } catch (ex) {
      setErr(String(ex));
    }
  }
  return (
    <form className="login card" onSubmit={onSubmit}>
      <div className="badge">Intela · REDES SGC</div>
      <h1>Entrar</h1>
      <p className="muted">Seed local. Clave única: intela. Roles: admin, titular, auditor.</p>
      <p><input value={email} onChange={(e) => setEmail(e.target.value)} /></p>
      <p><input type="password" value={clave} onChange={(e) => setClave(e.target.value)} /></p>
      {err && <p>{err}</p>}
      <button>Continuar</button>
    </form>
  );
}

function Shell() {
  const [me, setMe] = useState<Usuario | null>(null);
  useEffect(() => {
    if (!token()) return;
    api("/api/me").then(setMe).catch(() => setMe(null));
  }, []);
  if (!token()) return <Navigate to="/login" />;
  const rol = me?.rol || "";
  return (
    <div className="layout">
      <nav>
        <div className="badge">Intela</div>
        <p>{me?.nombre}<br /><span className="muted">{rol}</span></p>
        {(rol === "administrador" || rol === "distribucion" || rol === "contabilidad") && (
          <>
            <a href="/obras">Obras</a>
            <a href="/carga">Carga</a>
            <a href="/oni">Bandeja ONI</a>
            <a href="/procesos">Procesos</a>
            <a href="/parametros">Parametros</a>
            <a href="/alertas">Alertas</a>
          </>
        )}
        {rol === "titular" && <a href="/liquidaciones">Mis liquidaciones</a>}
        {rol === "auditoria" && <a href="/asientos">Asientos</a>}
        <a href="/login" onClick={() => localStorage.clear()}>Salir</a>
      </nav>
      <main>
        <Routes>
          <Route path="/" element={<Home rol={rol} />} />
          <Route path="/obras" element={<Tabla path="/api/obras" />} />
          <Route path="/carga" element={<Carga />} />
          <Route path="/oni" element={<ONI />} />
          <Route path="/procesos" element={<Procesos />} />
          <Route path="/parametros" element={<Tabla path="/api/parametros" />} />
          <Route path="/alertas" element={<Tabla path="/api/alertas" />} />
          <Route path="/liquidaciones" element={<Liq />} />
          <Route path="/asientos" element={<Asientos />} />
        </Routes>
      </main>
    </div>
  );
}

function Home({ rol }: { rol: string }) {
  return (
    <div className="card">
      <h1>Tablero</h1>
      <p>El dinero no llega por fila. Las parrillas ponderan una bolsa. Los splits salen de la declaracion de obra.</p>
      <p className="muted">Rol activo: {rol || "…"}</p>
    </div>
  );
}

function Tabla({ path }: { path: string }) {
  const [rows, setRows] = useState<any[]>([]);
  useEffect(() => { api(path).then((r) => setRows(Array.isArray(r) ? r : [r])); }, [path]);
  if (!rows.length) return <p className="muted">Sin filas.</p>;
  const keys = Object.keys(rows[0]);
  return (
    <table>
      <thead><tr>{keys.map((k) => <th key={k}>{k}</th>)}</tr></thead>
      <tbody>{rows.map((r, i) => <tr key={i}>{keys.map((k) => <td key={k}>{fmt(r[k])}</td>)}</tr>)}</tbody>
    </table>
  );
}

function fmt(v: unknown) {
  if (v === null || v === undefined) return "";
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
}

function Carga() {
  const [msg, setMsg] = useState("");
  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const fd = new FormData(e.currentTarget);
    const r = await api("/api/reportes", { method: "POST", body: fd });
    setMsg(JSON.stringify(r));
  }
  return (
    <form className="card" onSubmit={onSubmit}>
      <h2>Cargar reporte</h2>
      <p><input name="fuente" placeholder="fuente" defaultValue="canal-z" /></p>
      <p><input name="periodo" placeholder="periodo" defaultValue="2025" /></p>
      <p><input name="archivo" type="file" /></p>
      <button>Enviar</button>
      <p className="muted">{msg}</p>
    </form>
  );
}

function ONI() {
  const [rows, setRows] = useState<any[]>([]);
  const [obra, setObra] = useState("pelicula-x");
  useEffect(() => { api("/api/oni").then(setRows); }, []);
  async function resolver(id: string) {
    await api(`/api/oni/${id}/resolver`, { method: "POST", body: JSON.stringify({ obra_id: obra }) });
    setRows(await api("/api/oni"));
  }
  return (
    <div>
      <p>Obra destino <input value={obra} onChange={(e) => setObra(e.target.value)} /></p>
      <table>
        <thead><tr><th>titulo</th><th>fuente</th><th></th></tr></thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.id}><td>{r.titulo}</td><td>{r.fuente}</td><td><button type="button" onClick={() => resolver(r.id)}>Resolver</button></td></tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function Procesos() {
  const [rows, setRows] = useState<any[]>([]);
  const [bolsa, setBolsa] = useState("bolsa-z");
  useEffect(() => { api("/api/procesos").then(setRows); }, []);
  async function abrir() {
    await api("/api/procesos", { method: "POST", body: JSON.stringify({ bolsa_id: bolsa, circuito: "nacional" }) });
    setRows(await api("/api/procesos"));
  }
  async function act(id: string, path: string) {
    await api(`/api/procesos/${id}/${path}`, { method: "POST", body: "{}" });
    setRows(await api("/api/procesos"));
  }
  return (
    <div>
      <p><input value={bolsa} onChange={(e) => setBolsa(e.target.value)} /> <button onClick={abrir}>Abrir proceso</button></p>
      {rows.map((p) => (
        <div className="card" key={p.id}>
          <strong>{p.id}</strong> · {p.circuito} · {p.etapa}
          <p>
            <button onClick={() => act(p.id, "calcular")}>Calcular</button>{" "}
            <button onClick={() => act(p.id, "firmar")}>Firmar</button>{" "}
            <button onClick={() => act(p.id, "avanzar")}>Avanzar</button>{" "}
            <a href={`/api/liquidaciones/${p.id}/xlsx`}>Excel</a>{" "}
            <a href={`/api/liquidaciones/${p.id}/pdf`}>PDF</a>
          </p>
        </div>
      ))}
    </div>
  );
}

function Liq() {
  const [data, setData] = useState<any>(null);
  useEffect(() => { api("/api/liquidaciones").then(setData); }, []);
  return <pre className="card">{JSON.stringify(data, null, 2)}</pre>;
}

function Asientos() {
  const [rows, setRows] = useState<any[]>([]);
  const [id, setId] = useState("");
  const [exp, setExp] = useState<any>(null);
  useEffect(() => { api("/api/asientos").then(setRows); }, []);
  return (
    <div>
      <Tabla path="/api/asientos" />
      <p>Explicar cifra <input value={id} onChange={(e) => setId(e.target.value)} />
        <button type="button" onClick={async () => setExp(await api(`/api/asientos/${id}/explicar`))}>Ver linaje</button></p>
      {exp && <pre className="card">{JSON.stringify(exp, null, 2)}</pre>}
      {rows.length === 0 && <p className="muted">Sin asientos.</p>}
    </div>
  );
}
