/**
 * Placeholder para las rutas de Sprint 3-5 que ya tienen entrada en la nav
 * pero cuya pantalla real llega con su propio PR (issue #19: "so the feature
 * screens are pure additions").
 */
export default function EnConstruccion({ titulo }: { titulo: string }) {
  return (
    <section>
      <h1>{titulo}</h1>
      <p className="muted">Esta pantalla llega en un PR posterior.</p>
    </section>
  );
}
