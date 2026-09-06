import Cargando from "./Cargando";
import { useApi } from "./useApi";

export default function Estado() {
  const { datos, cargando, error } = useApi<{ estado: string }>("/api/ready");

  if (error) {
    return (
      <section>
        <h1>Estado</h1>
        <p role="alert">El backend no responde: {error.message}</p>
      </section>
    );
  }

  if (cargando) {
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
      <p>Backend: {datos.estado}</p>
    </section>
  );
}
