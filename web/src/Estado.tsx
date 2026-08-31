import { useEffect, useState } from "react";
import Cargando from "./Cargando";
import { api } from "./api";

type EstadoBackend = { estado: string } | null;

export default function Estado() {
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

  if (!salud) {
    return (
      <section>
        <h1>Estado</h1>
        <Cargando texto="Consultando el backend…" />
      </section>
    );
  }

  return (
    <section>
      <h1>Estado</h1>
      <p>Backend: {salud.estado}</p>
    </section>
  );
}
